# Laptop media and gaming apps: master plan

Written 2026-10-11. Status: **plan only, nothing built yet.**

Machine: HP OmniBook 7, Ryzen AI 9 HX 475, Radeon 890M, 64 GB RAM, 2880x1800 120 Hz OLED. Gentoo with 256 packages, OpenRC, Xorg + dwm, PipeWire on demand.

## 0. Decisions so far

| # | Decision | Chosen |
|---|---|---|
| D1 | App form | Standalone programs. No Thorium window, no Electron. |
| D2 | Seanime UI | **CEF**: Chromium's engine embedded as a library in our own program, running Seanime's real React UI 1:1. A custom GPU-drawn UI (Gio) is noted for later in §9. |
| D3 | Stremio replacement | A **Movies & TV section inside the Seanime fork**, same UI, same player |
| D4 | Debrid | Real-Debrid for both anime and movies/TV |
| D5 | Player | libmpv inside the app. Seanime's own player UI (VideoCore in "mpvcore" mode) is kept 1:1. |
| D6 | Anime quality | Anime4K Mode A+A + debanding (owner's current setup), chosen automatically per source |
| D7 | Minecraft | **New Go launcher**, 0 packages, Mojang-downloaded Java. Not a Prism fork. |
| D8 | Music player | Native C + GTK3 + libmpv (runs for hours, so it stays native) |
| D9 | Package budget | **0 new Portage packages** (see §8) |
| D10 | Names | `media-player` (the Seanime fork with Movies & TV), `music-player`, `mclaunch`. Seanime stays credited as upstream. |
| D11 | Seanime features | **Keep everything.** Nothing is trimmed; unused parts just don't start until used. |
| D12 | Stream add-ons | **Torrentio** first, **Comet** as backup |
| D13 | Battery | Keep Anime4K A+A on battery too |
| D14 | Hosting | Each project gets its own GitHub repo, all with the same README layout |

## 1. Ground rules and targets

- **Must-keep list** (the debloat skill's `reference/system.md`) stays working after every phase. These apps are already on it as "libmpv apps".
- **Every phase ends with measurements:** idle RAM, process count, watts while playing (`~/claude/battery-test.sh`), and startup time. Numbers come from runs, never guesses.
- **Audio is on demand:** PipeWire only runs after F2 or `audio ensure`. mpv is built with `-alsa -pulseaudio`, so PipeWire is its only audio output. **Every app runs `/usr/local/bin/audio ensure` before playing anything.**
- **mpv is built `-lua`:** no Lua scripts, no `profile-cond`. Anything automatic (profile switching, skip intro) happens in our own code through the libmpv client API.
- **HiDPI is a fixed 2x** (Xft.dpi 192). The CEF device scale factor is set to 2.0, an integer, which avoids CEF's known fractional-scale bug.
- **Root changes:** handed to the owner as a script (doas needs the password). Only one is planned: the launcher's power-profile rule (§6.7).

### Targets (to be measured, not promised)

| App | Idle with window open | While playing | Cold start |
|---|---|---|---|
| media-player (browsing) | ≤ 300 MB total across all its processes, 0% CPU when nothing moves | — | ≤ 1.5 s to first screen |
| media-player (anime, A+A) | — | Same or less power than Denshi with the same shaders | Episode starts ≤ 3 s after the click (RD link cached) |
| Music | ≤ 30 MB with a 20k-track library | ≤ 0.5 W above system idle | ≤ 0.3 s |
| Launcher | ≤ 250 MB while open; **0 MB while the game runs** (the UI exits) | — | ≤ 1.5 s |

## 2. Shared foundation

### 2.1 Source layout (all in `~/src`, each a git repo)

```
~/src/
  media-player/ fork of github.com/5rahim/seanime; our work on "main"; remote "upstream"
  appshell/     shared host: CEF + X11/EGL window + libmpv compositor + JS bridge (C, used from Go via cgo)
  mclaunch/     Minecraft launcher (Go backend + React UI, runs in appshell)
  music-player/ music player (C, GTK3, libmpv, ffmpeg, sqlite)
  third_party/  CEF SDK, bundled NSS/NSPR/xkbcommon libs, Anime4K shaders, Inter font
  tools/        update-apps.sh, bundle-libs.py (from ~/claude/thorium), measurement scripts
```

### 2.2 Toolchain (all already installed)

- **Go 1.27**
  - `GOAMD64=v4` (Zen 5 has AVX-512).
  - `-trimpath -ldflags="-s -w"`.
  - Profile-guided optimization: a `default.pgo` profile is recorded from real use, then rebuilt with it.
  - `CGO_CFLAGS="-O2 -march=native"`.
- **Node 26 / npm:** frontend builds only. `node_modules` lives in the repo, never in Portage.
- **gcc, meson, ninja, pkgconf**
  - The C parts build with `-O2 -march=native` and `b_lto=true`, matching the system.
  - CMake is **not** installed and isn't needed. CEF's C API is used directly, so its CMake-built C++ wrapper isn't required.

### 2.3 Install layout

- **Binaries:** `~/.local/opt/<app>/`, symlinked into `~/.local/bin/<app>`, so dmenu finds them.
- **Data:**
  - `~/.local/share/<app>/`: databases and Java runtimes.
  - `~/.cache/<app>/`: CEF cache, images, shader cache.
  - `~/.config/<app>/`: settings.
- **dwm:** optional keybinds and a rule per window class (`media-player`, `music-player`, `mclaunch`). Fullscreen uses `_NET_WM_STATE_FULLSCREEN`, which dwm already handles.

### 2.4 Updating

`tools/update-apps.sh`:
1. Fetches upstream Seanime and rebases `main` onto it. Conflicts stop the script; it never forces through them.
2. Rebuilds the frontend and backend.
3. Refreshes the CEF SDK when a new stable version is out (security fixes).
4. Re-runs `bundle-libs.py` for NSS/NSPR/xkbcommon.
5. Rebuilds music-player and mclaunch.
6. Prints a summary.

It doesn't run on its own. Updates happen when the owner runs it, like `doas update`.

## 3. appshell: the CEF + libmpv host (used by Seanime and the launcher)

This is the piece that replaces Electron. It's one shared library that both apps embed.

### 3.1 Process layout

| Process | What it is | Notes |
|---|---|---|
| Main | Our Go program: app backend, window, CEF's browser process, libmpv | libmpv runs inside it as a library, not as a separate process |
| `appshell-helper` | Tiny C program that CEF starts for its child processes | Keeps the Go runtime out of the renderer and GPU processes |
| CEF GPU process | Draws the page on the GPU | |
| CEF renderer | Runs the React UI | Limited to **1** (`--renderer-process-limit=1`; per-site isolation off for a trusted local app) |

- **Network service in-process** where CEF allows it.
- **`--no-sandbox --no-zygote`:** removes the zygote processes. The owner already accepted dropping hardening, and the content is local and trusted.
- **Expected total:** 3–4 processes while the window is open, and **0 after it closes**. There's no tray icon and no daemon.

### 3.2 Window and drawing

1. **Window:** one plain X11 window (libX11) with an EGL context (libglvnd/mesa). No GTK for the main window.
   - GTK3 is still used for CEF's file picker, with the same `GDK_SCALE=2 GDK_DPI_SCALE=0.5` fix as Thorium.
2. **Page:** CEF runs **off-screen** (`windowless_rendering_enabled`, `shared_texture_enabled`).
   - Each time the page changes, CEF hands over a **GPU buffer** (dmabuf) through `OnAcceleratedPaint`.
   - We import it with `EGL_EXT_image_dma_buf_import`, which mesa supports on X11.
3. **Video:** libmpv's **render API** (OpenGL) draws video frames into the same EGL context. This is what Seanime's own mpv-prism does on Linux (EGL + dmabuf).
4. **Each displayed frame:** draw the video (if playing) → blend the page on top with alpha (the player page is transparent where the video is) → swap buffers with vsync at 120 Hz.
5. **Redraw only when something changes:** mpv's update callback, a CEF paint, or a resize. A still screen costs **0 redraws**.
6. **Off-screen frame rate:** capped at 120. CEF only paints when the page actually changes.

### 3.3 Input

Because the page is off-screen, we forward input to CEF ourselves.

- **Mouse and keyboard:** X11 events → `SendMouseMoveEvent`, `SendMouseClickEvent`, `SendKeyEvent`.
  - Key translation uses a keysym→Windows-keycode table, taken from cefclient's `KeyboardCodeFromXKeysym`.
- **Smooth touchpad scrolling:** XInput2 smooth scrolling (libXi is installed). This matters for the feel at 120 Hz.
- **Cursor shapes:** `OnCursorChange` → `XDefineCursor` (libXcursor).
- **Clipboard:** CEF's own X11 handling. Verified in phase 0.
- **While a video plays:** keys go to the page first, so VideoCore's shortcuts work exactly as in Denshi.

### 3.4 JavaScript bridge (what keeps the UI 1:1)

Seanime's UI expects Electron's preload APIs. The helper defines the same objects in the page (a V8 extension in the renderer), and they forward to the main process over CEF process messages.

- **`window.electron.*`:** the subset Seanime actually calls (window controls, fullscreen, open external link, the 9 `mpvcore:*` calls in Denshi's `preload.js`).
- **The mpv-prism API:** about 33 channels, read from `@mpv-prism/electron` 0.1.9 (LGPL-3.0). Groups:
  - **Lifecycle:** create, load, destroy.
  - **Commands:** command, get/set/observe property, play, pause, seek, set speed, select track, set shaders.
  - **The "video presenter":** start, stop, resize, suspend, resume.
  - **Implementation:** the presenter isn't a texture shared into the page. Instead, its **bounds** tell the compositor where to draw the video underneath. The page stays transparent there.
- **Property updates:** observed mpv properties (time position, pause, tracks, cache) are pushed to the page about 10 times a second, plus on change. They aren't polled.
- **Fallback:** mpv-prism's CPU-frame channels (`video-cpu-frame-*`) stay as a slow backup path if GPU import ever fails.

### 3.5 How the page reaches the Go backend

- The Seanime backend runs **in the same process**, with its HTTP server bound to `127.0.0.1` on a random port, behind a random per-launch password. Seanime already supports a server password, and the shell injects it.
- The React code doesn't change: same `fetch` and websocket calls.
- A no-TCP custom URL scheme was considered. Websockets don't work over custom schemes, so localhost it is.

### 3.6 CEF settings for low overhead

```
--renderer-process-limit=1  --disable-site-isolation-trials  --no-sandbox --no-zygote
--disable-background-networking  --disable-component-update  --disable-breakpad
--disable-domain-reliability  --no-pings  --disable-spell-checking  --disable-extensions
--disable-features=Translate,MediaRouter,OptimizationHints,AutofillServerCommunication,
                   CalculateNativeWinOcclusion,BackForwardCache,InterestFeedContentSuggestions
--enable-gpu-rasterization --enable-zero-copy --ignore-gpu-blocklist
--force-device-scale-factor=2
```

- **Cache:** `~/.cache/media-player/cef` with a size limit.
- **When the window is hidden** (dwm unmaps windows on other tags): `WasHidden(true)` throttles the renderer.

### 3.7 CEF binary

1. **First try: the prebuilt "minimal" CEF** from the official CEF builds (Spotify CDN).
2. **Bundled libraries:** NSS, NSPR and libxkbcommon come from Gentoo's binhost into `third_party/`, the same way as `~/claude/thorium/bundle-libs.py`.
3. **CUPS:** the prebuilt CEF probably links `libcups.so.2`, which isn't installed. Phase 0 checks with `ldd`. If it does:
   - **(a) Stand-in library (preferred):** a tiny do-nothing `libcups.so.2` with the symbols CEF references, all returning "no printers". Printing is never used.
   - **(b) Build CEF ourselves** with `use_cups=false`, using the Thorium build setup in `~/claude/thorium` (`env.sh`, the same Zen 5 settings). This takes several hours and about 100 GB of temporary disk.
4. **Final fallback** if GPU-buffer off-screen rendering fails on X11 with the chosen CEF:
   - Run CEF in normal windowed mode for browsing.
   - Use a native mpv child window for playback, with controls drawn by mpv's on-screen display.
   - This loses only the 1:1 *player controls*. Browsing stays 1:1.

## 4. media-player (Seanime fork)

### 4.1 Fork strategy

- Our work is on `main`, rebased onto upstream releases (remote `upstream`). Small, separate commits so rebasing stays painless.
- **Builds:**
  - Frontend: `npm ci && npm run build` → `web/`.
  - Backend: `go build -tags=nosystray` with appshell linked in.
  - The upstream web-server mode still works, which makes debugging easy.
- Seanime and mpv-prism are GPL-3 and LGPL-3. Personal use, source kept locally.

### 4.2 Removed or turned off

- **Electron app (`seanime-denshi/`)**, updater, system tray, and Discord rich presence.
- **Services start only when first used:**
  - The anacrolix torrent client (DHT, peer connections) starts only when torrent streaming is used. Debrid-only use never starts it.
  - The goja plugin runtimes start only if extensions are enabled.
  - The library file watcher and scanner run only if a local library folder is set.
- **Background timers** (`internal/cron`): the AniList refresh runs on window focus plus a long interval, not on a short timer. Auto-downloader off unless used.
- **No features removed** (owner's choice). Manga, online streaming, Nakama, qBittorrent/Transmission, MAL sync, extensions, local library, auto-downloader and the rest all stay. They don't start until you first use them, so unused features cost no RAM or startup time.

### 4.3 Backend optimizations

- **SQLite:** WAL mode, `synchronous=NORMAL`, memory-mapped I/O. Indexes on whatever the slow queries turn out to be (measured, not guessed).
- **HTTP:** shared HTTP/2 clients with keep-alive for AniList, Real-Debrid and add-ons. AniList responses are cached in SQLite with fixed lifetimes.
- **Images:** posters and banners cached on disk at display size (2x), so nothing is decoded twice.
- **Go memory:** `GOMEMLIMIT` around 200 MiB; garbage-collection tuning measured.
- **Instant next episode:** at 85% of the current episode, the next file is identified and its RD link resolved, so autoplay starts instantly.
- **Torrent reuse:** RD torrent IDs are remembered per (torrent hash, show), so the next episode from the same batch reuses the torrent already on the RD account instead of adding it again.

### 4.4 The wrong-episode bug: diagnosis and fix

**Symptom:** with Real-Debrid streaming, the wrong episode plays whenever the file is chosen automatically (resume watching, autoplay next). Choosing the file by hand works.

**Suspects found in upstream code** (commit `2da73d9`, 2026-09-20):

| # | Where | Problem |
|---|---|---|
| A | `seanime-web/src/app/(main)/_features/video-core/video-core-playlist.tsx` (debrid autoplay branch) | Plays the file at `current + 1` in the list, **ignoring the episode number**. It also sends that list position as the **RD file ID** (`chosenFileId: String(fileIndex)`). Openings and endings (NCOP/NCED), specials, subtitle and extra files, or "Ep 10" sorting before "Ep 2" all make it pick the wrong file. The torrent-stream branch right above it does this properly (`getBatchSelectionParams` with the episode number). |
| B | `internal/debrid/client/finder.go` (`findBestTorrentFromManualSelection`, auto-select path) | Looks up the season's episode number (`opts.EpisodeNumber`) as if it were the overall number: `GetFileByAniDBEpisode(strconv.Itoa(episodeNumber))`. Sequels in absolute-numbered batches (S2E01 = episode 13) and seasons with specials pick the wrong file. Resume also re-runs auto-select, which can pick a **different torrent** with different numbering. |
| C | `internal/debrid/realdebrid/realdebrid.go` (`GetTorrentStreamUrl`, `GetTorrentDownloadUrl`) | Picks a file's download link by its **position** among the selected files (`ti.Links[idx]`). RD doesn't guarantee its link list follows that order. Any mismatch gives a different episode with no error. |
| D | `realdebrid.go` → `GetInstantAvailability` | RD [shut off this endpoint in Nov 2024](https://torrentfreak.com/real-debrid-implements-extreme-anti-piracy-filters-to-appease-film-companies-241122/), so "prefer cached torrents" sorting has nothing to work with. Auto-select may pick slow uncached torrents, and behaviour differs from manual picks. |

**Fix plan:**

1. **Reproduce first.**
   - Add a small command-line tool, `media-player debug pick --media <anilistId> --ep <n> [--hash <infohash>]`. It runs the same selection code without starting playback and prints every candidate file with its parsed season, episode and absolute number, plus the final choice.
   - Turn on debug logging. Run it against the show where the bug happened.
2. **One shared episode matcher** (`internal/episodematch`), used by anime autoplay, resume, manual auto-select and Movies & TV season packs:
   - **Parsing:** every file's name and path, using Seanime's existing parser (habari) plus Jellyfin's TV naming rules ported as extra patterns (`Emby.Naming`: season/episode, absolute, `S01E01`, `1x01`, `Ep01`, `- 01`, multi-episode `E01-E02`).
   - **Throw out non-episodes:** NCOP/NCED/Creditless/PV/CM/Menu/Preview/Recap, "Extras" and "SPs" folders, sample files, non-video files.
   - **Map** season-relative ↔ absolute numbering using AniList relations plus the anime-ID mapping Seanime already uses. A file matches when either numbering lines up and the season is consistent.
   - **Order-independent:** never rely on file order or "previous + 1".
   - **Confidence score:** below the threshold → **show the file picker** instead of guessing.
3. **Fix A:** the debrid autoplay branch uses the same batch-selection helper as torrent streaming (episode-based), and sends the real RD file ID (`files[i].id`), never a list position.
4. **Fix B:** resume and autoplay first reuse the **torrent from the last episode** (saved batch history) before searching again. The matcher is given the right numbering (season vs absolute, with the anime-ID mapping).
5. **Fix C:** after RD returns a download link, check the **file name and size** it reports against the file we asked for. If they differ, search the torrent's other links for the right one. If none match, re-add with only that file selected (`selectFiles=<id>`).
6. **Fix D:** replace the dead cache check.
   - **Candidates:** the top few torrents in preference order.
   - **Probe:** add the magnet, select the files, and look at the status. "downloaded" right away means cached. Otherwise delete it.
   - **Remember results** for 24 hours, so auto-select prefers cached releases again without pounding the API.
7. **Tests:**
   - A table of real release names: batches with extras, absolute-numbered sequels, multi-season packs, v2 re-releases, specials and OVAs, and different groups' naming styles.
   - Each expects one exact file or "ask the user".
   - Plus Real-Debrid response tests (link order shuffled, extra links).
8. **Acceptance:**
   - 0 wrong picks across the whole test set and across 3 real shows played start to finish with autoplay.
   - Every uncertain case shows the picker.

### 4.5 Player quality and power (mpvcore profiles, chosen in code)

There's no Lua and no `profile-cond`, so the Go side reads `width`, `height` and the content type after a file loads and applies the right shaders and options before the first frame.

**Base (every video):**
```
hwdec=vaapi            # VA-API → EGL import with no copy, inside the render API
deband=yes  deband-iterations=2 deband-threshold=35 deband-range=16 deband-grain=4   # starting values (the old Denshi settings are gone); tuned by eye in phase 3
dither-depth=auto
gpu-shader-cache-dir=~/.cache/media-player/mpv-shaders     # Anime4K compiles once, not every launch
video-sync=audio       # 120 Hz is an exact multiple of 24/30/60, so no judder or interpolation needed
cache=yes demuxer-max-bytes=512MiB demuxer-max-back-bytes=128MiB demuxer-readahead-secs=300
                       # downloads in big bursts so the Wi-Fi radio sleeps in between; 64 GB RAM makes this cheap
embeddedfonts=yes sub-ass-override=no   # typeset anime subtitles look exactly as authored (libass)
```
- `profile=fast` from the current `mpv.conf` is **not** used here. It turns debanding off.

**Which profile, chosen automatically:**

| Content | Source | Applied |
|---|---|---|
| Anime | ≤ 1080p | **Anime4K Mode A+A (Fast)**, which is what Seanime's player loads for "A+A" by default: Clamp_Highlights → Restore_CNN_M → Upscale_CNN_x2_M → Restore_CNN_S → AutoDownscalePre_x2 → AutoDownscalePre_x4 → Upscale_CNN_x2_S. Plus deband. 21.5 ms per frame at 2880x1800. The HQ chain (VL shaders) takes 51.3 ms and can't hold 23.976 fps on the 890M; A (HQ) at 35.9 ms is offered as "tight" (docs/phase0.md). |
| Anime | 1440p | Anime4K Restore_CNN_M + `scale=ewa_lanczossharp` (only 1.25x to 1800p, so a 2x upscaler is wasted work) |
| Anime | ≥ 2160p | No upscalers. `dscale=mitchell correct-downscaling=yes linear-downscaling=yes` + deband |
| Live action | any | `scale=ewa_lanczossharp cscale=ewa_lanczossharp`, deband lighter (`threshold=20`), never Anime4K. HDR → SDR: `tone-mapping=spline hdr-compute-peak=yes` |
| Battery | — | **Same as AC** (owner's choice: A+A stays on). Lighter options still exist in settings in case they're ever wanted. Battery-life impact is measured and written down. |

- **Battery detection:** reads `/sys/class/power_supply/ACAD/online` at load time, and when the power source changes during playback.

**To verify in phase 0:**
- Whether mpv 0.41's render API draws with the newer `gpu-next` renderer or the older `gpu` one. Anime4K and deband are identical on both; HDR tone mapping and Dolby Vision profile 5 are better on `gpu-next`.
- If the render API is still `gpu`, Dolby Vision titles get a fallback: a separate fullscreen mpv window with `vo=gpu-next`, still driven by the same app and controls over mpv's control interface.

**Also measure in phase 0:** GPU load and watts for A+A on 1080p → 2880x1800, to confirm the 890M holds 24 fps with headroom, and compare against Denshi on the same file.

**Skip intro/outro:** AniSkip API. The button is drawn by VideoCore, as in Denshi.

### 4.6 Movies & TV section (Stremio-style)

**Backend: `internal/stremio`**

- **Add-on manager:** install by manifest URL; list and remove; settings stored in SQLite. Implements the open Stremio add-on protocol: `manifest.json`, `/catalog/{type}/{id}.json` (+ search/skip extras), `/meta/{type}/{id}.json`, `/stream/{type}/{id}.json`, `/subtitles/...`.
- **Metadata:** Cinemeta (default, no key). Optional TMDB key for better artwork, cast and air dates. IMDb IDs are the main key.
- **Library and progress** (local SQLite):
  - **Continue Watching:** Jellyfin's resume thresholds: resumable between 5% and 90% played, and at least 5 minutes in. Past 90% = watched.
  - **Next Up:** like Jellyfin's: for each series with progress, the next unwatched aired episode after the last watched one; specials (season 0) skipped.
  - **Trakt sync:** optional, later.
- **Streams through Real-Debrid:**
  - **Add-ons used:** **Torrentio** first. **Comet** is asked only when Torrentio fails, times out (4 s), or finds nothing cached for the title. Results are merged and de-duplicated by torrent hash.
  - **Primary path:** both add-ons configured *with* the RD key. They keep their own cache data, so the "cached ⚡" markers still work after RD killed instant availability.
    - Play the add-on's link directly.
    - **Then check** the final file name (after redirects, from `Content-Disposition` or the URL) with the §4.4 episode matcher. If it's the wrong episode, re-resolve ourselves.
  - **Re-resolve path:** take the stream's torrent hash and file number (`infoHash`/`fileIdx`) and go through our own RD client with the §4.4 checks.
  - **Pre-resolve** the next episode at 85%, same as anime.
- **Subtitles:** add-on subtitles (e.g. OpenSubtitles add-on) are loaded into mpv with `sub-add`.

**Frontend: new routes in `seanime-web`, built from Seanime's own components**

- **Movies & TV home:** rows for Continue Watching, Next Up, then one row per add-on catalog (popular movies/series, etc.). Same row and card components as the anime home.
- **Search:** across every catalog that supports search.
- **Title page:** Seanime's entry header (banner, poster, info). A movie shows its streams list. A series shows season tabs → episode grid (Seanime's episode card) → streams list (Seanime's torrent/stream list styling, with resolution/size/cached badges).
- **Add-ons page:** in Settings.
- **Sidebar:** a "Movies & TV" entry next to Anime.
- **Player:** the same VideoCore/mpvcore player with the live-action profile; progress goes to the local library.

**Jellyfin used as a reference only (not installed):**
- `Emby.Naming` rules for the matcher.
- Next Up and resume logic.
- Its series/season/episode screens, for layout ideas.
- jellyfin-mpv-shim, for how it drives mpv for TV content.

### 4.7 Seanime acceptance checklist

- [ ] Every screen from upstream renders and works in the standalone app, compared side by side with screenshots of the upstream web UI
- [ ] VideoCore player in mpvcore mode: play/pause/seek, tracks, subtitles (typeset ASS), chapters, skip intro, next episode, fullscreen, Anime4K picker
- [ ] Wrong-episode acceptance from §4.4 passes
- [ ] Movies & TV: browse, search, play a movie, play a series with autoplay next, Continue Watching and Next Up correct
- [ ] Targets from §1 measured and written into `apps/MEASUREMENTS.md`
- [ ] Must-keep list re-verified

## 5. Music player (`music-player`)

**Tech:** C11, GTK3 (GtkTreeView in fixed-height mode, which stays fast with 100k rows), libmpv (audio only: `vid=no`, `audio-display=no`), libavformat/libavcodec (tags and cover art, no taglib), sqlite3 with full-text search (FTS5) for instant search. Meson. **0 packages.**

### 5.1 Features (foobar2000-style)

- **Layout:** splitters you can rearrange, saved per user.
  - Library tree (Artist → Album → Track, or by folder).
  - **Playlist tabs**, each a column list. Columns are customizable using foobar-like format strings (`%artist%`, `%album%`, `%tracknumber%`, `%title%`, `$if(...)`, `%length%`, `%codec%`, `%bitrate%`). Click to sort, drag to reorder.
  - Cover art panel and a properties panel (all tags plus technical info).
  - Status bar: format, sample rate, bitrate, position, total time.
- **Playback:**
  - Gapless (`gapless-audio=yes`).
  - **ReplayGain** track/album, with preamp and clipping prevention (`replaygain=album replaygain-clip=no`).
  - Playback orders: default / repeat playlist / repeat track / shuffle tracks / shuffle albums.
  - Queue; stop after current; a 10-band EQ (mpv `af=lavfi=[superequalizer]`).
- **Formats:** everything ffmpeg plays (FLAC, ALAC, Opus, MP3, AAC, WavPack, APE, DSD via ffmpeg), plus `.cue` sheets and `.m3u`/`.m3u8` playlists.
- **Library:** folders added in settings. Rescans happen on demand plus a quick check of file modification times at startup. The file watcher runs **only while the window is open**.
- **Search:** type-to-filter across the library. Results update on every keystroke from the FTS5 index.
- **Bit-perfect option:** asks PipeWire to follow the file's sample rate (`audio-samplerate=0` plus the right PipeWire `allowed-rates`, set per stream).
- **Control from anywhere:** `music-player-ctl play|pause|toggle|next|prev|stop|vol±` over a UNIX socket, bound to keys in the dwm savedconfig. No D-Bus or MPRIS needed.
- **Later:** ListenBrainz/Last.fm scrobbling, `.lrc` lyrics panel, tag editing (rewriting tags through ffmpeg remux).

### 5.2 Laptop optimizations

- **Paused:** 0 wakeups and no timers.
- **Playing:** only mpv's audio thread is active. The position display updates only while the window is visible (checked with X11 visibility events).
- **Large audio buffer** (`audio-buffer=1.0`, plus a larger PipeWire block for this stream through `PIPEWIRE_LATENCY` or the mpv pipewire option). The CPU and PipeWire wake up less often per second.
- **Cover art:** decoded lazily with ffmpeg's own JPEG/PNG decoders (gdk-pixbuf here has no JPEG support) and scaled once to the panel size.
- **No visualizers or animations.**
- **Audio on demand:** runs `audio ensure` before the first play.

### 5.3 Acceptance

- Gapless verified on a gapless album.
- ReplayGain values match the tags.
- 20k-track library: scan time, idle RAM ≤ 30 MB, and playing power measured against idle.

## 6. Minecraft launcher (`mclaunch`)

**Tech:** Go backend + React UI built from Seanime's design (Tailwind tokens and ported UI components), running in **appshell** without libmpv. **When the game starts, the CEF UI shuts down completely.** A ~10 MB Go supervisor waits for the game to exit (to restore the power profile and offer the crash log), then exits too.

### 6.1 Accounts

- **Microsoft sign-in, device-code flow:** shows a code and a URL that you open on any device or in Thorium. No embedded login page.
  - Chain: Microsoft → Xbox Live → XSTS → Minecraft services → profile.
  - Multiple accounts. Tokens refresh on their own. Stored in `~/.local/share/mclaunch/accounts.json` with mode 0600 (no keyring, since libsecret isn't installed).
- **Needs an app registration of our own** (Azure app ID with personal Microsoft accounts and public-client flows enabled).
  - **Mojang must approve it** before it can call Minecraft services. A non-approved app gets a 403 ([MS Q&A thread](https://learn.microsoft.com/en-gb/answers/questions/5768276/how-to-get-xboxlive-signin-permission-for-azure-ap)).
  - **Applied for in phase 0**, because approval can take a while.
  - Prism's app ID won't be reused; Prism asks forks to use their own.
- **Until approval:** an offline-mode profile for singleplayer testing only.

### 6.2 Versions and mod loaders

- **Vanilla:** Mojang version manifest v2 → version JSON → libraries (OS rules), natives (LWJGL 3 native jars), asset index, client jar. SHA1 checked on everything.
- **Fabric and Quilt:** their meta APIs merge a profile into the version JSON.
- **NeoForge, then Forge:** runs the official installer in headless mode with our Java (it runs its own setup steps), then reads the resulting profile.
- **Shared storage** (Prism-style): libraries, assets and Java live in one place, shared by all instances.
- **Downloads:** 16 at a time over HTTP/2, with resume and retry.

### 6.3 Java: 0 packages

- Uses Mojang's Java runtime manifest (the same one the official launcher uses). Each version JSON says which Java it needs (`javaVersion.component`: `java-runtime-delta`, etc.). That runtime downloads to `~/.local/share/mclaunch/java/<component>`, checked against its hashes.
- These Java builds link against glibc, X11, ALSA and GL, all already installed. LWJGL brings its own GLFW and OpenAL natives.
- Optional custom Java path per instance.

### 6.4 Instances

- **Create:** pick a version + loader. Per-instance settings: Java, memory, extra JVM arguments, window size, environment variables, and game-dir subfolders (mods, resource packs, shader packs, saves, screenshots).
- **Mods (Modrinth API v2, no key):** search with filters by version and loader, install with dependencies, update all, enable/disable, find the project for existing jars by file hash.
- **Modpacks:** `.mrpack` import/export. **Import existing Prism instances** (`instance.cfg` + `mmc-pack.json`) so nothing is lost.
- **CurseForge:** later. It needs our own API key.
- **Worlds:** list, open folder, zip backup.
- **Logs:** live log viewer with search and highlighting. On a crash, the crash report is opened with the likely mod named (scans the stack trace for mod package names).
- **Later:** skin management, servers list, CurseForge, Technic/ATLauncher import.

### 6.5 Launching

- Builds the classpath and arguments from the version JSON (game/JVM argument rules, features such as quick play), sets the natives path, and starts with `setsid` so the game is independent of the launcher.
- Runs `audio ensure` first: PipeWire on demand, as above.

### 6.6 Tuned for this laptop

- **Java flags (default, to be measured):**
  - `-Xms=-Xmx`: 4 GB vanilla or light mods, 6–8 GB for big packs. A huge heap doesn't help Minecraft and makes garbage-collection pauses longer.
  - Collector: G1 with `-XX:MaxGCPauseMillis=37 -XX:+AlwaysPreTouch -XX:+UseTransparentHugePages -XX:+ParallelRefProcEnabled`, **vs** generational ZGC (`-XX:+UseZGC`, generational by default on Java 23+). Frame-time spikes measured on both; the smoother one becomes the default.
- **Mesa (radeonsi):** `mesa_glthread=true` (big win for Minecraft), `MESA_SHADER_CACHE_MAX_SIZE=4G`.
- **Frame cap per power source:** before launch the launcher writes `maxFps` into the instance's `options.txt`. On AC it's 120 (matches the panel; VRR 30–120 is on in Xorg). On battery it's 60, configurable. That saves real watts at the cost of a little smoothness.
- **One-click performance mods** for new Fabric/Quilt instances, all from Modrinth:
  - Sodium, Lithium, FerriteCore, ModernFix, ImmediatelyFast, EntityCulling.
  - Optional: C2ME (chunk generation), Iris (shaders; heavy at 1800p on the 890M).
- **Power profile while playing:**
  - The launcher calls `doas /usr/local/sbin/powerprofile game` at launch and `powerprofile auto` after the game exits.
  - **Needs a root change:** add a `game` mode to the script (on AC: EPP `performance`, amd-pmf `performance`, hp-wmi `performance`; on battery: stays `balance_power`), plus a doas rule `permit nopass rec cmd /usr/local/sbin/powerprofile args game` (and `auto`).
  - Handed over as `~/claude/apps/powerprofile-game.sh` for the owner to run with doas.

### 6.7 Acceptance

- Microsoft login works with the approved app ID.
- Vanilla latest, Fabric + Sodium, and one NeoForge pack all launch.
- A Prism instance imports and launches.
- The UI process is gone while playing; the power profile is restored after exit.
- Frame times and watts measured with both collectors.

## 7. Phases (in order)

| Phase | Work | Exit criteria |
|---|---|---|
| **0. Spikes** (risk first) | (1) Prebuilt CEF: check `ldd` (CUPS?), get GPU-buffer off-screen rendering working on X11 + EGL, blend over a test pattern at 2x. (2) libmpv render API in the same EGL context with Anime4K A+A + deband + VA-API; measure watts vs Denshi; find out whether it uses `gpu` or `gpu-next`. (3) Submit the Azure app + Mojang approval request. | Go/no-go on the CEF design (else fallback §3.7.4). Measured A+A watts. Approval request sent. |
| **1. Seanime backend** | Fork, build unchanged in web-server mode, `debug pick` tool, episode matcher, fixes A–D, tests, lazy services | Bug acceptance §4.4 passes (tested in web-server mode) |
| **2. appshell + media-player UI** | Window/compositor, input, bridge (`window.electron` + mpv-prism channels), in-process backend, CEF settings | §4.7 boxes 1–2; first measurements |
| **3. Quality and tuning** | Automatic profiles, battery options, shader cache, streaming cache, instant next episode, power measurements | Watts/RAM/startup written down; owner happy with picture quality |
| **4. Movies & TV** | `internal/stremio`, routes/pages, library, Next Up, RD checks | §4.7 box 4 |
| **5. Music** | Full §5 | §5.3 |
| **6. Launcher** | Full §6 (UI from Seanime's components) | §6.7 |
| **7. Wrap-up** | `update-apps.sh`, dwm keys/rules, debloat-skill reference updated (new apps, numbers), `MEASUREMENTS.md` | Must-keep list verified after a fresh boot |

Each phase gets its own folder of notes under `~/claude/apps/phaseN/`, like the debloat batches.

## 8. Package ledger

**New Portage packages: 0.** (Measured alternatives: WebKitGTK would have been 131 to build, ~25 staying, and it brings back the gnutls chain. Stock Prism would have been 98 to build and ~30 staying.)

Already installed and used: go, nodejs/npm, gcc, meson, ninja, pkgconf, git, gtk+:3, cairo, pango, gdk-pixbuf, at-spi2-core, libX11/libXi/libXcursor/libXrandr, mesa (EGL/GL/VA-API/RADV), libglvnd, libmpv (mpv 0.41, libplacebo 7.360), ffmpeg, libass, fontconfig/freetype/harfbuzz, sqlite, openssl, curl.

**Outside Portage** (in `~/src/third_party` or app data; nothing system-wide):

| Item | Why | Updated by |
|---|---|---|
| CEF minimal SDK (≈250–300 MB) | The page engine | `update-apps.sh` |
| NSS, NSPR, libxkbcommon .so files | What CEF needs at runtime (same as Thorium's bundle) | `bundle-libs.py` |
| Stand-in `libcups.so.2` (only if needed) | CEF links it; printing never used | rebuilt with appshell |
| Anime4K GLSL shaders | Quality profiles | manual |
| Inter font (OFL) | Seanime's UI font. Only DejaVu is installed, and Seanime ships only Roboto. | manual |
| npm and Go modules | Build-time only | in the repos |
| Mojang Java runtimes | Minecraft | the launcher itself |

**System changes:** one root script for the power profile + doas rule (§6.6). Nothing else.

## 9. For later: a custom GPU-drawn Seanime UI (Gio)

Written down at the owner's request as a possible next step once the CEF version is done.

- **What:** rebuild Seanime's screens natively with **Gio** (gioui.org), a pure-Go UI toolkit that draws everything itself on the GPU (OpenGL/Vulkan).
- **Look:** because everything is custom-drawn, it can get very close to the React UI: rounded cards, gradients, banner fades, blur, smooth 120 Hz animations and scrolling.
- **Gains vs CEF** (estimates, to be measured):
  - ~50–80 MB instead of ~200–300 MB while browsing.
  - 1 process instead of 3–4.
  - No Chromium to rebuild for security updates.
  - Video and UI share one GPU context with no buffer import step.
- **Cost:**
  - Every screen and component is rebuilt by hand.
  - Upstream UI changes no longer arrive for free when rebasing; only backend changes do.
  - Gio's X11 backend links libxkbcommon: **+1 package**, or 0 if bundled like the CEF libraries.
- **Approach when the time comes:**
  - Keep the Go backend from this fork and call it in-process (no HTTP).
  - Port Seanime's Tailwind design tokens (colors, spacing, radii, fonts) into a Gio theme.
  - Rebuild screens in order of use: Home → Anime entry + episode list → Player controls → Movies & TV → Search → Settings.
  - Use the CEF build as the screenshot reference for each screen.
- **The same toolkit could later replace the launcher's CEF UI** and give the music player a unified look.

## 10. Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| GPU-buffer off-screen rendering doesn't work on X11 with the chosen CEF | Medium | Spike in phase 0. Software paint fallback (CPU copy, fine for static UI). Last resort §3.7.4. |
| libmpv render API is the older `gpu` renderer, not `gpu-next` | Medium | Anime4K and deband unaffected. Dolby Vision gets the separate-window fallback. |
| Prebuilt CEF needs CUPS or other missing libraries | High (CUPS) | Stand-in library, or build CEF with `use_cups=false` using the Thorium setup |
| Upstream Seanime changes Denshi/mpv-prism APIs | Medium | Bridge covers mpv-prism 0.1.9 channels. Rebase notes per release; bridge tests check every channel. |
| Mojang approval for the app ID is slow or refused | Medium | Applied for in phase 0. Offline-mode testing meanwhile. |
| RD changes its API again | Low–medium | All RD calls in one client with response tests; the cached-probe approach doesn't depend on the dead endpoint. |
| A+A too heavy on battery | Low | Battery options in §4.5, measured per option |

## 11. Answered (2026-10-11)

1. Seanime features: **keep everything** (D11).
2. Add-ons: **Torrentio**, with **Comet** as backup (D12). Trakt: not asked for, so not planned.
3. Battery: **keep A+A** (D13).
4. Old Denshi mpvcore settings: **gone**. The §4.5 starting values are used and tuned by eye in phase 3.
5. Names: **media-player**, **music-player**, **mclaunch** (D10).

## 12. GitHub

- One repo per project: `media-player`, `music-player`, `mclaunch`. appshell lives inside media-player and is used by mclaunch as a Go module or git submodule (decided in phase 2).
- **All three repos are private** (owner's choice, 2026-10-11). Any of them can be made public later.
- `media-player` is a private copy, not a GitHub fork (GitHub doesn't allow private forks of public repos). It keeps Seanime's full git history, with `upstream` pointing at 5rahim/seanime. The README credits Seanime and keeps its GPL-3.0 license.
- **README layout:** copied from the owner's existing GitHub repos, and the same for all three.
- Git uses HTTPS only (no ssh, per the must-keep list). Logged in through the `gh` CLI, built from source with Go into `~/.local/bin`, not Portage.
