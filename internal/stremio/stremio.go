// Package stremio is media-player's Movies & TV section: catalogs, details and
// streams from Stremio add-ons (Cinemeta for metadata, Torrentio first and
// Comet as a backup for Real-Debrid streams), played in Seanime's own player.
//
// Movies and series have no AniList entry, so each title gets a synthetic
// negative media ID (see SyntheticID); Seanime's watch history then gives
// resume and Continue Watching for free, and AniList is never told about them.
package stremio

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	CinemetaURL  = "https://v3-cinemeta.strem.io"
	TorrentioURL = "https://torrentio.strem.fun"
	CometURL     = "https://comet.elfhosted.com"
)

// MetaPreview is a catalog or search result.
type MetaPreview struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Poster      string   `json:"poster,omitempty"`
	Background  string   `json:"background,omitempty"`
	Logo        string   `json:"logo,omitempty"`
	Description string   `json:"description,omitempty"`
	ReleaseInfo string   `json:"releaseInfo,omitempty"`
	ImdbRating  string   `json:"imdbRating,omitempty"`
	Genres      []string `json:"genres,omitempty"`
	Runtime     string   `json:"runtime,omitempty"`
}

// Video is one episode of a series.
type Video struct {
	ID        string `json:"id"` // "tt1234567:1:3"
	Name      string `json:"name,omitempty"`
	Title     string `json:"title,omitempty"`
	Season    int    `json:"season"`
	Episode   int    `json:"episode"`
	Released  string `json:"released,omitempty"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Overview  string `json:"overview,omitempty"`
}

// Meta is a title's details; Videos holds a series' episodes.
type Meta struct {
	MetaPreview
	Cast     []string `json:"cast,omitempty"`
	Director []string `json:"director,omitempty"`
	Videos   []Video  `json:"videos,omitempty"`
}

// Stream is one playable source for a movie or episode.
type Stream struct {
	Source     string `json:"source"` // "torrentio" or "comet"
	Name       string `json:"name"`
	Title      string `json:"title"`
	Filename   string `json:"filename,omitempty"`
	Quality    string `json:"quality,omitempty"` // "2160p", "1080p", ...
	HDR        string `json:"hdr,omitempty"`     // "DV", "HDR", ...
	Size       string `json:"size,omitempty"`
	SizeBytes  int64  `json:"sizeBytes,omitempty"`
	Cached     bool   `json:"cached"`
	URL        string `json:"url"`
	BingeGroup string `json:"bingeGroup,omitempty"`
}

// Client talks to the add-ons. RDKey returns the Real-Debrid API key ("" if none).
type Client struct {
	RDKey func() string
	http  *http.Client

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	body    []byte
	expires time.Time
}

func NewClient(rdKey func() string) *Client {
	return &Client{
		RDKey: rdKey,
		http:  &http.Client{Timeout: 25 * time.Second},
		cache: map[string]cacheEntry{},
	}
}

func (c *Client) getJSON(ctx context.Context, u string, ttl time.Duration, out any) error {
	c.mu.Lock()
	if e, ok := c.cache[u]; ok && time.Now().Before(e.expires) {
		c.mu.Unlock()
		return json.Unmarshal(e.body, out)
	}
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "media-player")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", redact(u), resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if ttl > 0 {
		c.mu.Lock()
		if len(c.cache) > 500 {
			c.cache = map[string]cacheEntry{}
		}
		c.cache[u] = cacheEntry{body: body, expires: time.Now().Add(ttl)}
		c.mu.Unlock()
	}
	return json.Unmarshal(body, out)
}

// redact hides the API key in add-on URLs for error messages and logs.
func redact(u string) string {
	return regexp.MustCompile(`realdebrid=[^/|]+|/[A-Za-z0-9+/=_-]{60,}/`).ReplaceAllString(u, "…")
}

// ---- catalogs and details ----------------------------------------------

func validType(t string) bool { return t == "movie" || t == "series" }

// Catalog returns a Cinemeta catalog ("top", "imdbRating", "year").
func (c *Client) Catalog(ctx context.Context, typ, id string, skip int, genre string) ([]MetaPreview, error) {
	if !validType(typ) {
		return nil, errors.New("type must be movie or series")
	}
	var extras []string
	if genre != "" {
		extras = append(extras, "genre="+url.PathEscape(genre))
	}
	if skip > 0 {
		extras = append(extras, "skip="+strconv.Itoa(skip))
	}
	u := fmt.Sprintf("%s/catalog/%s/%s", CinemetaURL, typ, url.PathEscape(id))
	if len(extras) > 0 {
		u += "/" + strings.Join(extras, "&")
	}
	var r struct {
		Metas []MetaPreview `json:"metas"`
	}
	if err := c.getJSON(ctx, u+".json", 6*time.Hour, &r); err != nil {
		return nil, err
	}
	return r.Metas, nil
}

// Search looks through movies and series.
func (c *Client) Search(ctx context.Context, q string) ([]MetaPreview, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	var all []MetaPreview
	var firstErr error
	for _, typ := range []string{"movie", "series"} {
		var r struct {
			Metas []MetaPreview `json:"metas"`
		}
		u := fmt.Sprintf("%s/catalog/%s/top/search=%s.json", CinemetaURL, typ, url.PathEscape(q))
		if err := c.getJSON(ctx, u, time.Hour, &r); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		all = append(all, r.Metas...)
	}
	if len(all) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return all, nil
}

// Meta returns a title's details, with a series' episodes sorted.
func (c *Client) Meta(ctx context.Context, typ, id string) (*Meta, error) {
	if !validType(typ) {
		return nil, errors.New("type must be movie or series")
	}
	var r struct {
		Meta Meta `json:"meta"`
	}
	if err := c.getJSON(ctx, fmt.Sprintf("%s/meta/%s/%s.json", CinemetaURL, typ, url.PathEscape(id)), 6*time.Hour, &r); err != nil {
		return nil, err
	}
	if r.Meta.ID == "" {
		return nil, errors.New("title not found")
	}
	sort.SliceStable(r.Meta.Videos, func(i, j int) bool {
		a, b := r.Meta.Videos[i], r.Meta.Videos[j]
		if a.Season != b.Season {
			return a.Season < b.Season
		}
		return a.Episode < b.Episode
	})
	return &r.Meta, nil
}

// ---- streams -------------------------------------------------------------

func (c *Client) torrentioBase() string {
	if k := c.RDKey(); k != "" {
		return TorrentioURL + "/realdebrid=" + k
	}
	return TorrentioURL
}

func (c *Client) cometBase() string {
	k := c.RDKey()
	if k == "" {
		return CometURL
	}
	cfg, _ := json.Marshal(map[string]any{
		"debridServices": []map[string]string{{"service": "realdebrid", "apiKey": k}},
	})
	return CometURL + "/" + base64.StdEncoding.EncodeToString(cfg)
}

type rawStream struct {
	Name          string `json:"name"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	URL           string `json:"url"`
	BehaviorHints struct {
		Filename   string `json:"filename"`
		BingeGroup string `json:"bingeGroup"`
		VideoSize  int64  `json:"videoSize"`
	} `json:"behaviorHints"`
}

