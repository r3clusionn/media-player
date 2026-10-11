// Package desktop runs media-player as a standalone program: Seanime's server
// in this process (on 127.0.0.1:43211, the address the desktop web build
// expects) and the web UI in an appshell window. Closing the window ends the
// process; nothing keeps running.
package desktop

import (
	"embed"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"seanime/internal/appshell"
	"seanime/internal/server"
)

const port = 43211

// Run blocks until the window is closed and returns the exit status. It must
// be called on the main OS thread.
func Run(webFS embed.FS, logo []byte) int {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "media-player:", err)
		return 1
	}
	exe, _ = filepath.EvalSymlinks(exe)
	dir := filepath.Dir(exe)

	home, _ := os.UserHomeDir()
	dataDir := envOr("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	cacheDir := envOr("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	appData := filepath.Join(dataDir, "media-player")
	shellData := filepath.Join(appData, "desktop")
	shellCache := filepath.Join(cacheDir, "media-player", "cef")
	for _, d := range []string{appData, shellData, shellCache} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "media-player:", err)
			return 1
		}
	}

	if listening() {
		fmt.Fprintf(os.Stderr, "media-player: something is already listening on 127.0.0.1:%d (another media-player?)\n", port)
		return 1
	}

	// Seanime's flags are parsed from os.Args inside StartServer.
	os.Args = []string{os.Args[0], "--datadir", appData, "--host", "127.0.0.1", "--port", fmt.Sprint(port)}
	go server.StartServer(webFS, logo)
	deadline := time.Now().Add(20 * time.Second)
	for !listening() {
		if time.Now().After(deadline) {
			fmt.Fprintln(os.Stderr, "media-player: the server did not start")
			return 1
		}
		time.Sleep(20 * time.Millisecond)
	}

	return appshell.Run(appshell.Options{
		URL:          fmt.Sprintf("http://127.0.0.1:%d/", port),
		Title:        "media-player",
		Class:        "media-player",
		CacheDir:     shellCache,
		DataDir:      shellData,
		HelperPath:   filepath.Join(dir, "appshell-helper"),
		ResourcesDir: dir,
		LocalesDir:   filepath.Join(dir, "locales"),
		Anime4KDir:   filepath.Join(dir, "anime4k"),
		ServerPort:   port,
		Width:        1600,
		Height:       1000,
	})
}

func listening() bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
