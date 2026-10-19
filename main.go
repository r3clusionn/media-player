// The runtime re-reads the cgroup CPU limits in the background to update GOMAXPROCS
// (Go 1.25+); nothing here runs under a CPU limit, and in a 2-minute profile of
// media-player that was half of all the time spent in Go code.
//go:debug updatemaxprocs=0

package main

import (
	"embed"
	"os"
	"runtime"
	"seanime/internal/debugpick"
	"seanime/internal/desktop"
	"seanime/internal/server"
)

//go:embed all:web
var WebFS embed.FS

//go:embed internal/icon/logo.png
var embeddedLogo []byte

// media-player: CEF and X11 must run on the process's main thread.
func init() { runtime.LockOSThread() }

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "debug":
			// `debug pick` runs the episode-file selection without playback.
			os.Exit(debugpick.Main(os.Args[2:]))
		case "--server":
			// Seanime's web server alone, for a browser (the upstream behaviour).
			os.Args = append(os.Args[:1], os.Args[2:]...)
			server.StartServer(WebFS, embeddedLogo)
			return
		}
	}
	os.Exit(desktop.Run(WebFS, embeddedLogo))
}
