package manga

import (
	"path/filepath"
	"seanime/internal/database/db"
	"seanime/internal/events"
	"seanime/internal/extension"
	"seanime/internal/testutil"
	"seanime/internal/util"
	"seanime/internal/util/filecache"
	"testing"
)

func GetFakeRepository(t *testing.T, db *db.Database) *Repository {
	logger := util.NewLogger()
	cacheDir := filepath.Join(testutil.ConfigData.Path.DataDir, "cache")
	fileCacher, err := filecache.NewCacher(cacheDir)
	if err != nil {
		t.Fatal(err)
	}

	repository := NewRepository(&NewRepositoryOptions{
		Logger:           logger,
		FileCacher:       fileCacher,
		CacheDir:         cacheDir,
		ServerURI:        "",
		WsEventManager:   events.NewMockWSEventManager(logger),
		DownloadDir:      filepath.Join(testutil.ConfigData.Path.DataDir, "manga"),
		Database:         db,
		ExtensionBankRef: util.NewRef(extension.NewUnifiedBank()),
	})

	return repository
}
