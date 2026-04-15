package directstream

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"seanime/internal/api/anilist"
	"seanime/internal/library/anime"
	"testing"
	"time"

	"seanime/internal/events"

	"seanime/internal/mkvparser"
	"seanime/internal/nativeplayer"
	"seanime/internal/util"
	"seanime/internal/util/result"
	"seanime/internal/videocore"

	"github.com/samber/mo"
	"github.com/stretchr/testify/require"
)

type testStream struct {
	BaseStream
	handler http.Handler
}

func (s *testStream) Type() nativeplayer.StreamType {
	return nativeplayer.StreamTypeTorrent
}

func (s *testStream) GetStreamHandler() http.Handler {
	return s.handler
}

func (s *testStream) LoadPlaybackInfo() (*nativeplayer.PlaybackInfo, error) {
	return s.playbackInfo, s.playbackInfoErr
}

type trackingReadSeekCloser struct {
	closed bool
}

type blockingStream struct {
	clientID       string
	loadPlaybackCh chan struct{}
	terminatedCh   chan struct{}
	terminated     bool
}

func (s *blockingStream) Type() nativeplayer.StreamType               { return nativeplayer.StreamTypeTorrent }
func (s *blockingStream) LoadContentType() string                     { return "video/webm" }
func (s *blockingStream) ClientId() string                            { return s.clientID }
func (s *blockingStream) Media() *anilist.BaseAnime                   { return nil }
func (s *blockingStream) Episode() *anime.Episode                     { return nil }
func (s *blockingStream) ListEntryData() *anime.EntryListData         { return nil }
func (s *blockingStream) EpisodeCollection() *anime.EpisodeCollection { return nil }
func (s *blockingStream) LoadPlaybackInfo() (*nativeplayer.PlaybackInfo, error) {
	<-s.loadPlaybackCh
	return &nativeplayer.PlaybackInfo{ID: "blocked"}, nil
}
func (s *blockingStream) GetAttachmentByName(string) (*mkvparser.AttachmentInfo, bool) {
	return nil, false
}
func (s *blockingStream) GetStreamHandler() http.Handler { return http.NewServeMux() }
func (s *blockingStream) StreamError(error)              {}
func (s *blockingStream) Terminate() {
	if s.terminated {
		return
	}
	s.terminated = true
	close(s.terminatedCh)
}
func (s *blockingStream) GetSubtitleEventCache() *result.Map[string, *mkvparser.SubtitleEvent] {
	return result.NewMap[string, *mkvparser.SubtitleEvent]()
}
func (s *blockingStream) OnSubtitleFileUploaded(string, string) {}

func (r *trackingReadSeekCloser) Read(_ []byte) (int, error) {
	return 0, io.EOF
}

func (r *trackingReadSeekCloser) Seek(_ int64, _ int) (int64, error) {
	return 0, nil
}

func (r *trackingReadSeekCloser) Close() error {
	r.closed = true
	return nil
}

func TestGetStreamHandlerRejectsMismatchedPlaybackID(t *testing.T) {
	called := false
	stream := &testStream{
		BaseStream: BaseStream{
			clientId: "client-1",
			playbackInfo: &nativeplayer.PlaybackInfo{
				ID: "expected-playback-id",
			},
		},
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}),
	}

	manager := &Manager{
		currentStream: mo.Some[Stream](stream),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/directstream/stream?id=stale-playback-id", nil)
	rec := httptest.NewRecorder()

	manager.getStreamHandler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.False(t, called)
}

func TestGetStreamHandlerForwardsMatchingPlaybackID(t *testing.T) {
	called := false
	stream := &testStream{
		BaseStream: BaseStream{
			clientId: "client-1",
			playbackInfo: &nativeplayer.PlaybackInfo{
				ID: "playback-id",
			},
		},
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}),
	}

	manager := &Manager{
		currentStream: mo.Some[Stream](stream),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/directstream/stream?id=playback-id", nil)
	rec := httptest.NewRecorder()

	manager.getStreamHandler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.True(t, called)
}

func TestStartSubtitleStreamPClosesReaderWhenParserMissing(t *testing.T) {
	reader := &trackingReadSeekCloser{}
	stream := &BaseStream{
		logger: util.NewLogger(),
		playbackInfo: &nativeplayer.PlaybackInfo{
			MkvMetadataParser: mo.None[*mkvparser.MetadataParser](),
		},
		activeSubtitleStreams: result.NewMap[string, *SubtitleStream](),
	}

	stream.StartSubtitleStreamP(stream, context.Background(), reader, 0, 1024)

	require.True(t, reader.closed)
}

func TestListenToPlayerEventsTerminatesWithoutWaitingForPlaybackInfo(t *testing.T) {
	logger := util.NewLogger()
	ws := events.NewMockWSEventManager(logger)
	vc := videocore.New(videocore.NewVideoCoreOptions{
		WsEventManager: ws,
		Logger:         logger,
	})
	np := nativeplayer.New(nativeplayer.NewNativePlayerOptions{
		WsEventManager: ws,
		Logger:         logger,
		VideoCore:      vc,
	})
	manager := NewManager(NewManagerOptions{
		Logger:         logger,
		WSEventManager: ws,
		NativePlayer:   np,
		VideoCore:      vc,
	})

	stream := &blockingStream{
		clientID:       "player-client",
		loadPlaybackCh: make(chan struct{}),
		terminatedCh:   make(chan struct{}),
	}
	manager.currentStream = mo.Some[Stream](stream)

	t.Cleanup(func() {
		close(stream.loadPlaybackCh)
		vc.Shutdown()
	})

	ws.MockSendClientEvent(&events.WebsocketClientEvent{
		ClientID: "socket-client",
		Type:     events.VideoCoreEventType,
		Payload: videocore.ClientEvent{
			ClientId: "player-client",
			Type:     videocore.PlayerEventVideoTerminated,
		},
	})

	select {
	case <-stream.terminatedCh:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected terminate to bypass playback info loading")
	}
}
