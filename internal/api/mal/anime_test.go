package mal

import (
	"seanime/internal/testutil"
	"seanime/internal/util"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestGetAnimeDetails(t *testing.T) {
	testutil.InitTestProvider(t, testutil.MyAnimeList())

	malWrapper := NewWrapper(testutil.ConfigData.Provider.MalJwt, util.NewLogger())

	res, err := malWrapper.GetAnimeDetails(51179)

	spew.Dump(res)

	if err != nil {
		t.Fatalf("error while fetching media, %v", err)
	}

	t.Log(res.Title)
}

func TestGetAnimeCollection(t *testing.T) {
	testutil.InitTestProvider(t, testutil.MyAnimeList())

	malWrapper := NewWrapper(testutil.ConfigData.Provider.MalJwt, util.NewLogger())

	res, err := malWrapper.GetAnimeCollection()

	if err != nil {
		t.Fatalf("error while fetching anime collection, %v", err)
	}

	for _, entry := range res {
		t.Log(entry.Node.Title)
		if entry.Node.ID == 51179 {
			spew.Dump(entry)
		}
	}
}

func TestUpdateAnimeListStatus(t *testing.T) {
	testutil.InitTestProvider(t, testutil.MyAnimeList(), testutil.MyAnimeListMutation())

	malWrapper := NewWrapper(testutil.ConfigData.Provider.MalJwt, util.NewLogger())

	mId := 51179
	progress := 2
	status := MediaListStatusWatching

	err := malWrapper.UpdateAnimeListStatus(&AnimeListStatusParams{
		Status:             &status,
		NumEpisodesWatched: &progress,
	}, mId)

	if err != nil {
		t.Fatalf("error while fetching media, %v", err)
	}
}
