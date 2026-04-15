package anime_test

import (
	"seanime/internal/api/anilist"
	"seanime/internal/api/metadata"
	"seanime/internal/api/metadata_provider"
	"seanime/internal/extension"
	"seanime/internal/library/anime"
	"seanime/internal/platforms/anilist_platform"
	"seanime/internal/platforms/platform"
	"seanime/internal/testutil"
	"seanime/internal/util"
	"sort"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

type animeTestHarness struct {
	animeCollection     *anilist.AnimeCollection
	metadataProvider    *animeTestMetadataProvider
	platformRef         *util.Ref[platform.Platform]
	metadataProviderRef *util.Ref[metadata_provider.Provider]
}

type animeTestMetadataProvider struct {
	metadata_provider.Provider
	overrides map[int]*metadata.AnimeMetadata
}

func newAnimeTestHarness(t *testing.T) *animeTestHarness {
	t.Helper()

	// keep the real fixture stack, but make metadata overrides cheap and explicit per test.
	env := testutil.NewTestEnv(t)
	logger := util.NewLogger()
	database := env.MustNewDatabase(logger)
	metadataProvider := &animeTestMetadataProvider{
		Provider:  metadata_provider.NewTestProviderWithEnv(env, database),
		overrides: make(map[int]*metadata.AnimeMetadata),
	}
	anilistClient := anilist.NewTestAnilistClient()
	anilistPlatform := anilist_platform.NewAnilistPlatform(util.NewRef(anilistClient), util.NewRef(extension.NewUnifiedBank()), logger, database)
	animeCollection, err := anilistPlatform.GetAnimeCollection(t.Context(), false)
	require.NoError(t, err)

	metadataProviderInterface := metadata_provider.Provider(metadataProvider)
	platformInterface := platform.Platform(anilistPlatform)

	return &animeTestHarness{
		animeCollection:     animeCollection,
		metadataProvider:    metadataProvider,
		platformRef:         util.NewRef(platformInterface),
		metadataProviderRef: util.NewRef(metadataProviderInterface),
	}
}

func (p *animeTestMetadataProvider) GetAnimeMetadata(platform metadata.Platform, mediaID int) (*metadata.AnimeMetadata, error) {
	if animeMetadata, ok := p.overrides[mediaID]; ok {
		return animeMetadata, nil
	}
	return p.Provider.GetAnimeMetadata(platform, mediaID)
}

func (h *animeTestHarness) findEntry(t *testing.T, mediaID int) *anilist.AnimeListEntry {
	t.Helper()
	return findCollectionEntryByMediaID(t, h.animeCollection, mediaID)
}

func (h *animeTestHarness) setEpisodeMetadata(t *testing.T, mediaID int, mainEpisodes []int, specials map[string]int) *metadata.AnimeMetadata {
	t.Helper()

	// most anime tests only need stable episode numbering, not a full metadata payload.
	media := h.findEntry(t, mediaID).Media
	animeMetadata := anime.NewAnimeMetadataFromEpisodeCount(media, mainEpisodes)
	for aniDBEpisode, episodeNumber := range specials {
		animeMetadata.Episodes[aniDBEpisode] = &metadata.EpisodeMetadata{
			Title:                 media.GetTitleSafe(),
			Image:                 media.GetBannerImageSafe(),
			EpisodeNumber:         episodeNumber,
			Episode:               aniDBEpisode,
			AbsoluteEpisodeNumber: episodeNumber,
			HasImage:              true,
		}
		animeMetadata.SpecialCount++
	}
	h.metadataProvider.overrides[mediaID] = animeMetadata
	return animeMetadata
}

func (h *animeTestHarness) setCustomMetadata(mediaID int, animeMetadata *metadata.AnimeMetadata) {
	h.metadataProvider.overrides[mediaID] = animeMetadata
}

func (h *animeTestHarness) clearMetadataAirDates(mediaID int) {
	if animeMetadata, ok := h.metadataProvider.overrides[mediaID]; ok {
		for _, episode := range animeMetadata.Episodes {
			episode.AirDate = ""
		}
	}
}

func (h *animeTestHarness) newMetadataWithAirDates(t *testing.T, mediaID int, airDates map[int]string) *metadata.AnimeMetadata {
	t.Helper()

	// this is just for the fallback path where current episode count is inferred from aired dates.
	episodes := make([]int, 0, len(airDates))
	for episodeNumber := range airDates {
		episodes = append(episodes, episodeNumber)
	}
	sort.Ints(episodes)

	animeMetadata := anime.NewAnimeMetadataFromEpisodeCount(h.findEntry(t, mediaID).Media, episodes)
	for episodeNumber, airDate := range airDates {
		animeMetadata.Episodes[strconv.Itoa(episodeNumber)].AirDate = airDate
	}

	return animeMetadata
}

func (h *animeTestHarness) clearNextAiringEpisode(t *testing.T, mediaID int) {
	t.Helper()
	h.findEntry(t, mediaID).Media.NextAiringEpisode = nil
}

func (h *animeTestHarness) clearAllNextAiringEpisodes() {
	for _, list := range h.animeCollection.GetMediaListCollection().GetLists() {
		for _, entry := range list.GetEntries() {
			entry.Media.NextAiringEpisode = nil
		}
	}
}

func (h *animeTestHarness) clearEpisodeCount(t *testing.T, mediaID int) {
	t.Helper()
	h.findEntry(t, mediaID).Media.Episodes = nil
}

func (h *animeTestHarness) newLibraryCollection(t *testing.T, localFiles []*anime.LocalFile) *anime.LibraryCollection {
	t.Helper()

	libraryCollection, err := anime.NewLibraryCollection(t.Context(), &anime.NewLibraryCollectionOptions{
		AnimeCollection:     h.animeCollection,
		LocalFiles:          localFiles,
		PlatformRef:         h.platformRef,
		MetadataProviderRef: h.metadataProviderRef,
	})
	require.NoError(t, err)
	return libraryCollection
}

func (h *animeTestHarness) newEntryDownloadInfo(t *testing.T, mediaID int, localFiles []*anime.LocalFile, progress int, status anilist.MediaListStatus) *anime.EntryDownloadInfo {
	t.Helper()

	animeMetadata, err := h.metadataProvider.GetAnimeMetadata(metadata.AnilistPlatform, mediaID)
	require.NoError(t, err)

	info, err := anime.NewEntryDownloadInfo(&anime.NewEntryDownloadInfoOptions{
		LocalFiles:          localFiles,
		Progress:            new(progress),
		Status:              new(status),
		Media:               h.findEntry(t, mediaID).Media,
		MetadataProviderRef: h.metadataProviderRef,
		AnimeMetadata:       animeMetadata,
	})
	require.NoError(t, err)

	return info
}

func (h *animeTestHarness) newMissingEpisodes(t *testing.T, localFiles []*anime.LocalFile, silencedMediaIDs []int) *anime.MissingEpisodes {
	t.Helper()

	missingEpisodes := anime.NewMissingEpisodes(&anime.NewMissingEpisodesOptions{
		AnimeCollection:     h.animeCollection,
		LocalFiles:          localFiles,
		SilencedMediaIds:    silencedMediaIDs,
		MetadataProviderRef: h.metadataProviderRef,
	})
	require.NotNil(t, missingEpisodes)

	return missingEpisodes
}

func (h *animeTestHarness) newUpcomingEpisodes(t *testing.T) *anime.UpcomingEpisodes {
	t.Helper()

	upcomingEpisodes := anime.NewUpcomingEpisodes(&anime.NewUpcomingEpisodesOptions{
		AnimeCollection:     h.animeCollection,
		MetadataProviderRef: h.metadataProviderRef,
	})
	require.NotNil(t, upcomingEpisodes)

	return upcomingEpisodes
}

func patchAnimeCollectionEntry(t *testing.T, collection *anilist.AnimeCollection, mediaID int, patch anilist.AnimeCollectionEntryPatch) *anilist.AnimeListEntry {
	t.Helper()
	anilist.PatchAnimeCollectionEntry(collection, mediaID, patch)
	return findCollectionEntryByMediaID(t, collection, mediaID)
}

func patchCollectionEntryFormat(t *testing.T, collection *anilist.AnimeCollection, mediaID int, format anilist.MediaFormat) {
	t.Helper()
	entry := findCollectionEntryByMediaID(t, collection, mediaID)
	entry.Media.Format = &format
}

func patchCollectionEntryEpisodeCount(t *testing.T, collection *anilist.AnimeCollection, mediaID int, episodeCount int) {
	t.Helper()
	entry := findCollectionEntryByMediaID(t, collection, mediaID)
	entry.Media.Episodes = &episodeCount
	entry.Media.NextAiringEpisode = nil
}

func patchEntryMediaStatus(t *testing.T, collection *anilist.AnimeCollection, mediaID int, status anilist.MediaStatus) {
	t.Helper()
	findCollectionEntryByMediaID(t, collection, mediaID).Media.Status = new(status)
}

func findCollectionEntryByMediaID(t *testing.T, collection *anilist.AnimeCollection, mediaID int) *anilist.AnimeListEntry {
	t.Helper()
	entry, found := collection.GetListEntryFromAnimeId(mediaID)
	require.True(t, found)
	return entry
}