var (
	qualityRe = regexp.MustCompile(`(?i)\b(4k|2160p|1440p|1080p|720p|576p|480p)\b`)
	hdrRe     = regexp.MustCompile(`\b(DV|HDR10\+|HDR10|HDR)\b`)
	sizeRe    = regexp.MustCompile(`💾\s*([\d.]+)\s*(GB|MB|GiB|MiB|TB)`)
	fileRe    = regexp.MustCompile(`📄\s*([^\n]+)`)
)

func normalize(source string, r rawStream) Stream {
	text := r.Title
	if text == "" {
		text = r.Description
	}
	s := Stream{Source: source, Name: r.Name, Title: text, URL: r.URL,
		Filename: r.BehaviorHints.Filename, BingeGroup: r.BehaviorHints.BingeGroup}
	if s.Filename == "" {
		if m := fileRe.FindStringSubmatch(text); m != nil {
			s.Filename = strings.TrimSpace(m[1])
		}
	}
	if m := qualityRe.FindString(r.Name + " " + text); m != "" {
		s.Quality = strings.ToLower(m)
		if s.Quality == "4k" {
			s.Quality = "2160p"
		}
	}
	var tags []string
	for _, m := range hdrRe.FindAllString(r.Name+" "+text, -1) {
		if !slices.Contains(tags, m) {
			tags = append(tags, m)
		}
	}
	s.HDR = strings.Join(tags, " ")
	if m := sizeRe.FindStringSubmatch(text); m != nil {
		s.Size = m[1] + " " + m[2]
		v, _ := strconv.ParseFloat(m[1], 64)
		mult := map[string]float64{"MB": 1e6, "MiB": 1 << 20, "GB": 1e9, "GiB": 1 << 30, "TB": 1e12}[m[2]]
		s.SizeBytes = int64(v * mult)
	} else if r.BehaviorHints.VideoSize > 0 {
		s.SizeBytes = r.BehaviorHints.VideoSize
		s.Size = fmt.Sprintf("%.1f GB", float64(r.BehaviorHints.VideoSize)/1e9)
	}
	s.Cached = strings.Contains(r.Name, "RD+") || strings.Contains(r.Name, "⚡")
	return s
}

func (c *Client) addonStreams(ctx context.Context, source, base, typ, id string) ([]Stream, error) {
	var r struct {
		Streams []rawStream `json:"streams"`
	}
	if err := c.getJSON(ctx, fmt.Sprintf("%s/stream/%s/%s.json", base, typ, url.PathEscape(id)), 30*time.Minute, &r); err != nil {
		return nil, err
	}
	out := make([]Stream, 0, len(r.Streams))
	for _, rs := range r.Streams {
		if rs.URL == "" {
			continue
		}
		out = append(out, normalize(source, rs))
	}
	return out, nil
}

