package stremio

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckEpisodeFile(t *testing.T) {
	cases := []struct {
		file    string
		s, e    int
		wantErr bool
	}{
		{"Frieren Beyond Journey's End - S01E01 - The Journey's End.mkv", 1, 1, false},
		{"Show.S02E05.1080p.WEB.mkv", 2, 6, true},  // explicit other episode
		{"Show.S03E05.1080p.WEB.mkv", 2, 5, true},  // explicit other season
		{"Show S01E05-E06 1080p.mkv", 1, 6, false}, // multi-episode file
		{"[aL] Sousou no Frieren (2023) - 01 (WEB 2160p).mkv", 1, 1, false},
		{"[SubsPlease] Jujutsu Kaisen - 25 (1080p).mkv", 2, 1, false}, // absolute numbering: trusted
		{"The.Movie.2019.2160p.mkv", 1, 1, false},
		{"", 1, 1, false},
	}
	for _, c := range cases {
		err := checkEpisodeFile(c.file, c.s, c.e)
		if (err != nil) != c.wantErr {
			t.Errorf("%q S%dE%d: err=%v, want error %v", c.file, c.s, c.e, err, c.wantErr)
		}
	}
}

func TestNormalize(t *testing.T) {
	s := normalize("torrentio", rawStream{
		Name:  "[RD+] Torrentio\n4k DV | HDR",
		Title: "The Shawshank Redemption 1994 2160p\n👤 12 💾 58.3 GB ⚙️ RARBG",
		URL:   "https://torrentio.strem.fun/resolve/realdebrid/KEY/hash/null/0/file.mkv",
	})
	if !s.Cached || s.Quality != "2160p" || s.HDR != "DV HDR" || s.Size != "58.3 GB" || s.SizeBytes < 58e9 {
		t.Fatalf("torrentio stream parsed as %+v", s)
	}
	c := normalize("comet", rawStream{
		Name:        "[RD⚡] Comet 1080p",
		Description: "📄 Show.S01E02.1080p.WEB.mkv\n💾 1.4 GB",
		URL:         "https://comet.elfhosted.com/playback/x",
	})
	if !c.Cached || c.Quality != "1080p" || c.Filename != "Show.S01E02.1080p.WEB.mkv" {
		t.Fatalf("comet stream parsed as %+v", c)
	}
}

func TestSyntheticIDs(t *testing.T) {
	a, b := SyntheticID("tt0111161"), SyntheticID("tt22248376")
	if a >= 0 || b >= 0 || a == b {
		t.Fatalf("synthetic ids %d %d", a, b)
	}
	if EpisodeNumber(2, 5) != 2005 || EpisodeNumber(0, 0) != 1 {
		t.Fatal("episode numbers")
	}
	id, s, e := SplitVideoID("tt22248376:1:3")
	if id != "tt22248376" || s != 1 || e != 3 {
		t.Fatal("split")
	}
}

func TestStreamOrder(t *testing.T) {
	streams := []Stream{
		{URL: "remux", Cached: true, Quality: "2160p", SizeBytes: 109e9},
		{URL: "uncached", Cached: false, Quality: "2160p", SizeBytes: 20e9},
		{URL: "dv", Cached: true, Quality: "2160p", HDR: "DV", SizeBytes: 25e9},
		{URL: "web", Cached: true, Quality: "2160p", HDR: "HDR", SizeBytes: 18e9},
		{URL: "1080", Cached: true, Quality: "1080p", SizeBytes: 12e9},
	}
	sortStreams("movie", streams)
	var got []string
	for _, s := range streams {
		got = append(got, s.URL)
	}
	want := "web dv 1080 remux uncached"
	if strings.Join(got, " ") != want {
		t.Fatalf("order %v, want %s", got, want)
	}
}

func TestResolvePlaceholder(t *testing.T) {
	// An add-on that redirects to its "downloading" clip instead of the file.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/videos/downloading_v3.mp4" {
			w.Header().Set("Content-Type", "video/mp4")
			w.Write([]byte{0})
			return
		}
		http.Redirect(w, r, "/videos/downloading_v3.mp4", http.StatusFound)
	}))
	defer srv.Close()
	c := NewClient(func() string { return "key" })
	if _, _, err := c.Resolve(context.Background(), Stream{URL: srv.URL + "/resolve/x"}); !errors.Is(err, ErrNotReady) {
		t.Fatalf("placeholder resolved with err=%v, want ErrNotReady", err)
	}
}
