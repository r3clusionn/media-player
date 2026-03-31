package anime_test

import (
	"seanime/internal/api/anilist"
	"seanime/internal/api/metadata_provider"
	"seanime/internal/database/db"
	"seanime/internal/extension"
	"seanime/internal/library/anime"
	"seanime/internal/platforms/anilist_platform"
	"seanime/internal/util"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewLibraryCollection(t *testing.T) {
	logger := util.NewLogger()

	database, err := db.NewDatabase(t.TempDir(), "test", logger)
	assert.NoError(t, err)

	metadataProvider := metadata_provider.NewTestProvider(t, database)
	//wsEventManager := events.NewMockWSEventManager(logger)

	anilistClient := anilist.NewTestAnilistClient()
	anilistPlatform := anilist_platform.NewAnilistPlatform(util.NewRef(anilistClient), util.NewRef(extension.NewUnifiedBank()), logger, database)

	animeCollection, err := anilistPlatform.GetAnimeCollection(t.Context(), false)

	if assert.NoError(t, err) {

		// Mock Anilist collection and local files
		// User is currently watching Sousou no Frieren and One Piece
		lfs := make([]*anime.LocalFile, 0)

		// Sousou no Frieren
		// 7 episodes downloaded, 4 watched
		mediaId := 154587
		lfs = append(lfs, anime.NewTestLocalFiles(
			anime.TestLocalFileGroup{
				LibraryPath:      "E:/Anime",
				FilePathTemplate: "E:\\Anime\\Sousou no Frieren\\[SubsPlease] Sousou no Frieren - %ep (1080p) [F02B9CEE].mkv",
				MediaID:          mediaId,
				Episodes: []anime.TestLocalFileEpisode{
					{Episode: 1, AniDBEpisode: "1", Type: anime.LocalFileTypeMain},
					{Episode: 2, AniDBEpisode: "2", Type: anime.LocalFileTypeMain},
					{Episode: 3, AniDBEpisode: "3", Type: anime.LocalFileTypeMain},
					{Episode: 4, AniDBEpisode: "4", Type: anime.LocalFileTypeMain},
					{Episode: 5, AniDBEpisode: "5", Type: anime.LocalFileTypeMain},
					{Episode: 6, AniDBEpisode: "6", Type: anime.LocalFileTypeMain},
					{Episode: 7, AniDBEpisode: "7", Type: anime.LocalFileTypeMain},
				},
			},
		)...)
		anilist.PatchAnimeCollectionEntry(animeCollection, mediaId, anilist.AnimeCollectionEntryPatch{
			Status:   new(anilist.MediaListStatusCurrent),
			Progress: new(4), // Mock progress
		})

		// One Piece
		// Downloaded 1070-1075 but only watched up until 1060
		mediaId = 21
		lfs = append(lfs, anime.NewTestLocalFiles(
			anime.TestLocalFileGroup{
				LibraryPath:      "E:/Anime",
				FilePathTemplate: "E:\\Anime\\One Piece\\[SubsPlease] One Piece - %ep (1080p) [F02B9CEE].mkv",
				MediaID:          mediaId,
				Episodes: []anime.TestLocalFileEpisode{
					{Episode: 1070, AniDBEpisode: "1070", Type: anime.LocalFileTypeMain},
					{Episode: 1071, AniDBEpisode: "1071", Type: anime.LocalFileTypeMain},
					{Episode: 1072, AniDBEpisode: "1072", Type: anime.LocalFileTypeMain},
					{Episode: 1073, AniDBEpisode: "1073", Type: anime.LocalFileTypeMain},
					{Episode: 1074, AniDBEpisode: "1074", Type: anime.LocalFileTypeMain},
					{Episode: 1075, AniDBEpisode: "1075", Type: anime.LocalFileTypeMain},
				},
			},
		)...)
		anilist.PatchAnimeCollectionEntry(animeCollection, mediaId, anilist.AnimeCollectionEntryPatch{
			Status:   new(anilist.MediaListStatusCurrent),
			Progress: new(1060), // Mock progress
		})

		// Add unmatched local files
		mediaId = 0
		lfs = append(lfs, anime.NewTestLocalFiles(
			anime.TestLocalFileGroup{
				LibraryPath:      "E:/Anime",
				FilePathTemplate: "E:\\Anime\\Unmatched\\[SubsPlease] Unmatched - %ep (1080p) [F02B9CEE].mkv",
				MediaID:          mediaId,
				Episodes: []anime.TestLocalFileEpisode{
					{Episode: 1, AniDBEpisode: "1", Type: anime.LocalFileTypeMain},
					{Episode: 2, AniDBEpisode: "2", Type: anime.LocalFileTypeMain},
					{Episode: 3, AniDBEpisode: "3", Type: anime.LocalFileTypeMain},
					{Episode: 4, AniDBEpisode: "4", Type: anime.LocalFileTypeMain},
				},
			},
		)...)

		libraryCollection, err := anime.NewLibraryCollection(t.Context(), &anime.NewLibraryCollectionOptions{
			AnimeCollection:     animeCollection,
			LocalFiles:          lfs,
			PlatformRef:         util.NewRef(anilistPlatform),
			MetadataProviderRef: util.NewRef(metadataProvider),
		})

		if assert.NoError(t, err) {

			assert.Equal(t, 1, len(libraryCollection.ContinueWatchingList)) // Only Sousou no Frieren is in the continue watching list
			assert.Equal(t, 4, len(libraryCollection.UnmatchedLocalFiles))  // 4 unmatched local files

		}
	}

}
