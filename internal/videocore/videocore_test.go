package videocore

import (
	"encoding/json"
	"testing"
	"time"

	"seanime/internal/events"
	"seanime/internal/library/anime"
	"seanime/internal/util"

	"github.com/stretchr/testify/require"
)

type recordedWSEvent struct {
	clientId  string
	eventType string
	payload   interface{}
}

type recordingWSEventManager struct {
	videoCoreSubscriber *events.ClientEventSubscriber
	sent                []recordedWSEvent
}

func newRecordingWSEventManager() *recordingWSEventManager {
	return &recordingWSEventManager{
		videoCoreSubscriber: &events.ClientEventSubscriber{Channel: make(chan *events.WebsocketClientEvent, 1)},
	}
}

func (m *recordingWSEventManager) SendEvent(string, interface{}) {}

func (m *recordingWSEventManager) SendEventTo(clientId string, eventType string, payload interface{}, _ ...bool) {
	m.sent = append(m.sent, recordedWSEvent{clientId: clientId, eventType: eventType, payload: payload})
}

func (m *recordingWSEventManager) SubscribeToClientEvents(string) *events.ClientEventSubscriber {
	return &events.ClientEventSubscriber{Channel: make(chan *events.WebsocketClientEvent, 1)}
}

func (m *recordingWSEventManager) SubscribeToClientNativePlayerEvents(string) *events.ClientEventSubscriber {
	return &events.ClientEventSubscriber{Channel: make(chan *events.WebsocketClientEvent, 1)}
}

func (m *recordingWSEventManager) SubscribeToClientVideoCoreEvents(string) *events.ClientEventSubscriber {
	return m.videoCoreSubscriber
}

func (m *recordingWSEventManager) SubscribeToClientNakamaEvents(string) *events.ClientEventSubscriber {
	return &events.ClientEventSubscriber{Channel: make(chan *events.WebsocketClientEvent, 1)}
}

func (m *recordingWSEventManager) SubscribeToClientPlaylistEvents(string) *events.ClientEventSubscriber {
	return &events.ClientEventSubscriber{Channel: make(chan *events.WebsocketClientEvent, 1)}
}

func (m *recordingWSEventManager) UnsubscribeFromClientEvents(string) {}

func (m *recordingWSEventManager) MockSendVideoCoreEvent(event ClientEvent) {
	m.videoCoreSubscriber.Channel <- &events.WebsocketClientEvent{
		ClientID: event.ClientId,
		Type:     events.VideoCoreEventType,
		Payload:  event,
	}
}

func decodeVideoCoreEnvelope(t *testing.T, payload interface{}) map[string]interface{} {
	t.Helper()

	marshaled, err := json.Marshal(payload)
	require.NoError(t, err)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(marshaled, &decoded))

	return decoded
}

func mustMarshalRaw(t *testing.T, payload interface{}) json.RawMessage {
	t.Helper()

	marshaled, err := json.Marshal(payload)
	require.NoError(t, err)

	return marshaled
}

func newPlaybackState(playbackID string) *PlaybackState {
	return &PlaybackState{
		ClientId:   "player-client",
		PlayerType: WebPlayer,
		PlaybackInfo: &VideoPlaybackInfo{
			Id:           playbackID,
			PlaybackType: PlaybackTypeOnlinestream,
			Episode:      &anime.Episode{},
		},
	}
}

func TestVideoTerminatedEventUsesPayloadClientIDWithoutPlaybackState(t *testing.T) {
	logger := util.NewLogger()
	ws := events.NewMockWSEventManager(logger)
	vc := New(NewVideoCoreOptions{
		WsEventManager: ws,
		Logger:         logger,
	})
	sub := vc.Subscribe("test")
	t.Cleanup(func() {
		vc.Unsubscribe("test")
		vc.Shutdown()
	})

	ws.MockSendClientEvent(&events.WebsocketClientEvent{
		ClientID: "socket-client",
		Type:     events.VideoCoreEventType,
		Payload: ClientEvent{
			ClientId: "player-client",
			Type:     PlayerEventVideoTerminated,
		},
	})

	select {
	case rawEvent := <-sub.Events():
		event, ok := rawEvent.(*VideoTerminatedEvent)
		require.True(t, ok)
		require.Equal(t, "player-client", event.GetClientId())
		require.Equal(t, NativePlayer, event.GetPlayerType())
	case <-time.After(time.Second):
		t.Fatal("expected terminated event")
	}
}

func TestSetSkipDataSendsSanitizedOverride(t *testing.T) {
	logger := util.NewLogger()
	ws := newRecordingWSEventManager()
	vc := New(NewVideoCoreOptions{
		WsEventManager: ws,
		Logger:         logger,
	})

	t.Cleanup(vc.Shutdown)

	vc.setPlaybackState(newPlaybackState("playback-1"))
	vc.SetSkipData(&SkipData{
		Op: &SkipDataEntry{Interval: SkipInterval{StartTime: 12, EndTime: 42}},
		Ed: &SkipDataEntry{Interval: SkipInterval{StartTime: 20, EndTime: 60}},
	})

	// overlapping ed ranges should be dropped before they reach the player.
	require.Len(t, ws.sent, 1)
	require.Equal(t, "player-client", ws.sent[0].clientId)
	require.Equal(t, string(events.VideoCoreEventType), ws.sent[0].eventType)

	envelope := decodeVideoCoreEnvelope(t, ws.sent[0].payload)
	require.Equal(t, string(ServerEventSetSkipData), envelope["type"])

	sentSkipData, ok := envelope["payload"].(map[string]interface{})
	require.True(t, ok)
	require.NotNil(t, sentSkipData["op"])
	require.Nil(t, sentSkipData["ed"])
}

