package stremio

import (
	"context"
	"os"
	"strings"
	"testing"
)

// go test -run Live ./internal/stremio/ with MP_RD_KEY_FILE set; skipped otherwise.
func TestLiveResolve(t *testing.T) {
	f := os.Getenv("MP_RD_KEY_FILE")
	if f == "" {
		t.Skip("MP_RD_KEY_FILE not set")
	}
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	key := strings.TrimSpace(string(b))
	c := NewClient(func() string { return key })
	streams, err := c.Streams(context.Background(), "series", "tt10986410:1:2")
	if err != nil || len(streams) == 0 {
		t.Fatal(err, len(streams))
	}
	u, name, err := c.Resolve(context.Background(), streams[0])
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("hint=%q resolved=%q host=%s", streams[0].Filename, name, u[:strings.Index(u[8:], "/")+8])
	if err := checkEpisodeFile(name, 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := checkEpisodeFile(name, 1, 3); err == nil {
		t.Fatal("S01E02 file accepted for S01E03")
	}
}
