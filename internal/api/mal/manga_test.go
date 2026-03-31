package mal

import (
	"seanime/internal/testutil"
	"seanime/internal/util"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestGetMangaDetails(t *testing.T) {
	cfg := testutil.InitTestProvider(t, testutil.MyAnimeList())

	malWrapper := NewWrapper(cfg.Provider.MalJwt, util.NewLogger())

	res, err := malWrapper.GetMangaDetails(13)

	spew.Dump(res)

	if err != nil {
		t.Fatalf("error while fetching media, %v", err)
	}

	t.Log(res.Title)
}

func TestGetMangaCollection(t *testing.T) {
	cfg := testutil.InitTestProvider(t, testutil.MyAnimeList())

	malWrapper := NewWrapper(cfg.Provider.MalJwt, util.NewLogger())

	res, err := malWrapper.GetMangaCollection()

	if err != nil {
		t.Fatalf("error while fetching anime collection, %v", err)
	}

	for _, entry := range res {
		t.Log(entry.Node.Title)
		if entry.Node.ID == 13 {
			spew.Dump(entry)
		}
	}
}

func TestUpdateMangaListStatus(t *testing.T) {
	cfg := testutil.InitTestProvider(t, testutil.MyAnimeList(), testutil.MyAnimeListMutation())

	malWrapper := NewWrapper(cfg.Provider.MalJwt, util.NewLogger())

	mId := 13
	progress := 1000
	status := MediaListStatusReading

	err := malWrapper.UpdateMangaListStatus(&MangaListStatusParams{
		Status:          &status,
		NumChaptersRead: &progress,
	}, mId)

	if err != nil {
		t.Fatalf("error while fetching media, %v", err)
	}
}
