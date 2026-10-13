# media-player

An anime and movie player for Linux built on [Seanime](https://github.com/5rahim/seanime): Seanime's interface and features, unchanged, in a standalone window instead of Electron, with mpv playing inside it and a Stremio-style Movies & TV section next to the anime. It is made for one laptop (Ryzen AI 9 HX 475, Radeon 890M, 2880x1800 at 120 Hz, Gentoo with dwm) and tuned for it: no new system packages, nothing running when the window is closed, Anime4K and debanding for anime, and Real-Debrid always streaming the episode you asked for.

**Status:** working, used on the target laptop. Anime (debrid, torrent and local files), Movies & TV through Torrentio and Comet, autoplay, resume and Anime4K are all tested in the real program with a Real-Debrid account (see Verification). Built and run on that one machine only.

![The built-in player at 2880x1800 playing Frieren episode 1 with Anime4K Mode A+A and debanding: Fern and Heiter in a lamp-lit town at night with the subtitle "It's nothing.", the show and episode title at the top left, and Seanime's control bar at the bottom with play, next, volume, the time 6:56 / 25:59, and the subtitle, audio, picture-in-picture and fullscreen buttons](docs/images/player.png)

## Features

- **Seanime, 1:1.** Every screen and feature of upstream Seanime: AniList and MAL lists, local library and scanner, torrent and debrid streaming, manga, online streaming, extensions, schedule, Nakama, auto-downloader. Nothing is removed, and features you don't use aren't started. The built-in torrent search providers that upstream moved to extensions (AnimeTosho, Nyaa, SeaDex) are back.
- **A standalone window, no Electron.** The React interface runs in Chromium's engine embedded as a library (CEF) inside this program. There is no browser window and no Node.js at runtime, and the server runs in the same process. Closing the window ends everything.
- **mpv inside the window.** libmpv draws the video and the page is drawn over it, so Seanime's own player (mpvcore) keeps its controls, menus and Anime4K picker exactly as they are.
- **Smooth with shaders.** Video renders on its own thread, ahead of time, so the controls, menus and mouse stay at the panel's 120 Hz while Anime4K works: input reaches the page in about 10 ms during playback, with 0 dropped frames up to Anime4K Mode A (HQ).
- **Picture quality.** Anime4K and debanding for anime, chosen in the player's menu (up arrow > Shaders) and off until you pick them, as in Seanime. Without shaders, video is scaled with `ewa_lanczossharp`. VA-API decoding and HDR tone mapping with peak detection for everything. With X at depth 30 the video reaches the panel in 10 bits per channel. Movies & TV is live action: no Shaders entry, no Anime4K, no debanding.
- **The right episode.** A rewritten episode matcher for debrid batches (see How it works) is used by autoplay, resume and manual picks. When it isn't sure, it shows the file picker instead of guessing. Movies & TV refuses a file whose name says another season or episode.
- **Torrent providers.** Anime searches SeaDex by default with AnimeTosho (and Nyaa) one click away in the torrent drawer. Movies & TV asks Torrentio by default or Comet if you pick it, with the other as the backup.
- **Movies & TV.** Catalogs, search and details from Cinemeta. Real-Debrid streams come from the chosen add-on, and from the other one when it fails or has nothing cached. You get Continue Watching and Next Up, resume, and next-episode autoplay that stays on the same release. The pages are built from Seanime's own components.
- **Only one file from big packs.** For an uncached 40 GB season pack, Real-Debrid fetches only the episode being played, so playback starts in seconds. An already-cached or already-added torrent is reused.

## How to install

Requires Go 1.27, Node.js 26 with npm, gcc, libmpv (mpv 0.41), yyjson, and the X11 libraries (libX11, libXi, libXss, libXext) with EGL and OpenGL 4.6. Linux with X11 only.

The build also needs three directories in `~/src/third_party` (or `$THIRD_PARTY`), none installed system-wide:

- `cef/`: the CEF 154.0.34 "minimal" Linux x64 build from the [official CEF builds](https://cef-builds.spotifycdn.com/index.html), extracted.
- `runtime/`: NSS, NSPR and xkbcommon, which CEF needs and this Gentoo install doesn't have (copied from a Chromium-based browser's bundled libraries).
- `anime4k/`: the [Anime4K](https://github.com/bloc97/Anime4K) v4 GLSL shaders.

```sh
git clone https://github.com/r3clusionn/media-player
cd media-player
appshell/build.sh --install
```

This builds the web interface, the helper process and the program into `build/dist`, then copies them to `~/.local/opt/media-player` (444 MB, with CEF stripped of its debug info) and links `~/.local/bin/media-player`. `--skip-web` reuses the last web build.

## How to use

```sh
media-player
```

The window opens on Seanime's home screen. Accounts, Real-Debrid and the library are set up in the interface as in Seanime ([its documentation](https://seanime.app/docs)). Data lives in `~/.local/share/media-player`, caches in `~/.cache/media-player`.

**Movies & TV** is the film icon in the sidebar. Pick a title, then an episode (or Watch for a movie). The drawer lists the streams, with cached ones first; the switch at its top picks Torrentio or Comet (remembered). **Play best** plays the highest-quality cached file that streams smoothly over Wi-Fi: remux-sized files (over 30 GB for a movie, 10 GB for an episode) and Dolby-Vision-only files come after the rest. If Real-Debrid isn't ready with that file, it moves on to the next one. Clicking a stream plays exactly that file.

![The Movies & TV page of a series: Ted Lasso's backdrop and poster, its logo, year, runtime, IMDb rating and genres, and the stream drawer open on the right for S1 E1 "Pilot" with a Play best button and a list of cached Torrentio streams tagged with quality, HDR and size, such as Ted.Lasso.S01E01.HDR.2160p.WEB.H265-SKEDADDLE.mkv at 5.41 GB](docs/images/streams.png)

**Anime4K and debanding** are in the player's settings menu (up arrow > Shaders), off until chosen and then remembered. Movies & TV doesn't offer them.

| | |
|---|---|
| `media-player` | Open the window. |
| `media-player --server` | Run only Seanime's web server (open `http://127.0.0.1:43211` in a browser). |
| `media-player debug pick` | Run the episode-file matcher on saved lists, without playback. |
| `APPSHELL_DEBUG=1` | Log the page console, mpv warnings, and paint and draw rates to stderr. |

## How it works

- **The window** is one X11 window with a desktop OpenGL 4.6 context (EGL), at the screen's depth: 10 bits per channel when X runs at depth 30. CEF renders the page off-screen and hands each changed frame over as a GPU buffer (dmabuf), imported with `EGL_EXT_image_dma_buf_import`. Each frame is the video, then the page blended on top. Nothing is redrawn while nothing changes, and the swap is skipped while the window is hidden.
- **Video on its own thread.** Each player has a render thread with its own OpenGL context, sharing textures with the window's. mpv hands frames over up to 0.1 s early (`video-timing-offset`), and the thread draws each one into a queue of six textures, tagged with the time it should appear. The window's loop shows the newest frame that is due at each swap and never waits for mpv, so a 20–35 ms Anime4K frame doesn't hold up the page or input. The render context runs at low GPU priority, and every shader pass is flushed to the GPU on its own, so the window's small frames are scheduled between mpv's passes. While the video isn't visible, frames are consumed without drawing.
- **Desktop OpenGL** gives mpv compute shaders (HDR peak detection, faster polar scaling) and runs Anime4K about 8% faster than GLES.
- **The page talks to the player** through the JavaScript interface Seanime's Electron app provides (`window.electron` and the mpv-prism calls). A small script injected by the helper process implements it over CEF process messages, so the React code is unchanged. Every mpv request is asynchronous, so the window never waits on mpv.
- **Video under the page.** The player's ancestors are made transparent by a CSS `:has()` rule, not by classes, because React rewrites class names when it re-renders. Page content painted beneath the player gets a mask with a hole the video's shape; the player's own menus and popovers, painted above it, are left alone. The hidden control bar is skipped with `content-visibility`, so the page stays idle during playback instead of restyling an invisible seek bar.
- **The wrong-episode bug** (debrid streaming, file chosen automatically) had four causes upstream, documented in [the plan](docs/PLAN.md#44-the-wrong-episode-bug-diagnosis-and-fix):
  - Autoplay took the file after the last one by position and sent that position as the Real-Debrid file ID.
  - Resume matched a season's episode number as an absolute one.
  - Links were taken from Real-Debrid by position.
  - It relied on Real-Debrid's cache check, which Real-Debrid has removed.

  The fix is one order-independent matcher. It drops openings, endings and extras, maps season and absolute numbering, checks the name and size Real-Debrid returns, and asks when unsure.
- **Movies & TV titles** get a negative media ID (a hash of the IMDb ID), so Seanime's watch history gives resume and Continue Watching while AniList is never updated. The add-on's link is followed to the final Real-Debrid file. A link that ends anywhere else is a "still downloading" placeholder clip and is skipped.
- **Streaming** goes from Real-Debrid through the in-process server to mpv with no disk cache. mpv reads ahead in large bursts (up to 256 MB, 300 s), so the Wi-Fi radio can sleep in between.
- **Audio** on this laptop is started on demand. The player starts PipeWire when a video opens, and reopens audio by itself if PipeWire is restarted during playback.

## Measurements

On the target laptop on battery, backlight 70%, streaming from Real-Debrid over Wi-Fi, window fullscreen at 2880x1800. Power is the battery's own reading averaged over 20 s, for the whole laptop.

| | |
|---|---|
| Laptop idle, nothing started | 3.6 W |
| media-player open, Movies & TV page, idle | 3.7 W |
| 4K HDR HEVC episode (Ted Lasso, 5 GB), VA-API, tone mapped | 9.0 W, 0 dropped frames |
| 1080p anime with Anime4K A+A (Fast) and debanding (Frieren) | 19.1 W, 0 dropped frames |
| CPU during A+A playback | main process 11%, page renderer 9%, GPU process 1% of one core |
| Server ready after launch | 0.19 s |
| Memory, all 6 processes, idle | 510 MB PSS |
| Movies & TV: streams for an episode / start playing | 0.1–0.2 s / 0.6–1.2 s server side |
| Next episode by autoplay (Movies & TV, same release) | 2.2 s to the new stream |
| Uncached episode out of a 42 GB season pack | about 11 s to playback |

For comparison, [phase 0](docs/phase0-power.txt) measured a bare embedded mpv with A+A (Fast) and debanding on a local file at 15.5 W. The difference is the network stream and the page.

Playback while using the interface, plugged in on the gaming power plan: Frieren episode 1 (1080p) streamed from Real-Debrid, window fullscreen at 2880x1800, the mouse moving over the control bar every 50 ms for 30 s (`appshell/test/bench-playback.mjs`). "Input" is a mouse event's round trip through the window's main loop; "page to screen" is how long a frame of the interface waits until a swap shows it.

| Shaders | GPU per video frame (budget 41.7 ms) | GPU busy | Dropped frames | Input p50 / p95 | Page to screen p50 / p95 |
|---|---|---|---|---|---|
| none (`ewa_lanczossharp`) | 3.4 ms | 22% | 0 | 9.9 / 17.1 ms | 8.2 / 8.3 ms |
| Anime4K A+A (Fast) | 20.9 ms | 64% | 0 | 10.3 / 20.6 ms | 8.2 / 14.0 ms |
| Anime4K A (HQ) | 35.2 ms | 97% | 0 | 12.4 / 31.1 ms | 8.2 / 24.4 ms |

Anime4K A+A (HQ) takes 45 ms per frame, more than a 24 fps frame lasts, and drops frames.

## Verification

- **Real playback with Real-Debrid**, driven through the Chrome DevTools Protocol with screenshots of the real window (`appshell/test/`):
  - Frieren episode 1 from a SubsPlease batch: picture, audio, VA-API, the Anime4K A+A chain (7 shaders), debanding.
  - Autoplay from episode 1 to 2 picked "- 02" from the same batch.
  - Mayonaka Heart Tune episode 3 via auto-select (S01E03, whole torrent cached), then autoplay to 4 reusing the torrent.
  - Ted Lasso S01E01 to E02 by autoplay on the same release; resume at 10:06 after quitting and reopening the program.
  - The Shawshank Redemption in 4K HDR with a seek to 50:00.
- `appshell/test/pause-fs-stress.mjs`: 3 pause cycles, then 3 fullscreen off/on cycles with a pause after each; the final screenshot shows the picture.
- `appshell/test/bench-playback.mjs`: the table in Measurements, with mpv's drop counters and the window's own frame timing (`appshell:stats`).
- **Hidden window:** unmapped for 6 s during playback (as when switching dwm tags), the video went on with 0 dropped frames and showed again at 24 fps within a frame of being mapped.
- **Audio recovery:** PipeWire stopped and started during playback; audio came back on its own about 2 s later.
- **Clean exit:** SIGTERM during playback closes the window in 0.2 s and leaves no helper processes.
- **Wrong episode.** `media-player debug pick` runs the episode-file selection with live AniList data and no playback. On the lists in [`internal/debugpick/lists`](internal/debugpick/lists):

  | Pack | Episode | Upstream plays | media-player plays |
  |---|---|---|---|
  | Attack on Titan Seasons 1-2, watching Season 2 | 5 | S01E05 | S02E05 |
  | Frieren with openings, endings and a mini anime | 1 | Mini Anime - 01 | 01 |
  | Frieren with a v2 | 7 | 07 | 07v2 |
  | Jujutsu Kaisen S2 in absolute numbers (25-47) | 1, 23 | 25, 47 | 25, 47 |

- `go test ./internal/episodematch/`: 21 cases, including seasons in packs, absolute numbering, extras, v2, multi-episode files, OVA entries, and every case again with the file order shuffled 50 times.
- `go test ./internal/debrid/realdebrid/`: links in order, out of order, packed into an archive and renamed, against a fake Real-Debrid server.
- `go test ./internal/stremio/`: episode-file checks (another episode or season refused, absolute anime numbering trusted), stream parsing for Torrentio and Comet, the stream order, and placeholder detection. A live test against Torrentio and Real-Debrid runs when `MP_RD_KEY_FILE` is set.
- **Not verified:** smooth touchpad scrolling. It is implemented with XInput 2.1 scroll valuators, which the touchpad reports, but this kernel has no uinput to simulate a touchpad. The 10-bit path (X at depth 30) is implemented but was not run with X at depth 30. The power figures above were measured on battery before video moved to its own thread.

## Limits

- Linux with X11 only. Wayland is not a target.
- Built for one machine: `GOAMD64=v4` binaries need AVX-512, and the build expects CEF and its libraries in `third_party/`.
- Movies & TV needs Real-Debrid; there is no torrent streaming for it.
- Anime4K A+A doubles the playback power draw. That is the price of the picture; turn it off in the player's Shaders menu when battery matters more.
- The Anime4K presets are timed above with the GPU at full clock. Mode A (HQ) leaves the GPU almost no headroom: the interface stays usable, but at 20–30 ms instead of 10. On a lower power plan, use the Fast presets.
- libmpv's render API runs mpv's `gpu` renderer, not `gpu-next`, so shaders written for `gpu-next` only (ArtCNN, chroma-from-luma prediction) don't load in the built-in player.
- CEF is 272 MB of the install and most of the memory use.

## License

GPL-3.0, as Seanime (see `LICENSE`). Seanime is by [5rahim](https://github.com/5rahim) and its contributors; its original README is kept in [docs/SEANIME-README.md](docs/SEANIME-README.md).
