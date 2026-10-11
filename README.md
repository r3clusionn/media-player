# media-player

An anime and movie player for Linux built on [Seanime](https://github.com/5rahim/seanime): Seanime's interface and features, unchanged, in a standalone window instead of Electron, with mpv playing inside it and a Stremio-style Movies & TV section next to the anime. It is made for one laptop (Ryzen AI 9 HX 475, Radeon 890M, 2880x1800 at 120 Hz, Gentoo with dwm) and tuned for it: no new system packages, nothing running when the window is closed, Anime4K and debanding chosen per video, and Real-Debrid picking the right episode file every time.

**Status:** in development, phase 1 of 7 (see [the plan](docs/PLAN.md)). The code is Seanime 3.10.3 with no changes yet; it builds on the target laptop with the system's Go and Node and runs in web-server mode. None of the features below that differ from upstream exist yet.

![The Discover page of the current build at 2880x1800: a banner for a releasing show with its poster, genres, score and synopsis, the Home, Schedule, My lists and Discover tabs, the icon sidebar on the left, and a Trending Right Now row of cover cards under genre filters](docs/images/discover.png)

## Features

- **Seanime, 1:1.** Every screen and feature of upstream Seanime: AniList and MAL lists, local library and scanner, torrent and debrid streaming, manga, online streaming, extensions, schedule, Nakama, auto-downloader. Nothing is removed; features you do not use are not started.
- **A standalone window, no Electron.** The React interface runs in Chromium's engine embedded as a library (CEF) in this program, with no browser window, no Node.js at runtime and no separate server process. Closing the window ends everything.
- **mpv inside the window.** libmpv draws the video and the page is drawn over it, so Seanime's own player controls (VideoCore in mpvcore mode) stay exactly as they are, Anime4K picker included.
- **Picture quality chosen per video.** Anime4K Mode A+A and debanding for anime up to 1080p, lighter processing for 1440p and 4K sources, no Anime4K on live action, HDR tone mapping, VA-API decoding. The same on battery.
- **The right episode.** A rewritten episode matcher for debrid batches (see How it works) used by autoplay, resume and manual picks; when it is not sure, it shows the file picker instead of guessing.
- **Movies & TV.** Stremio add-ons (Torrentio first, Comet as a backup) through Real-Debrid, with catalogs and details from Cinemeta, Continue Watching and Next Up, in pages built from Seanime's own components.
- **Fast next episode.** The next episode's file is found and its Real-Debrid link resolved at 85% of the current one.

## How to install

Requires Go 1.27, Node.js 26 with npm, gcc, pkgconf and libmpv (mpv 0.41). Linux with X11 only.

```sh
git clone https://github.com/r3clusionn/media-player
cd media-player/seanime-web
npm ci && npm run build
cd .. && rm -rf web && cp -a seanime-web/out web
GOAMD64=v4 go build -o media-player -trimpath -ldflags="-s -w" -tags=nosystray
```

This builds the web-server form for now. The standalone window (phase 2) adds the CEF runtime, which is downloaded into `third_party/`, not installed system-wide.

## How to use

Until the standalone window exists:

```sh
./media-player --datadir ~/.local/share/media-player --host 127.0.0.1 --port 43211
```

then open `http://127.0.0.1:43211`. Settings, accounts and Real-Debrid are set up in the interface as in Seanime ([its documentation](https://seanime.app/docs)).

| Flag | What it does |
|---|---|
| `--datadir DIR` | Where the database, settings, cache and downloads live. |
| `--host`, `--port` | The address the server listens on (default `127.0.0.1:43211`). |
| `--password` | Require a password from the interface. |
| `--disable-features LIST` | Turn named features off. |

## How it works

- **The window** is one X11 window with an EGL context. CEF renders the page off-screen and hands each changed frame over as a GPU buffer (dmabuf), which is imported with `EGL_EXT_image_dma_buf_import`; libmpv's render API draws video into the same context underneath, and the page is blended on top. Nothing is redrawn while nothing changes.
- **The page talks to the player** through the same JavaScript interface Seanime's Electron app provides (`window.electron` and the mpv-prism channels), so the React code is unchanged.
- **The wrong-episode bug** (debrid streaming, file chosen automatically) has four suspects in the upstream code, documented in [the plan](docs/PLAN.md#44-the-wrong-episode-bug-diagnosis-and-fix): autoplay taking the file after the last one by position and sending that position as the Real-Debrid file ID; resume matching a season's episode number as an absolute one; links taken from Real-Debrid by position; and Real-Debrid's removed cache check. The fix is one order-independent matcher that drops openings, endings and extras, maps season and absolute numbering, checks the name and size Real-Debrid returns, and asks when unsure.

## Measurements

The unchanged upstream build on the target laptop (Ryzen AI 9 HX 475, Gentoo, Go 1.27.1, Node 26.3), first run with an empty data directory and no account:

| | |
|---|---|
| Web interface build (`npm run build`) | 8 s, 16 MB of output |
| Server build (`go build`, first, modules downloaded) | 68 s, 81 MB binary |
| Server idle after start | 59.7 MB resident (16.4 MB anonymous), 19 threads |

Targets for the finished program are in [the plan](docs/PLAN.md#targets-to-be-measured-not-promised); results go here as they are measured.

## Verification

Nothing of this project's own is verified yet. So far: the upstream code builds unchanged with the system toolchain, the server starts, the first-run setup accepts default settings, and the Discover page loads live AniList data (the screenshot above, captured with headless Thorium at 1440x900 and scale 2).

## Limits

- Not usable as a standalone program yet; it is upstream Seanime in web-server mode.
- Linux with X11 only. Wayland is not a target.
- Built for one machine: `GOAMD64=v4` binaries need AVX-512.
- The Movies & TV section needs Real-Debrid; there is no torrent streaming for it.

## License

GPL-3.0, as Seanime (see `LICENSE`). Seanime is by [5rahim](https://github.com/5rahim) and its contributors; its original README is kept in [docs/SEANIME-README.md](docs/SEANIME-README.md).
