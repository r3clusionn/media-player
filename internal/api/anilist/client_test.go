package anilist

import (
	"context"
	"seanime/internal/util"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
)

func TestGetAnimeById(t *testing.T) {
	anilistClient := NewTestAnilistClient()

	tests := []struct {
		name    string
		mediaId int
	}{
		{
			name:    "Re:Zero",
			mediaId: 21355,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := anilistClient.BaseAnimeByID(context.Background(), &tt.mediaId)
			assert.NoError(t, err)
			assert.NotNil(t, res)
		})
	}
}

func TestGetAnimeByIdLive(t *testing.T) {
	anilistClient := newLiveAnilistClient(t)
	mediaID := 1

	res, err := anilistClient.BaseAnimeByID(context.Background(), &mediaID)
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestListAnime(t *testing.T) {
	tests := []struct {
		name                string
		Page                *int
		Search              *string
		PerPage             *int
		Sort                []*MediaSort
		Status              []*MediaStatus
		Genres              []*string
		AverageScoreGreater *int
		Season              *MediaSeason
		SeasonYear          *int
		Format              *MediaFormat
		IsAdult             *bool
		CountryOfOrigin     *string
	}{
		{
			name:                "Popular",
			Page:                new(1),
			Search:              nil,
			PerPage:             new(20),
			Sort:                []*MediaSort{new(MediaSortTrendingDesc)},
			Status:              nil,
			Genres:              nil,
			AverageScoreGreater: nil,
			Season:              nil,
			SeasonYear:          nil,
			Format:              nil,
			IsAdult:             nil,
			CountryOfOrigin:     nil,
		},
	}

	anilistClient := NewTestAnilistClient()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			cacheKey := ListAnimeCacheKey(
				tt.Page,
				tt.Search,
				tt.PerPage,
				tt.Sort,
				tt.Status,
				tt.Genres,
				tt.AverageScoreGreater,
				tt.Season,
				tt.SeasonYear,
				tt.Format,
				tt.IsAdult,
				tt.CountryOfOrigin,
			)

			t.Log(cacheKey)

			res, err := ListAnimeM(
				anilistClient,
				tt.Page,
				tt.Search,
				tt.PerPage,
				tt.Sort,
				tt.Status,
				tt.Genres,
				tt.AverageScoreGreater,
				tt.Season,
				tt.SeasonYear,
				tt.Format,
				tt.IsAdult,
				tt.CountryOfOrigin,
				util.NewLogger(),
				"",
			)
			assert.NoError(t, err)

			assert.Equal(t, *tt.PerPage, len(res.GetPage().GetMedia()))

			spew.Dump(res)
		})
	}
}
