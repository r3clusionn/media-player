package main

import (
	"embed"
	"os"
	"seanime/internal/debugpick"
	"seanime/internal/server"
)

//go:embed all:web
var WebFS embed.FS

//go:embed internal/icon/logo.png
var embeddedLogo []byte

func main() {
	// media-player: `debug pick` runs the episode-file selection without playback.
	if len(os.Args) > 1 && os.Args[1] == "debug" {
		os.Exit(debugpick.Main(os.Args[2:]))
	}
	server.StartServer(WebFS, embeddedLogo)
}
