package directstream

import (
	"context"
	"fmt"
	"net/http"
	"seanime/internal/api/anilist"
	"seanime/internal/library/anime"
	"seanime/internal/mkvparser"
	"seanime/internal/nativeplayer"
	"seanime/internal/util/result"
)

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// URL Stream
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

var _ Stream = (*URLStream)(nil)

// URLStream is an HTTP-proxied stream sourced from an arbitrary URL (e.g. from a plugin).
type URLStream struct {
	httpBaseStream
}

func (s *URLStream) Type() nativeplayer.StreamType {
	return nativeplayer.StreamTypeURL
}

func (s *URLStream) LoadPlaybackInfo() (*nativeplayer.PlaybackInfo, error) {
	return s.httpBaseStream.loadPlaybackInfo(s.Type())
}

func (s *URLStream) GetStreamHandler() http.Handler {
	return s.httpBaseStream.getStreamHandler(s)
}

func (s *URLStream) GetAttachmentByName(filename string) (*mkvparser.AttachmentInfo, bool) {
	return getAttachmentByName(s.manager.playbackCtx, s, filename)
}

type PlayURLStreamOptions struct {
	StreamUrl    string
	AnidbEpisode string
	Media        *anilist.BaseAnime
	ClientId     string
}

// PlayURLStream starts built-in player playback for an arbitrary HTTP URL with progress tracking.
func (m *Manager) PlayURLStream(ctx context.Context, opts PlayURLStreamOptions) error {
	m.ResetOpenState(opts.ClientId)

	episodeCollection, err := anime.NewEpisodeCollection(anime.NewEpisodeCollectionOptions{
		AnimeMetadata:       nil,
		Media:               opts.Media,
		MetadataProviderRef: m.metadataProviderRef,
		Logger:              m.Logger,
	})
	if err != nil {
		return fmt.Errorf("cannot play URL stream, could not create episode collection: %w", err)
	}

	episode, ok := episodeCollection.FindEpisodeByAniDB(opts.AnidbEpisode)
	if !ok {
		return fmt.Errorf("cannot play URL stream, could not find episode: %s", opts.AnidbEpisode)
	}

	stream := &URLStream{
		httpBaseStream: httpBaseStream{
			streamUrl: opts.StreamUrl,
			filepath:  "",
			BaseStream: BaseStream{
				manager:               m,
				logger:                m.Logger,
				clientId:              opts.ClientId,
				media:                 opts.Media,
				episode:               episode,
				episodeCollection:     episodeCollection,
				subtitleEventCache:    result.NewMap[string, *mkvparser.SubtitleEvent](),
				activeSubtitleStreams: result.NewMap[string, *SubtitleStream](),
			},
		},
	}

	go m.loadStream(stream)

	return nil
}
