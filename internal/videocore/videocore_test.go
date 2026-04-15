package videocore

import (
	"testing"
	"time"

	"seanime/internal/events"
	"seanime/internal/util"

	"github.com/stretchr/testify/require"
)

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
