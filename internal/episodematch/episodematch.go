// Package episodematch picks the file for one episode out of a torrent's (or
// debrid torrent's) file list.
//
// It replaces upstream's "first file whose AniDB episode equals the episode
// number" lookup, which plays the wrong file when several files claim the same
// number: the same episode of another season in a multi-season pack, a special
// ("Mini Anime - 01") next to episode 1, or the original next to its v2.
//
// The decision only uses the files' names and folders and the entry being
// watched (its season and how many episodes came before it). It never depends
// on the order of the files, and when it cannot tell, it says so (ErrUnsure)
// so the caller can show the file picker instead of guessing.
package episodematch

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"seanime/internal/library/anime"
)

var (
	// ErrNotFound means no file matches the episode.
	ErrNotFound = errors.New("episodematch: no file matches the episode")
	// ErrUnsure means several files match equally well.
	ErrUnsure = errors.New("episodematch: several files match the episode equally well")
)

// Target is the episode being looked for.
type Target struct {
	Episode int // episode number relative to the entry (what Seanime passes around)
	// Season is the entry's season among the show's TV seasons (1 for the
	// first), from its prequel chain; 0 if unknown.
	Season int
	// Offset is how many episodes the show's earlier TV seasons have, so that
	// absolute numbering (S2E01 named "- 25") can be recognised; 0 if none.
	Offset int
	// EpisodeCount is the entry's episode count; 0 if unknown.
	EpisodeCount int
	// SpecialEntry is true when the entry itself is an OVA, ONA, special or
	// movie, so files in "Specials" folders are not thrown out.
	SpecialEntry bool
}

// Candidate is one file of the torrent.
type Candidate struct {
	Index     int              // the file's index in the torrent or debrid file list
	Path      string           // its path inside the torrent
	Size      int64            // its size in bytes, 0 if unknown
	LocalFile *anime.LocalFile // parsed name; built from Path when nil
}

// Choice is the file picked and why.
type Choice struct {
	Candidate
	Reason string
}

var videoExt = map[string]bool{
	".mkv": true, ".mp4": true, ".m4v": true, ".avi": true, ".webm": true,
	".mov": true, ".ts": true, ".m2ts": true, ".wmv": true, ".flv": true,
}

// Folders whose contents are never regular episodes.
var extraFolder = regexp.MustCompile(`(?i)^(?:extras?|bonus(?:es)?|specials?|sps?|ncs?|nc ?op(?:s)?|nc ?ed(?:s)?|creditless|menus?|pvs?|cms?|trailers?|previews?|scans|fonts|subs?|subtitles|ovas?|oads?|onas?|featurettes?|interviews?)$`)

// Words in a file name that mark it as something other than an episode.
// Only strong markers: episode titles can contain words like "Special" or
// "Ending", and openings and endings are already typed as NC by the parser.
var extraWord = regexp.MustCompile(`(?i)\b(?:nc ?op\d*|nc ?ed\d*|creditless|mini anime|menu\d*|pv\d*|cm\d*|trailer|teaser|ova|oad|ona|sp ?\d+|omake|picture drama)\b`)

var (
	versionRe = regexp.MustCompile(`(?i)(?:\d|\b)v([2-9])\b`)
	multiEpRe = regexp.MustCompile(`(?i)E(\d{1,4})\s*-\s*E?(\d{1,4})\b`)
)

func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimLeft(strings.TrimSpace(s), "0"))
	if err != nil {
		if strings.Trim(s, "0") == "" && s != "" {
			return 0
		}
		return -1
	}
	return n
}

// parsed is what the matcher reads from one candidate.
type parsed struct {
	c        Candidate
	season   int // 0 when the name and folders don't say
	first    int // episode number in the name (-1 if none)
	last     int // last episode of a multi-episode file (== first otherwise)
	version  int // 1 unless the name says v2, v3...
	extra    bool
	extraWhy string
}

