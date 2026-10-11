package realdebrid

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/samber/mo"
)

// fakeRD answers /unrestrict/link with the file each link really points to.
func fakeRD(t *testing.T, files map[string]unrestrictLinkResponse) (*RealDebrid, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/unrestrict/link" {
			http.NotFound(w, r)
			return
		}
		calls++
		resp, ok := files[r.FormValue("link")]
		if !ok {
			http.Error(w, `{"error":"unknown link"}`, http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	logger := zerolog.Nop()
	rd := NewRealDebridT(&logger)
	rd.baseUrl = srv.URL
	rd.apiKey = mo.Some("test")
	return rd, &calls
}

func selectedFiles(names ...string) []*TorrentInfoFile {
	out := make([]*TorrentInfoFile, len(names))
	for i, n := range names {
		out[i] = &TorrentInfoFile{ID: i + 1, Path: "/Pack/" + n, Bytes: int64(1000 + i), Selected: 1}
	}
	return out
}

func TestUnrestrictFileLinkInOrder(t *testing.T) {
	rd, calls := fakeRD(t, map[string]unrestrictLinkResponse{
		"L1": {Filename: "Show - 01.mkv", Filesize: 1000, Download: "https://dl/01"},
		"L2": {Filename: "Show - 02.mkv", Filesize: 1001, Download: "https://dl/02"},
	})
	got, err := rd.unrestrictFileLink([]string{"L1", "L2"}, selectedFiles("Show - 01.mkv", "Show - 02.mkv"), 1)
	if err != nil || got.Download != "https://dl/02" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if *calls != 1 {
		t.Fatalf("%d unrestrict calls, want 1 when the order is right", *calls)
	}
}

// Links in a different order than the files: upstream would play episode 3.
func TestUnrestrictFileLinkOutOfOrder(t *testing.T) {
	rd, _ := fakeRD(t, map[string]unrestrictLinkResponse{
		"L1": {Filename: "Show - 01.mkv", Filesize: 1000, Download: "https://dl/01"},
		"L2": {Filename: "Show - 03.mkv", Filesize: 1002, Download: "https://dl/03"},
		"L3": {Filename: "Show - 02.mkv", Filesize: 1001, Download: "https://dl/02"},
	})
	files := selectedFiles("Show - 01.mkv", "Show - 02.mkv", "Show - 03.mkv")
	got, err := rd.unrestrictFileLink([]string{"L1", "L2", "L3"}, files, 1)
	if err != nil || got.Download != "https://dl/02" {
		t.Fatalf("got %+v, %v; want episode 02", got, err)
	}
}

// RD sometimes packs a torrent into one archive: no link is the episode.
func TestUnrestrictFileLinkArchive(t *testing.T) {
	rd, _ := fakeRD(t, map[string]unrestrictLinkResponse{
		"L1": {Filename: "Pack.rar", Filesize: 99999, Download: "https://dl/rar"},
	})
	_, err := rd.unrestrictFileLink([]string{"L1"}, selectedFiles("Show - 01.mkv", "Show - 02.mkv"), 1)
	if err == nil {
		t.Fatal("got a link for a file that is only inside an archive")
	}
}

// A renamed link still matches by its exact size.
func TestUnrestrictFileLinkMatchesBySize(t *testing.T) {
	rd, _ := fakeRD(t, map[string]unrestrictLinkResponse{
		"L1": {Filename: "Show_-_01.mkv", Filesize: 1000, Download: "https://dl/01"},
	})
	got, err := rd.unrestrictFileLink([]string{"L1"}, selectedFiles("Show - 01.mkv"), 0)
	if err != nil || got.Download != "https://dl/01" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestUnrestrictFileLinkFirstCallFails(t *testing.T) {
	rd, _ := fakeRD(t, map[string]unrestrictLinkResponse{})
	_, err := rd.unrestrictFileLink([]string{"L1"}, selectedFiles("Show - 01.mkv"), 0)
	if err == nil {
		t.Fatal("want an error when RD refuses the link")
	}
}
