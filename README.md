# media-player

An anime and movie player for Linux built on [Seanime](https://github.com/5rahim/seanime): Seanime's interface and features, unchanged, in a standalone window instead of Electron, with mpv playing inside it and a Stremio-style Movies & TV section next to the anime. It is made for one laptop (Ryzen AI 9 HX 475, Radeon 890M, 2880x1800 at 120 Hz, Gentoo with dwm) and tuned for it: no new system packages, nothing running when the window is closed, Anime4K and debanding chosen per video, and Real-Debrid picking the right episode file every time.

**Status:** in development (see [the plan](docs/PLAN.md)).
- **Phase 0 (risk checks): done.** Seanime's UI renders through CEF into a plain X11/EGL window with no new packages, and libmpv draws video into the same kind of window ([results](docs/phase0.md)).
- **Phase 1: the wrong-episode fixes are in.** A new episode matcher, autoplay and resume fixes, and checked Real-Debrid links. Tested with unit tests and live AniList data, not yet in real playback with a Real-Debrid account.
- **Not yet:** the standalone window (phase 2), Movies & TV, and the tuning. Until then this builds and runs as Seanime's web server.

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

- **Wrong episode.** `media-player debug pick` runs the episode-file selection with live AniList data and no playback. On the lists in [`internal/debugpick/lists`](internal/debugpick/lists):

  | Pack | Episode | Upstream plays | media-player plays |
  |---|---|---|---|
  | Attack on Titan Seasons 1-2, watching Season 2 | 5 | S01E05 | S02E05 |
  | Frieren with openings, endings and a mini anime | 1 | Mini Anime - 01 | 01 |
  | Frieren with a v2 | 7 | 07 | 07v2 |
  | Jujutsu Kaisen S2 in absolute numbers (25-47) | 1, 23 | 25, 47 | 25, 47 |

  The entry's place in the show comes out right from AniList: Attack on Titan Season 2 is season 2 after 25 episodes, Jujutsu Kaisen Season 2 is season 2 after 24.
- `go test ./internal/episodematch/`: 21 cases. They include seasons in packs, absolute numbering, extras folders and names, v2, multi-episode files, a season AniList splits differently, OVA entries, and every case again with the file order shuffled 50 times (the answer must not change).
- `go test ./internal/debrid/realdebrid/`: Real-Debrid links in order, out of order, packed into an archive, and renamed, against a fake RD server.
- Upstream's tests for the packages touched (analyzer, autoselect, debrid client, torrent streaming) still pass.
- **Phase 0:** see [docs/phase0.md](docs/phase0.md). CEF first frame 307 ms; 0.73 ms per frame to import and draw; Anime4K costs per preset at 2880x1800.
- **Not verified yet:** real playback with a Real-Debrid account; battery power; frame pacing with the screen on.

## Limits

- Not usable as a standalone program yet; it is upstream Seanime in web-server mode.
- Linux with X11 only. Wayland is not a target.
- Built for one machine: `GOAMD64=v4` binaries need AVX-512.
- The Movies & TV section needs Real-Debrid; there is no torrent streaming for it.

## License

GPL-3.0, as Seanime (see `LICENSE`). Seanime is by [5rahim](https://github.com/5rahim) and its contributors; its original README is kept in [docs/SEANIME-README.md](docs/SEANIME-README.md).