func parse(c Candidate, special bool) parsed {
	lf := c.LocalFile
	if lf == nil {
		lf = anime.NewLocalFile(c.Path, "")
	}
	p := parsed{c: c, first: -1, version: 1}
	p.c.LocalFile = lf
	name := path.Base(strings.ReplaceAll(c.Path, "\\", "/"))

	if lf.ParsedData != nil {
		if s := atoi(lf.ParsedData.Season); s > 0 {
			p.season = s
		}
		p.first = atoi(lf.ParsedData.Episode)
	}
	// The nearest folder that names one season, if the file itself doesn't.
	if p.season == 0 {
		for i := len(lf.ParsedFolderData) - 1; i >= 0; i-- {
			if s := atoi(lf.ParsedFolderData[i].Season); s > 0 {
				p.season = s
				break
			}
		}
	}
	p.last = p.first
	if m := multiEpRe.FindStringSubmatch(name); m != nil {
		if a, b := atoi(m[1]), atoi(m[2]); a >= 0 && b >= a {
			p.first, p.last = a, b
		}
	}
	if m := versionRe.FindStringSubmatch(name); m != nil {
		p.version = atoi(m[1])
	}

	switch {
	case !videoExt[strings.ToLower(path.Ext(name))]:
		p.extra, p.extraWhy = true, "not a video file"
	case lf.Metadata != nil && lf.Metadata.Type == anime.LocalFileTypeNC:
		p.extra, p.extraWhy = true, "opening or ending"
	case !special:
		dirs := strings.Split(path.Dir(strings.ReplaceAll(c.Path, "\\", "/")), "/")
		for _, d := range dirs {
			if extraFolder.MatchString(strings.TrimSpace(d)) {
				p.extra, p.extraWhy = true, fmt.Sprintf("in the %q folder", d)
				break
			}
		}
		if !p.extra {
			// Only the part of the name after the show's title, so titles
			// that contain words like "Ending" are not thrown out.
			rest := name
			if lf.ParsedData != nil && lf.ParsedData.Title != "" {
				if i := strings.Index(rest, lf.ParsedData.Title); i >= 0 {
					rest = rest[i+len(lf.ParsedData.Title):]
				}
				// "Show - Mini Anime" parsed as the title itself.
				if j := strings.LastIndex(lf.ParsedData.Title, " - "); j >= 0 {
					rest = lf.ParsedData.Title[j:] + " " + rest
				}
			}
			if w := extraWord.FindString(rest); w != "" {
				p.extra, p.extraWhy = true, fmt.Sprintf("named %q", w)
			}
		}
	}
	return p
}

// Pick returns the candidate that holds t.Episode.
func Pick(cands []Candidate, t Target) (*Choice, error) {
	if t.Episode <= 0 {
		return nil, fmt.Errorf("episodematch: invalid episode %d", t.Episode)
	}
	all := make([]parsed, 0, len(cands))
	for _, c := range cands {
		all = append(all, parse(c, t.SpecialEntry))
	}
	var eps []parsed
	for _, p := range all {
		if !p.extra && p.first >= 0 {
			eps = append(eps, p)
		}
	}
	if len(eps) == 0 {
		return nil, ErrNotFound
	}

	// Which season to accept. A file of another season never matches; a file
	// with no season matches only if the pack doesn't mix seasons.
	seasons := map[int]bool{}
	for _, p := range eps {
		if p.season > 0 {
			seasons[p.season] = true
		}
	}
	want := t.Season
	if want == 0 && len(seasons) == 1 {
		for s := range seasons {
			want = s
		}
	}
	if want == 0 && len(seasons) > 1 {
		return nil, fmt.Errorf("%w: the torrent has several seasons and the entry's season is unknown", ErrUnsure)
	}
	// A pack that names a different season than the entry's (AniList splits a
	// season the release doesn't, or the reverse) is accepted when it is the
	// only season in the torrent.
	switched := false
	if want > 0 && !seasons[want] && len(seasons) == 1 {
		for s := range seasons {
			want, switched = s, true
		}
	}
	if switched && t.EpisodeCount > 0 {
		// The release's season is longer than the entry: the entry is one
		// part of it, and where that part starts can't be told from here.
		for _, p := range eps {
			if p.season == want && p.last > t.EpisodeCount {
				return nil, fmt.Errorf("%w: the release numbers seasons differently than AniList", ErrUnsure)
			}
		}
	}

	// Absolute numbering: numbers in this season's files run past the entry's
	// episode count, so "- 25" is the entry's episode 1.
	absolute := false
	if t.Offset > 0 {
		for _, p := range eps {
			if (p.season == 0 || p.season == want) && t.EpisodeCount > 0 && p.last > t.EpisodeCount {
				absolute = true
				break
			}
		}
	}

	type hit struct {
		parsed
		how   string
		score int
	}
	var hits []hit
	for _, p := range eps {
		if p.season != 0 && want != 0 && p.season != want {
			continue
		}
		e := t.Episode
		if absolute {
			e += t.Offset
		}
		if e < p.first || e > p.last {
			continue
		}
		h := hit{parsed: p, how: "episode number"}
		if absolute {
			h.how = fmt.Sprintf("absolute number %d (%d earlier episodes)", e, t.Offset)
		}
		if p.season != 0 {
			h.score += 2 // the name or folder confirms the season
			h.how += fmt.Sprintf(", season %d", p.season)
		}
		if p.first == p.last {
			h.score++ // a file of only this episode beats a multi-episode file
		}
		hits = append(hits, h)
	}
	if len(hits) == 0 {
		return nil, ErrNotFound
	}

	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.version != b.version {
			return a.version > b.version
		}
		return a.c.Size > b.c.Size
	})
	best := hits[0]
	if len(hits) > 1 {
		next := hits[1]
		if next.score == best.score && next.version == best.version &&
			(best.c.Size == 0 || next.c.Size == best.c.Size) {
			var names []string
			for _, h := range hits {
				names = append(names, h.c.Path)
			}
			return nil, fmt.Errorf("%w: %s", ErrUnsure, strings.Join(names, ", "))
		}
	}
	why := "matched by " + best.how
	if best.version > 1 {
		why += fmt.Sprintf(", release v%d", best.version)
	}
	return &Choice{Candidate: best.c, Reason: why}, nil
}