func TestSetSkipDataKeepsExplicitEmptyOverride(t *testing.T) {
	logger := util.NewLogger()
	ws := newRecordingWSEventManager()
	vc := New(NewVideoCoreOptions{
		WsEventManager: ws,
		Logger:         logger,
	})

	t.Cleanup(vc.Shutdown)

	vc.setPlaybackState(newPlaybackState("playback-1"))
	vc.SetSkipData(&SkipData{})

	// an empty override should stay distinct from clearing so plugins can disable AniSkip fallback.
	require.Len(t, ws.sent, 1)
	envelope := decodeVideoCoreEnvelope(t, ws.sent[0].payload)
	require.Equal(t, string(ServerEventSetSkipData), envelope["type"])
	require.NotNil(t, envelope["payload"])
}

func TestClearSkipDataSendsNilOverride(t *testing.T) {
	logger := util.NewLogger()
	ws := newRecordingWSEventManager()
	vc := New(NewVideoCoreOptions{
		WsEventManager: ws,
		Logger:         logger,
	})

	t.Cleanup(vc.Shutdown)

	vc.setPlaybackState(newPlaybackState("playback-1"))
	vc.SetSkipData(&SkipData{Op: &SkipDataEntry{Interval: SkipInterval{StartTime: 12, EndTime: 42}}})
	ws.sent = nil

	vc.ClearSkipData()

	require.Len(t, ws.sent, 1)

	envelope := decodeVideoCoreEnvelope(t, ws.sent[0].payload)
	require.Equal(t, string(ServerEventSetSkipData), envelope["type"])
	require.Nil(t, envelope["payload"])
}

func TestGetSkipDataReturnsClientOwnedState(t *testing.T) {
	logger := util.NewLogger()
	ws := newRecordingWSEventManager()
	vc := New(NewVideoCoreOptions{
		WsEventManager: ws,
		Logger:         logger,
	})

	t.Cleanup(vc.Shutdown)

	vc.setPlaybackState(newPlaybackState("playback-1"))
	type result struct {
		skipData *SkipData
		ok       bool
	}
	resultCh := make(chan result, 1)

	go func() {
		skipData, ok := vc.GetSkipData()
		resultCh <- result{skipData: skipData, ok: ok}
	}()

	require.Eventually(t, func() bool {
		return len(ws.sent) == 1
	}, time.Second, 10*time.Millisecond)

	envelope := decodeVideoCoreEnvelope(t, ws.sent[0].payload)
	require.Equal(t, string(ServerEventGetSkipData), envelope["type"])
	require.Nil(t, envelope["payload"])

	ws.MockSendVideoCoreEvent(ClientEvent{
		ClientId: "player-client",
		Type:     PlayerEventVideoSkipData,
		Payload: mustMarshalRaw(t, clientVideoSkipDataPayload{SkipData: &SkipData{
			Op: &SkipDataEntry{Interval: SkipInterval{StartTime: 12, EndTime: 42}},
		}}),
	})

	select {
	case ret := <-resultCh:
		require.True(t, ret.ok)
		require.NotNil(t, ret.skipData)
		require.NotNil(t, ret.skipData.Op)
		require.Equal(t, 12.0, ret.skipData.Op.Interval.StartTime)
	case <-time.After(time.Second):
		t.Fatal("expected skip data result")
	}
}

func TestGetSkipDataAllowsEmptyClientState(t *testing.T) {
	logger := util.NewLogger()
	ws := newRecordingWSEventManager()
	vc := New(NewVideoCoreOptions{
		WsEventManager: ws,
		Logger:         logger,
	})

	t.Cleanup(vc.Shutdown)

	vc.setPlaybackState(newPlaybackState("playback-1"))
	type result struct {
		skipData *SkipData
		ok       bool
	}
	resultCh := make(chan result, 1)

	go func() {
		skipData, ok := vc.GetSkipData()
		resultCh <- result{skipData: skipData, ok: ok}
	}()

	require.Eventually(t, func() bool {
		return len(ws.sent) == 1
	}, time.Second, 10*time.Millisecond)

	ws.MockSendVideoCoreEvent(ClientEvent{
		ClientId: "player-client",
		Type:     PlayerEventVideoSkipData,
		Payload:  mustMarshalRaw(t, clientVideoSkipDataPayload{}),
	})

	select {
	case ret := <-resultCh:
		require.True(t, ret.ok)
		require.Nil(t, ret.skipData)
	case <-time.After(time.Second):
		t.Fatal("expected skip data result")
	}
}
