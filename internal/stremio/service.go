package stremio

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"seanime/internal/api/anilist"
	"seanime/internal/continuity"
	"seanime/internal/episodematch"
	"seanime/internal/library/anime"

	"github.com/rs/zerolog"
)

// Player starts built-in playback of a URL with the given title and episode.
type Player interface {
	PlayMediaUrlStream(ctx context.Context, streamURL, clientID string, media *anilist.BaseAnime, episode *anime.Episode) error
}

// Service ties the add-ons, the library and the player together.
type Service struct {
	Client     *Client
	Library    *Library
	Player     Player
	Continuity *continuity.Manager
	Logger     *zerolog.Logger
}

// PlayRequest is a stream the page chose (or the next episode's best stream).
type PlayRequest struct {
	Type     string `json:"type"`
	VideoID  string `json:"videoId"` // movie ID or "tt..:s:e"
	Stream   Stream `json:"stream"`
	ClientID string `json:"clientId"`
	// Fallback: if this stream isn't ready or is the wrong episode, try the
	// next cached streams ("Play best", autoplay).
	Fallback bool `json:"fallback"`
}

// Play resolves the stream, checks that a series' file is the requested
// episode, remembers the title and starts playback.
func (s *Service) Play(ctx context.Context, req PlayRequest) error {
	titleID, season, episode := SplitVideoID(req.VideoID)
	meta, err := s.Client.Meta(ctx, req.Type, titleID)
	if err != nil {
		return fmt.Errorf("loading the title: %w", err)
	}
	candidates := []Stream{req.Stream}
	if req.Fallback {
		if all, err := s.Client.Streams(ctx, req.Type, req.VideoID); err == nil {
			for _, st := range all {
				if st.Cached && st.URL != req.Stream.URL && len(candidates) < 4 {
					candidates = append(candidates, st)
				}
			}
		}
	}
	var streamURL, filename string
	var firstErr error
	for _, st := range candidates {
		u, name, err := s.Client.Resolve(ctx, st)
		if err == nil && req.Type == "series" {
			err = checkEpisodeFile(name, season, episode)
		}
		if err != nil {
			s.Logger.Warn().Err(err).Str("source", st.Source).Str("filename", st.Filename).Msg("stremio: Stream not playable")
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		streamURL, filename, req.Stream = u, name, st
		break
	}
	if streamURL == "" {
		return firstErr
	}

	var video *Video
	for i := range meta.Videos {
		if meta.Videos[i].ID == req.VideoID || (meta.Videos[i].Season == season && meta.Videos[i].Episode == episode) {
			video = &meta.Videos[i]
			break
		}
	}
	mediaID := SyntheticID(titleID)
	ref := &TitleRef{
		MediaID: mediaID, Type: req.Type, ImdbID: titleID, Name: meta.Name,
		Poster: meta.Poster, Background: meta.Background, VideoID: req.VideoID,
		Season: season, Episode: episode, BingeGroup: req.Stream.BingeGroup, Quality: req.Stream.Quality,
	}
	if video != nil {
		ref.EpisodeTitle = firstNonEmpty(video.Name, video.Title)
		ref.Thumbnail = video.Thumbnail
	}
	s.Library.Remember(ref)

	media, ep := syntheticMedia(meta, ref, len(meta.Videos))
	s.Logger.Info().Str("title", meta.Name).Str("video", req.VideoID).Str("filename", filename).
		Str("source", req.Stream.Source).Msg("stremio: Playing")
	return s.Player.PlayMediaUrlStream(ctx, streamURL, req.ClientID, media, ep)
}

// checkEpisodeFile refuses a file whose name explicitly says another season
// or episode than the one asked for (S02E05 when S02E06 was requested), so a
// wrong mapping in an add-on doesn't play the wrong episode. Names without a
// season (absolute anime numbering, single files) are trusted: the add-on maps
// those itself.
func checkEpisodeFile(filename string, season, episode int) error {
	if filename == "" || episode <= 0 {
		return nil
	}
	lf := anime.NewLocalFile(filename, "")
	if lf.ParsedData == nil {
		return nil
	}
	fs, ferr := strconv.Atoi(strings.TrimLeft(lf.ParsedData.Season, "0"))
	fe, eerr := strconv.Atoi(strings.TrimLeft(lf.ParsedData.Episode, "0"))
	if ferr != nil || eerr != nil || fs <= 0 {
		return nil
	}
	if fs == season && fe == episode {
		return nil
	}
	// A multi-episode file ("S01E05-E06") that contains the episode is fine.
	if fs == season {
		if c, err := episodematch.Pick([]episodematch.Candidate{{Path: filename}},
			episodematch.Target{Episode: episode, Season: season}); err == nil && c != nil {
			return nil
		}
	}
	return fmt.Errorf("the add-on's file is S%02dE%02d, not S%02dE%02d: %s", fs, fe, season, episode, filename)
}

// syntheticMedia dresses a Stremio title as the AniList media and episode
// Seanime's player expects.
func syntheticMedia(meta *Meta, ref *TitleRef, episodes int) (*anilist.BaseAnime, *anime.Episode) {
	name := meta.Name
	poster := meta.Poster
	format := anilist.MediaFormatMovie
	if ref.Type == "series" {
		format = anilist.MediaFormatTv
	}
	eps := 1
	if ref.Type == "series" && episodes > 0 {
		eps = episodes
	}
	media := &anilist.BaseAnime{
		ID:       ref.MediaID,
		Format:   &format,
		Episodes: &eps,
		Title:    &anilist.BaseAnime_Title{English: &name, Romaji: &name, UserPreferred: &name},
		CoverImage: &anilist.BaseAnime_CoverImage{
			Large: &poster, ExtraLarge: &poster, Medium: &poster,
		},
	}
	if meta.Background != "" {
		bg := meta.Background
		media.BannerImage = &bg
	}
	num := EpisodeNumber(ref.Season, ref.Episode)
	display := name
	if ref.Type == "series" {
		display = fmt.Sprintf("S%d E%d", ref.Season, ref.Episode)
	}
	ep := &anime.Episode{
		Type:           anime.LocalFileTypeMain,
		DisplayTitle:   display,
		EpisodeTitle:   ref.EpisodeTitle,
		EpisodeNumber:  num,
		ProgressNumber: num,
		AniDBEpisode:   fmt.Sprint(num),
		EpisodeMetadata: &anime.EpisodeMetadata{
			Image: firstNonEmpty(ref.Thumbnail, meta.Background, poster),
			Title: ref.EpisodeTitle,
		},
		BaseAnime: media,
	}
	return media, ep
}

// ---- Continue Watching and Next Up ---------------------------------------

// ContinueItem is a row in Continue Watching.
type ContinueItem struct {
	Title       *TitleRef `json:"title"`
	VideoID     string    `json:"videoId"`
	Season      int       `json:"season,omitempty"`
	Episode     int       `json:"episode,omitempty"`
	CurrentTime float64   `json:"currentTime"`
	Duration    float64   `json:"duration"`
	NextUp      bool      `json:"nextUp"` // the previous episode was finished; this is the next one
}

// Continue lists titles to resume (5%–90% watched, Jellyfin's rule) and, for
// series whose last episode was finished, the next episode.
func (s *Service) Continue(ctx context.Context) []ContinueItem {
	history := s.Continuity.GetWatchHistory()
	var out []ContinueItem
	for _, t := range s.Library.All() {
		h, ok := history[t.MediaID]
		if !ok || h.Duration <= 0 {
			continue
		}
		ratio := h.CurrentTime / h.Duration
		switch {
		case ratio >= 0.05 && ratio < 0.9:
			out = append(out, ContinueItem{Title: t, VideoID: t.VideoID, Season: t.Season, Episode: t.Episode,
				CurrentTime: h.CurrentTime, Duration: h.Duration})
		case ratio >= 0.9 && t.Type == "series":
			if next, ok := s.nextVideo(ctx, t); ok {
				out = append(out, ContinueItem{Title: t, VideoID: next.ID, Season: next.Season,
					Episode: next.Episode, NextUp: true})
			}
		}
	}
	return out
}

func (s *Service) nextVideo(ctx context.Context, t *TitleRef) (*Video, bool) {
	meta, err := s.Client.Meta(ctx, "series", t.ImdbID)
	if err != nil {
		return nil, false
	}
	for i, v := range meta.Videos {
		if v.Season == t.Season && v.Episode == t.Episode {
			for _, n := range meta.Videos[i+1:] {
				if n.Season > 0 { // skip specials (season 0)
					return &n, true
				}
			}
		}
	}
	return nil, false
}

// PlayNext plays the episode after the one last played for mediaID, with the
// stream from the same release (binge group) when there is one, else the best
// cached stream of the same quality, else the best stream.
func (s *Service) PlayNext(ctx context.Context, mediaID int, clientID string) error {
	t, ok := s.Library.Get(mediaID)
	if !ok || t.Type != "series" {
		return errors.New("no series to continue")
	}
	next, ok := s.nextVideo(ctx, t)
	if !ok {
		return errors.New("that was the last episode")
	}
	streams, err := s.Client.Streams(ctx, "series", next.ID)
	if err != nil {
		return err
	}
	if len(streams) == 0 {
		return fmt.Errorf("no streams for S%02dE%02d", next.Season, next.Episode)
	}
	best := streams[0]
	found := false
	if t.BingeGroup != "" {
		for _, st := range streams {
			if st.BingeGroup == t.BingeGroup {
				best, found = st, true
				break
			}
		}
	}
	if !found && t.Quality != "" {
		for _, st := range streams {
			if st.Cached && st.Quality == t.Quality {
				best = st
				break
			}
		}
	}
	return s.Play(ctx, PlayRequest{Type: "series", VideoID: next.ID, Stream: best, ClientID: clientID, Fallback: true})
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
