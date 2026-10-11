// Package debugpick runs the episode-file selection used by debrid and torrent
// streaming without starting playback, and prints what it decided and why.
//
//	media-player debug pick --media <anilistId> --ep <n> --files <list.txt>
//
// list.txt holds one torrent file path per line, in the torrent's order (the
// order Real-Debrid and the torrent client report). The AniList and episode
// metadata are fetched live; no account or debrid key is needed.
package debugpick

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"seanime/internal/api/anilist"
	"seanime/internal/api/metadata_provider"
	"seanime/internal/database/db"
	"seanime/internal/extension"
	"seanime/internal/platforms/anilist_platform"
	"seanime/internal/platforms/platform"
	torrentanalyzer "seanime/internal/torrents/analyzer"
	"seanime/internal/util"
	"seanime/internal/util/filecache"

	"github.com/rs/zerolog"
)

// Main handles `debug <command> ...`. It returns the process exit code.
func Main(args []string) int {
	if len(args) == 0 || args[0] != "pick" {
		fmt.Fprintln(os.Stderr, "usage: media-player debug pick --media ID --ep N --files LIST")
		return 2
	}
	fs := flag.NewFlagSet("debug pick", flag.ContinueOnError)
	mediaID := fs.Int("media", 0, "AniList media ID of the entry being watched")
	episode := fs.Int("ep", 0, "episode number as Seanime passes it (relative to the entry)")
	filesPath := fs.String("files", "", "file with one torrent file path per line, in torrent order")
	dataDir := fs.String("datadir", "", "cache directory (default: a temporary one)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *mediaID == 0 || *episode == 0 || *filesPath == "" {
		fs.Usage()
		return 2
	}
	if err := run(*mediaID, *episode, *filesPath, *dataDir); err != nil {
		fmt.Fprintln(os.Stderr, "debug pick:", err)
		return 1
	}
	return 0
}

func readLines(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out, nil
}

func run(mediaID, episode int, filesPath, dataDir string) error {
	paths, err := readLines(filesPath)
	if err != nil {
		return err
	}
	if dataDir == "" {
		if dataDir, err = os.MkdirTemp("", "media-player-debugpick-"); err != nil {
			return err
		}
		defer os.RemoveAll(dataDir)
	}

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).Level(zerolog.WarnLevel)
	database, err := db.NewDatabase(dataDir, "debugpick", &logger)
	if err != nil {
		return err
	}
	fileCacher, err := filecache.NewCacher(filepath.Join(dataDir, "cache"))
	if err != nil {
		return err
	}
	bankRef := util.NewRef(extension.NewUnifiedBank())
	clientRef := util.NewRef[anilist.AnilistClient](anilist.NewAnilistClient("", filepath.Join(dataDir, "anilist")))
	plat := anilist_platform.NewAnilistPlatform(clientRef, bankRef, &logger, database)
	provider := metadata_provider.NewProvider(&metadata_provider.NewProviderImplOptions{
		Logger: &logger, FileCacher: fileCacher, Database: database, ExtensionBankRef: bankRef,
	})

	media, err := plat.GetAnimeWithRelations(context.Background(), mediaID)
	if err != nil {
		return fmt.Errorf("fetching AniList media %d: %w", mediaID, err)
	}
	fmt.Printf("Media %d: %s (%d episodes)\n", mediaID, media.GetTitleSafe(), media.GetCurrentEpisodeCount())
	fmt.Printf("Watching episode %d; %d files in the torrent\n\n", episode, len(paths))

	analyzer := torrentanalyzer.NewAnalyzer(&torrentanalyzer.NewAnalyzerOptions{
		Logger:              &logger,
		Filepaths:           paths,
		Media:               media,
		PlatformRef:         util.NewRef[platform.Platform](plat),
		MetadataProviderRef: util.NewRef[metadata_provider.Provider](provider),
		ForceMatch:          true,
	})
	analysis, err := analyzer.AnalyzeTorrentFiles()
	if err != nil {
		return fmt.Errorf("analyzing files: %w", err)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "index\ttype\tepisode\taniDB\tfile")
	for _, f := range analysis.GetFiles() {
		lf := f.GetLocalFile()
		fmt.Fprintf(tw, "%d\t%s\t%d\t%s\t%s\n", f.GetIndex(), lf.Metadata.Type, lf.Metadata.Episode,
			lf.Metadata.AniDBEpisode, f.GetPath())
	}
	tw.Flush()

	// What the upstream code picks: the first file whose AniDB episode string
	// equals the relative episode number (finder.go, file_selection.go).
	fmt.Println()
	if f, ok := analysis.GetFileByAniDBEpisode(strconv.Itoa(episode)); ok {
		fmt.Printf("upstream pick (GetFileByAniDBEpisode(%q)): [%d] %s\n", strconv.Itoa(episode), f.GetIndex(), f.GetPath())
	} else {
		fmt.Printf("upstream pick (GetFileByAniDBEpisode(%q)): none\n", strconv.Itoa(episode))
	}
	var claims []string
	for _, f := range analysis.GetFiles() {
		if f.GetLocalFile().Metadata.AniDBEpisode == strconv.Itoa(episode) {
			claims = append(claims, fmt.Sprintf("[%d] %s", f.GetIndex(), f.GetPath()))
		}
	}
	if len(claims) > 1 {
		fmt.Printf("AMBIGUOUS: %d files claim episode %d; upstream takes the first in torrent order:\n  %s\n",
			len(claims), episode, strings.Join(claims, "\n  "))
	}
	if f, ok := analysis.GetMainFileByEpisode(episode); ok {
		fmt.Printf("main-file pick (GetMainFileByEpisode(%d)): [%d] %s\n", episode, f.GetIndex(), f.GetPath())
	}

	t := analysis.EpisodeTarget(episode)
	fmt.Printf("\nentry: season %d, %d episodes before it, %d episodes, special entry %v\n",
		t.Season, t.Offset, t.EpisodeCount, t.SpecialEntry)
	if f, reason, err := analysis.PickEpisode(episode, nil); err != nil {
		fmt.Printf("media-player pick: none (%v)\n", err)
	} else {
		fmt.Printf("media-player pick: [%d] %s\n  because: %s\n", f.GetIndex(), f.GetPath(), reason)
	}
	return nil
}