// Streams asks the primary add-on, and the other when it fails, times out
// or has nothing cached on Real-Debrid. Cached streams come first, then by quality
// and size.
func (c *Client) Streams(ctx context.Context, typ, id, primary string) ([]Stream, error) {
	if !validType(typ) {
		return nil, errors.New("type must be movie or series")
	}
	// The chosen add-on first (Torrentio unless Comet was picked), the other as the backup.
	first, second := "torrentio", "comet"
	if primary == "comet" {
		first, second = second, first
	}
	base := map[string]func() string{"torrentio": c.torrentioBase, "comet": c.cometBase}
	tctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	streams, terr := c.addonStreams(tctx, first, base[first](), typ, id)
	cancel()
	anyCached := false
	for _, s := range streams {
		if s.Cached {
			anyCached = true
			break
		}
	}
	if terr != nil || !anyCached {
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		backup, cerr := c.addonStreams(cctx, second, base[second](), typ, id)
		cancel()
		if cerr == nil {
			streams = append(streams, backup...)
		} else if terr != nil {
			return nil, fmt.Errorf("%s: %v; %s: %v", first, terr, second, cerr)
		}
	}
	sortStreams(typ, streams)
	return streams, nil
}

// sortStreams puts the best stream to play first: cached on Real-Debrid, not
// remux-sized, highest quality, HDR10/SDR before Dolby-Vision-only, larger
// (higher bitrate) files first.
func sortStreams(typ string, streams []Stream) {
	rank := map[string]int{"2160p": 5, "1440p": 4, "1080p": 3, "720p": 2, "576p": 1, "480p": 1}
	sort.SliceStable(streams, func(i, j int) bool {
		a, b := streams[i], streams[j]
		if a.Cached != b.Cached {
			return a.Cached
		}
		// Remux-sized files (~100 Mbit/s) stall on Wi-Fi; they stay in the
		// list for picking by hand but are not "best".
		if ba, bb := tooBig(typ, a), tooBig(typ, b); ba != bb {
			return bb
		}
		if rank[a.Quality] != rank[b.Quality] {
			return rank[a.Quality] > rank[b.Quality]
		}
		// Dolby Vision without an HDR10 base layer can show wrong colours in
		// mpv; keep those after the other files of the same quality.
		if da, db := dvOnly(a.HDR), dvOnly(b.HDR); da != db {
			return db
		}
		return a.SizeBytes > b.SizeBytes
	})
}

func tooBig(typ string, s Stream) bool {
	limit := int64(30e9)
	if typ == "series" {
		limit = 10e9
	}
	return s.SizeBytes > limit
}

func dvOnly(hdr string) bool {
	return strings.Contains(hdr, "DV") && !strings.Contains(hdr, "HDR")
}

// ErrNotReady: the add-on answered with a placeholder instead of the file.
var ErrNotReady = errors.New("Real-Debrid is still preparing this file; pick another stream or try again in a minute")

// Resolve follows an add-on's playback link to the final file URL and name.
// Torrentio and Comet answer with a redirect to the Real-Debrid download.
func (c *Client) Resolve(ctx context.Context, s Stream) (finalURL, filename string, err error) {
	client := &http.Client{Timeout: 90 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Range", "bytes=0-0")
	req.Header.Set("User-Agent", "media-player")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("resolving the stream: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("resolving the stream: %s", resp.Status)
	}
	finalURL = resp.Request.URL.String()
	// With Real-Debrid the add-ons redirect to the file on an RD download
	// host. Anything else is a placeholder clip (Torrentio's
	// /videos/downloading_v3.mp4 while RD re-fetches a "cached" torrent).
	if c.RDKey() != "" && !strings.HasSuffix(resp.Request.URL.Hostname(), "real-debrid.com") {
		return "", "", ErrNotReady
	}
	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "text/") || strings.Contains(ct, "json") {
		return "", "", errors.New("the add-on returned an error page instead of a video (the torrent may not be ready on Real-Debrid yet)")
	}
	filename = s.Filename
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if m := regexp.MustCompile(`filename\*?=(?:UTF-8'')?"?([^";]+)`).FindStringSubmatch(cd); m != nil {
			if f, err := url.QueryUnescape(m[1]); err == nil {
				filename = f
			}
		}
	}
	if filename == "" {
		if f, err := url.PathUnescape(finalURL[strings.LastIndex(finalURL, "/")+1:]); err == nil {
			filename = f
		}
	}
	return finalURL, filename, nil
}

// ---- IDs ---------------------------------------------------------------

// SyntheticID is the negative media ID a title uses in Seanime's watch history.
func SyntheticID(imdbID string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(imdbID))
	return -int(h.Sum32()&0x3fffffff) - 1
}

// EpisodeNumber encodes season and episode in one number for the watch history.
func EpisodeNumber(season, episode int) int {
	if season <= 0 && episode <= 0 {
		return 1
	}
	return season*1000 + episode
}

// SplitVideoID splits "tt1234567:1:3" into the title ID, season and episode.
func SplitVideoID(id string) (titleID string, season, episode int) {
	parts := strings.Split(id, ":")
	titleID = parts[0]
	if len(parts) >= 3 {
		season, _ = strconv.Atoi(parts[1])
		episode, _ = strconv.Atoi(parts[2])
	}
	return
}
