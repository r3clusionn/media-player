package core

import (
	"seanime/internal/extension"
	"seanime/internal/extension_repo"
	manga_providers "seanime/internal/manga/providers"
	"seanime/internal/torrents/animetosho"
	"seanime/internal/torrents/nyaa"
	"seanime/internal/torrents/seadex"
	"seanime/internal/util"

	"github.com/rs/zerolog"
)

func LoadCustomSourceExtensions(extensionRepository *extension_repo.Repository) {
	extensionRepository.LoadOnlyWrapper([]extension.Type{extension.TypeCustomSource}, func() {
		extensionRepository.ReloadExternalExtensions()
	})
}

func LoadExtensions(extensionRepository *extension_repo.Repository, logger *zerolog.Logger, config *Config) {
	// Load built-in extensions
	extensionRepository.ReloadBuiltInExtension(extension.Extension{
		ID:          manga_providers.LocalProvider,
		Name:        "Local",
		Version:     "",
		ManifestURI: "builtin",
		Language:    extension.LanguageGo,
		Type:        extension.TypeMangaProvider,
		Author:      "Seanime",
		Lang:        "multi",
		Icon:        "https://raw.githubusercontent.com/5rahim/hibike/main/icons/local-manga.png",
	}, manga_providers.NewLocal(config.Manga.LocalDir, logger))

	// media-player: Seanime's own built-in torrent providers, removed upstream
	// in 3.x (581fdb46); the official marketplace no longer lists any, and
	// torrent and debrid streaming need one to search.
	nyaaUserConfig := extension.UserConfig{
		Version: 1,
		Fields: []extension.ConfigField{{
			Name:    "apiUrl",
			Label:   "API URL",
			Type:    extension.ConfigFieldTypeText,
			Default: util.Decode("aHR0cHM6Ly9ueWFhLnNpLz9wYWdlPXJzcyZxPSs="),
		}},
	}
	builtin := func(id, name, lang, icon string, cfg *extension.UserConfig, provider interface{}) {
		extensionRepository.ReloadBuiltInExtension(extension.Extension{
			ID:          id,
			Name:        name,
			ManifestURI: "builtin",
			Language:    extension.LanguageGo,
			Type:        extension.TypeAnimeTorrentProvider,
			Author:      "Seanime",
			Lang:        lang,
			Icon:        "https://raw.githubusercontent.com/5rahim/hibike/main/icons/" + icon,
			UserConfig:  cfg,
		}, provider)
	}
	builtin("animetosho", "AnimeTosho", "en", "animetosho.png", nil, animetosho.NewProvider(logger))
	builtin("nyaa", "Nyaa", "en", "nyaa.png", &nyaaUserConfig, nyaa.NewProvider(logger, "anime-eng"))
	builtin("nyaa-non-eng", "Nyaa (Non-English)", "multi", "nyaa.png", &nyaaUserConfig, nyaa.NewProvider(logger, "anime-non-eng"))
	builtin("seadex", "SeaDex", "en", "seadex.png", &extension.UserConfig{
		Version: 1,
		Fields: []extension.ConfigField{{
			Name:    "apiUrl",
			Label:   "API URL",
			Type:    extension.ConfigFieldTypeText,
			Default: util.Decode("aHR0cHM6Ly9yZWxlYXNlcy5tb2UvYXBpL2NvbGxlY3Rpb25zL2VudHJpZXMvcmVjb3Jkcw=="),
		}},
	}, seadex.NewProvider(logger))

	// Load external extensions
	//extensionRepository.ReloadExternalExtensions()
	extensionRepository.LoadOnlyWrapper([]extension.Type{extension.TypeMangaProvider, extension.TypeOnlinestreamProvider, extension.TypeAnimeTorrentProvider, extension.TypePlugin}, func() {
		extensionRepository.ReloadExternalExtensions()
	})
}
