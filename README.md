# media-player

An anime and movie player for Linux built on [Seanime](https://github.com/5rahim/seanime): Seanime's interface and features, unchanged, in a standalone window instead of Electron, with mpv playing inside it and a Stremio-style Movies & TV section next to the anime. It is made for one laptop (Ryzen AI 9 HX 475, Radeon 890M, 2880x1800 at 120 Hz, Gentoo with dwm) and tuned for it: no new system packages, nothing running when the window is closed, Anime4K and debanding for anime, and Real-Debrid always streaming the episode you asked for.

**Status:** working, used on the target laptop. Anime (debrid, torrent and local files), Movies & TV through Torrentio and Comet, autoplay, resume and Anime4K are all tested in the real program with a Real-Debrid account (see Verification). Built and run on that one machine only.

![The built-in player at 2880x1800 playing Frieren episode 1 with Anime4K Mode A+A and debanding: Fern and Heiter in a lamp-lit town at night with the subtitle "It's nothing.", the show and episode title at the top left, and Seanime's control bar at the bottom with play, next, volume, the time 6:56 / 25:59, and the subtitle, audio, picture-in-picture and fullscreen buttons](docs/images/player.png)

## Features

- **Seanime, 1:1.** Every screen and feature of upstream Seanime: AniList and MAL lists, local library and scanner, torrent and debrid streaming, manga, online streaming, extensions, schedule, Nakama, auto-downloader. Nothing is removed, and features you don't use aren't started. The built-in torrent search providers that upstream moved to extensions (AnimeTosho, Nyaa, SeaDex) are back.
- **A standalone window, no Electron.** The React interface runs in Chromium's engine embedded as a library (CEF) inside this program. There is no browser window and no Node.js at runtime, and the server runs in the same process. Closing the window ends everything.
- **mpv inside the window.** libmpv draws the video and the page is drawn over it, so Seanime's own player (mpvcore) keeps its controls, menus and Anime4K picker exactly as they are.
- **mpv's best renderer.** The video is drawn by mpv's `gpu-next` (libplacebo) on Vulkan, the renderer standalone mpv uses, so every mpv shader works, including those written for `gpu-next` only, such as ArtCNN and chroma-from-luma prediction. Video decoding is on the GPU (Vulkan Video or VA-API).
- **Smooth with shaders.** mpv renders on its own threads at low GPU priority, so the controls, menus and mouse stay at the panel's 120 Hz while Anime4K works: input reaches the page in 11–16 ms during playback, with 0 dropped frames up to Anime4K Mode A (HQ).
- **Picture quality.** Shaders and debanding for anime, chosen in the player's menu (up arrow > Shaders) and off until you pick them. The **Recommended** presets were measured on this laptop: Auto picks a chain by the video (Anime4K A+A with chroma-from-luma for 1080p, HQ chains for 720p and SD, nothing for 4K and HDR), and Crisp, Crisp Light and Clean & Crisp can be chosen by hand. The [shader guide](docs/shaders.md) says which to use for which video. Seanime's Anime4K presets are still there, and so are ArtCNN presets: ArtCNN, ArtCNN Sharp, ArtCNN Denoise (each with chroma-from-luma prediction for the colour) and Chroma only. ArtCNN gives the closest result to the source in the quality test below. Without shaders, video is scaled with `ewa_lanczossharp`. HDR tone mapping with peak detection for everything, and real HDR fullscreen with the X driver patch and `colormode watch` from [x11-amdgpu-hdr](https://github.com/r3clusionn/x11-amdgpu-hdr): the screen switches to HDR while an HDR video is fullscreen and on screen, the video is rendered as PQ for the panel's peak, and the controls and subtitles stay at the desktop's white. With X at depth 30 the video reaches the panel in 10 bits per channel. Movies & TV is live action: no Shaders entry, no Anime4K, no debanding.
- **The right episode.** A rewritten episode matcher for debrid batches (see How it works) is used by autoplay, resume and manual picks. When it isn't sure, it shows the file picker instead of guessing. Movies & TV refuses a file whose name says another season or episode.
- **Torrent providers.** Anime searches SeaDex by default with AnimeTosho (and Nyaa) one click away in the torrent drawer. Movies & TV asks Torrentio by default or Comet if you pick it, with the other as the backup.
- **Movies & TV.** Catalogs, search and details from Cinemeta. Real-Debrid streams come from the chosen add-on, and from the other one when it fails or has nothing cached. You get Continue Watching and Next Up, resume, and next-episode autoplay that stays on the same release. The pages are built from Seanime's own components.
- **Only one file from big packs.** For an uncached 40 GB season pack, Real-Debrid fetches only the episode being played, so playback starts in seconds. An already-cached or already-added torrent is reused.

## How to install

Requires Go 1.27, Node.js 26 with npm, gcc, libmpv (mpv 0.41, with Vulkan), yyjson, and the X11 libraries (libX11, libXi, libXss, libXext, libXcomposite, libXdamage) with EGL and OpenGL 4.6. Linux with X11 only.

The build also needs three directories in `~/src/third_party` (or `$THIRD_PARTY`), none installed system-wide:

- `cef/`: the CEF 154.0.34 "minimal" Linux x64 build from the [official CEF builds](https://cef-builds.spotifycdn.com/index.html), extracted.
- `runtime/`: NSS, NSPR and xkbcommon, which CEF needs and this Gentoo install doesn't have (copied from a Chromium-based browser's bundled libraries).
- `anime4k/`: the [Anime4K](https://github.com/bloc97/Anime4K) v4 GLSL shaders, plus `ArtCNN_C4F16.glsl`, `ArtCNN_C4F16_DS.glsl` and `ArtCNN_C4F16_DN.glsl` from [ArtCNN](https://github.com/Artoriuz/ArtCNN) and `CfL_Prediction.glsl` from [glsl-chroma-from-luma-prediction](https://github.com/Artoriuz/glsl-chroma-from-luma-prediction).

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

**Anime4K and debanding** are in the player's settings menu (up arrow > Shaders), off until chosen and then remembered. Recommended > Auto is the simplest choice; the [shader guide](docs/shaders.md) covers the rest. Stats for Nerds (Z) shows which shaders run and their GPU time per frame. Movies & TV doesn't offer them.

| | |
|---|---|
| `media-player` | Open the window. |
| `media-player --server` | Run only Seanime's web server (open `http://127.0.0.1:43211` in a browser). |
| `media-player debug pick` | Run the episode-file matcher on saved lists, without playback. |
| `APPSHELL_DEBUG=1` | Log the page console, mpv warnings, and paint and draw rates to stderr. |

## How it works

- **The window** is one X11 window with a desktop OpenGL 4.6 context (EGL), at the screen's depth: 10 bits per channel when X runs at depth 30. CEF's GPU process uses ANGLE over EGL (`--use-angle=gl-egl`): over GLX it can't bind the page's 8-bit buffers on a depth-30 screen, which has no depth-24 visuals. CEF renders the page off-screen and hands each changed frame over as a GPU buffer (dmabuf), imported with `EGL_EXT_image_dma_buf_import`. Each frame is the video, then the page blended on top. Nothing is redrawn while nothing changes, and the swap is skipped while the window is hidden.
- **HDR.** While an HDR file plays, the window sets `_COLORMODE_HDR_VIDEO`; when `colormode watch` answers with `_COLORMODE_HDR_OUTPUT` (the screen is in HDR mode), every player's mpv switches to `target-trc=pq`, `target-prim=bt.2020`, the panel's peak and the SDR white (asynchronously, so a busy mpv can't hold up the window), and the compositor draws the page and popups through a shader that turns sRGB into PQ at the SDR white. When the screen goes back to SDR, mpv's own settings come back. `appshell/test/hdr-play.mjs` plays a local file through the player for testing.
- **Video through Composite.** libmpv's render API only runs mpv's older `gpu` renderer, so each player is instead a whole mpv with its own `gpu-next` video output, drawing into a child window the size of the player on the page. That window is redirected off-screen (X Composite): X keeps what mpv draws in a pixmap instead of putting it on screen, Damage reports each new frame, and the pixmap is imported as an EGLImage and drawn under the page, without a copy. The window takes no input (an empty input shape), so the page gets the mouse and keyboard. mpv paces its own frames, so a 20–35 ms Anime4K frame never holds up the page or input; while the player isn't visible the window is unmapped and mpv stops drawing, with playback and audio going on.
- **Closing a player** hides it at once and shuts mpv down on a thread of its own; the window is removed once mpv has finished. mpv can take long to quit (a network read waiting for its timeout after the stream was closed), and the window keeps running meanwhile.
- **Frame timing.** mpv times frames by the audio clock. The panel refreshes at 119.88 Hz, exactly 5 refreshes per frame of 23.976 fps video (4 per frame of 29.97 fps), so every frame stays on screen equally long. mpv's `display-resample` mode, used before, drew the frame again on every refresh, which cost about 8 W, and held frames for 4 or 6 refreshes once a shader frame took longer than one refresh ([measurements](docs/shaders.md#frame-timing)).
- **GPU priority.** Mesa creates every GPU context in the process at low priority once the window's own context exists (`AMD_PRIORITY`, set after it and cleared in CEF's helper processes), so mpv runs at low priority and the window's and the page's small frames are scheduled ahead of a long Anime4K frame.
- **The page talks to the player** through the JavaScript interface Seanime's Electron app provides (`window.electron` and the mpv-prism calls). A small script injected by the helper process implements it over CEF process messages, so the React code is unchanged. Every mpv request is asynchronous, so the window never waits on mpv.
- **Video under the page.** The player's ancestors are made transparent by a CSS `:has()` rule, not by classes, because React rewrites class names when it re-renders. Page content painted beneath the player gets a mask with a hole the video's shape (rounded like the mini player's frame); the player's own menus and popovers, painted above it, are left alone.
- **Video over the page while it moves.** Chromium scrolls the page and runs transform animations on its compositor, so a hole cut from the page's script trails them by a frame or more (a black ghost of the old position). Holes are therefore cut only while the video stands still and either fills most of the window or has the pointer over it (a mini player's controls must then show over it); otherwise the window draws the video over the page, clipped to the frame's rounded corners, and goes back under it once the page has painted the holes. The script reports the player's rectangle every animation frame while anything moves (`getBoundingClientRect` includes transforms). Switching between the mini player and fullscreen runs without a view transition while a video is presented (Chromium would animate snapshots while the real layout, and so the video, jumps to the end); the drawer's own FLIP animation runs instead and the video follows it. mpv's window takes a new size only once it has settled; meanwhile the last frame is scaled, aspect kept, until mpv has drawn the new size twice.
- **mpv's window size before the video starts.** Until mpv reports `vo-configured`, its own window stays at its 320x200 default; the video window is held at exactly that size, since any other size made libplacebo recreate the swapchain on every wakeup while the decoder started, until X refused and the video stayed black (only the first play after a reboot worked). The hidden control bar is skipped with `content-visibility`, so the page stays idle during playback instead of restyling an invisible seek bar.
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
| none (`ewa_lanczossharp`) | 2.4 ms | 27% | 0 | 10.9 / 17.2 ms | 8.2 / 8.3 ms |
| Anime4K A+A (Fast) | 19.7 ms | 69% | 0 | 13.6 / 25.3 ms | 8.2 / 14.5 ms |
| Anime4K A (HQ) | 33.7 ms | 97% | 0 | 16.2 / 33.9 ms | 8.1 / 24.6 ms |
| ArtCNN (C4F16 + chroma-from-luma) | 24.3 ms | 70% | 0 | 12.9 / 25.7 ms | 8.1 / 15.2 ms |
| Chroma only (chroma-from-luma) | 6.2 ms | 40% | 0 | 11.0 / 23.4 ms | 8.1 / 8.4 ms |

This table was measured with mpv's `display-resample` timing; with the audio timing used now, Anime4K A+A keeps the GPU 62–64% busy instead of 79–80%. The [shader guide](docs/shaders.md) has the current numbers for each preset. GPU time is mpv's own measurement (`vo-passes`) and includes time its passes wait behind the window's frames; the ArtCNN rows had debanding on. ArtCNN Sharp and Denoise cost the same as ArtCNN. Anime4K A+A (HQ) (50 ms per frame) and ArtCNN C4F32 (53 ms) take longer than a 24 fps frame lasts and drop frames, so C4F32 isn't offered. While watching without touching anything, every video frame reaches the screen (719 of 719 in 30 s with A+A); while the mouse keeps the controls repainting, about 1 frame in 100 is replaced by the next before it is shown.

### Picture quality test

Three frames of Frieren episode 1 (1080p) were halved to 960x540 with Lanczos, then doubled back to 1920x1080 by each method in mpv `gpu-next`, and the result compared with the original frame (higher is closer):

| Method | PSNR | SSIM |
|---|---|---|
| ArtCNN (C4F16 + chroma-from-luma) | **37.08 dB** | **0.9700** |
| ArtCNN Denoise | 37.01 dB | 0.9631 |
| Anime4K C (HQ) | 36.92 dB | 0.9663 |
| Lanczos (mpv's default) | 36.70 dB | 0.9663 |
| `ewa_lanczossharp` with chroma-from-luma | 36.41 dB | 0.9652 |
| `ewa_lanczossharp` | 36.40 dB | 0.9647 |
| ArtCNN Sharp | 35.51 dB | 0.9615 |
| Anime4K A (HQ) | 33.15 dB | 0.9414 |
| Anime4K A (Fast) | 32.56 dB | 0.9396 |
| Anime4K A+A (Fast) | 29.99 dB | 0.9197 |

Side by side, ArtCNN keeps the painted texture of backgrounds and gives crisp lines; Anime4K A and A+A draw bolder lines but smooth the background art flat, which is a look some prefer, not more detail. The test favours faithful reconstruction, which is what ArtCNN is trained for.

## Verification

- **Real playback with Real-Debrid**, driven through the Chrome DevTools Protocol with screenshots of the real window (`appshell/test/`):
  - Frieren episode 1 from a SubsPlease batch: picture, audio, VA-API, the Anime4K A+A chain (7 shaders), debanding.
  - Autoplay from episode 1 to 2 picked "- 02" from the same batch.
  - Mayonaka Heart Tune episode 3 via auto-select (S01E03, whole torrent cached), then autoplay to 4 reusing the torrent.
  - Ted Lasso S01E01 to E02 by autoplay on the same release; resume at 10:06 after quitting and reopening the program.
  - The Shawshank Redemption in 4K HDR with a seek to 50:00.
- `appshell/test/pause-fs-stress.mjs`: 3 pause cycles, then 3 fullscreen off/on cycles with a pause after each; the final screenshot shows the picture.
- `appshell/test/bench-playback.mjs`: the table in Measurements, with mpv's drop counters and the window's own frame timing (`appshell:stats`).
- **X at depth 30:** the window and mpv's Vulkan swapchain are 10 bits per channel (mpv dithers to 10); every measurement above was taken at depth 30.
- **HDR:** an HDR10 clip played fullscreen through the player switched the screen to HDR; mpv reported `target-trc=pq`, `target-prim=bt.2020`, `target-peak=1107`, `hdr-reference-white=347`, and a white label on the page was drawn as 0.63 (PQ for the 347-nit desktop white) and looked normal.
- **Hidden window:** unmapped for 6 s during playback (as when switching dwm tags), the video went on with 0 dropped frames and showed again at 24 fps after being mapped.
- **Recommended presets:** Auto loaded Anime4K A+A Fast with chroma-from-luma for the 1080p Frieren stream, A+A HQ when mpv scaled it to 720p, C+A HQ at 480p and no shaders when the video was marked HDR, and Stats for Nerds named each chain. With Crisp, 478 of 478 frames stayed on screen exactly 5 refreshes, with 0 dropped ([shader guide](docs/shaders.md)).
- **ArtCNN presets:** choosing ArtCNN in the Shaders menu loaded `CfL_Prediction.glsl` and `ArtCNN_C4F16.glsl` into mpv; every preset played with 0 dropped frames.
- **Closing during playback:** destroying a player returns in 2 ms whether its video is visible or hidden; closing from the mini player and starting another episode afterwards left one mpv video thread running.
- **A new player in the same window:** starting the episode again replaced the player; the new one showed 24 frames a second with 0 dropped.
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
- **Not verified:** smooth touchpad scrolling. It is implemented with XInput 2.1 scroll valuators, which the touchpad reports, but this kernel has no uinput to simulate a touchpad. The power figures above were measured on battery before video moved to its own thread.

## Limits

- Linux with X11 only. Wayland is not a target.
- Built for one machine: `GOAMD64=v4` binaries need AVX-512, and the build expects CEF and its libraries in `third_party/`.
- Movies & TV needs Real-Debrid; there is no torrent streaming for it.
- Anime4K A+A doubles the playback power draw. That is the price of the picture; turn it off in the player's Shaders menu when battery matters more.
- The Anime4K presets are timed above with the GPU at full clock. Mode A (HQ) leaves the GPU almost no headroom: the interface stays usable, but at 16–34 ms instead of 11–17. On a lower power plan, use the Fast presets.
- The Auto preset decides by the video's size and HDR only. It can't tell a rough rip from a clean release; choose Clean & Crisp for those.
- Resizing the player (entering or leaving fullscreen) makes mpv rebuild its Vulkan swapchain, which can drop a frame or two.
- CEF is 272 MB of the install and most of the memory use.

## License

GPL-3.0, as Seanime (see `LICENSE`). Seanime is by [5rahim](https://github.com/5rahim) and its contributors; its original README is kept in [docs/SEANIME-README.md](docs/SEANIME-README.md).
