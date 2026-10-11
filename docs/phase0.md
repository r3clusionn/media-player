# Phase 0: risk checks

Results of the checks the plan puts first (§7). Machine: Ryzen AI 9 HX 475, Radeon 890M (Mesa 26.1.8, radeonsi), Gentoo, Xorg + dwm. All numbers below are from runs on it.

## 1. CEF off-screen rendering into GPU buffers on X11: works

**Setup.** CEF 154.0.34 (Chromium 154.0.8037.98), the official "minimal" Linux build, checked against its published SHA1. The test program is [`appshell/spike/osr_spike.c`](../appshell/spike/osr_spike.c): an X11 window with an EGL/GLES 3.2 context, a CEF browser with `windowless_rendering_enabled` and `shared_texture_enabled`, and `OnAcceleratedPaint` buffers imported with `EGL_EXT_image_dma_buf_import` and blended over a colored "underlay" standing in for the video.

**Libraries, 0 new packages.** `libcef.so` needs six libraries the system doesn't have:
- NSS, NSPR and libxkbcommon: copies of the ones Thorium already bundles (nspr 4.39, nss 3.125, libxkbcommon 1.13.2).
- `libcups.so.2`: replaced by [`appshell/cups-stub`](../appshell/cups-stub/cups_stub.c), 68 do-nothing functions. CEF never prints. `check-symbols.sh` compares the stub against what `libcef.so` imports, and it caught two functions missed on the first try.

`libcef.so` has `RUNPATH $ORIGIN`, so CEF, the bundled libraries and the stub sit in one runtime folder (hard links, no extra disk).

**Results:**

| | |
|---|---|
| CEF initialized | 117–161 ms after start |
| First frame of a local page | 204–219 ms after start |
| First frame of Seanime's Discover page (served locally) | 307 ms after start |
| Buffer CEF hands over | 1 plane, BGRA, implicit modifier (`DRM_FORMAT_MOD_INVALID`), stride 6400 for a 1582 px wide window |
| Per frame, vsync off, Discover with its carousel running (360 frames) | import 0.04 ms, draw + `glFinish` 0.46 ms, swap 0.04 ms: **0.73 ms in all** |
| Memory with Discover loaded (PSS) | main process 130 MB, renderer 208 MB, GPU 77 MB, network 41 MB, storage 28 MB: **485 MB** |

![The spike's own window showing Seanime's Discover page in its narrow layout (the test window was half the screen wide): the Anime/Schedule switch, Trending Right Now with genre chips, and cover cards for Dragon Ball Super: Beerus and Black Clover 2nd Season](images/phase0/osr-discover.png)

![A transparent test page over the underlay: a half-transparent black header reading "hello from CEF over the underlay", with the colored gradient visible through it and around it](images/phase0/osr-transparency.png)

**Found along the way:**
- CEF treats positional command-line arguments as start URLs. The real program takes no positional arguments in the browser process.
- Chromium reuses the memory that held the environment strings for the process title, so a pointer from `getenv` taken before `cef_initialize` reads as an empty string afterwards. The first runs loaded nothing because the URL was empty. Anything read from the environment is copied before CEF starts.
- With the display powered off (DPMS off, which it was during these runs), Xorg paces a window's buffer swaps with a 1 Hz fallback timer, so a swap with vsync blocks for about a second. The real program must not draw or swap while the window or display isn't visible. It tells CEF the view is hidden instead, which also lets Chromium stop rendering. Frame pacing with the screen on is still to be measured.

**Still open from this check:**
- **Memory** is above the plan's ~300 MB target. Candidates: running the network service in-process, merging the storage service, a smaller image cache, and V8 heap limits. Each is to be measured in phase 3.
- **Pacing at 120 Hz with the screen on**, and VRR.

## 2. libmpv render API in the same context, and Anime4K: works; A+A must be the Fast variant

**Setup.** [`appshell/spike/mpv_spike.c`](../appshell/spike/mpv_spike.c): an X11 window, fullscreen at the panel's 2880x1800, with an EGL/GLES 3.2 context. libmpv's render API (OpenGL) draws into it with `hwdec=vaapi`, deband at the plan's values, and Anime4K shaders. Per-pass GPU times come from mpv's `vo-passes`. The test clip is 60 s of 1920x1080 HEVC Main 10 at 23.976 fps, made with `hevc_vaapi` (no video files were on the machine; shader cost doesn't depend on content). The laptop was on AC, but in its battery platform profile (see below).

**Results through the render API:**

| | Plain (deband, lanczos) | Anime4K A+A (HQ) |
|---|---|---|
| Decoding | VA-API, handed to GL with no copy ("EGL dmabuf interop") | same |
| First frame after `loadfile` | 49 ms | 110 ms |
| GPU time per frame (sum of passes) | 3.48 ms | 51.98 ms |
| Frames dropped in 20 s | 0 | 97 |
| GPU busy | 10.7% | 99.2% |
| CPU (whole process) | 7.1% of one core | 8.5% |

- The render API uses mpv's older `gpu` renderer (FBO format `rgba16f`, and "not a libplacebo vulkan gpu api context"), not `gpu-next`.
- That doesn't matter for Anime4K. The same A+A (HQ) chain rendered as fast as possible with the mpv program costs about the same on every renderer: `gpu`/OpenGL 57.1, `gpu-next`/OpenGL 55.7, `gpu`/Vulkan 56.2, `gpu-next`/Vulkan 53.8 ms per frame (240 frames, startup included).

**Anime4K presets at 2880x1800 from a 1080p source** (mpv `gpu-next`/Vulkan, 480 frames as fast as possible, deband on; 41.7 ms is one frame at 23.976 fps):

| Preset | ms per frame | |
|---|---|---|
| A+A (HQ) | 51.3 | too slow: drops frames |
| A (HQ) | 35.9 | fits, but uses 86% of the frame time |
| **A+A (Fast)** | **21.5** | fits with room to spare |
| A (Fast) | 13.9 | fits |
| deband only | 3.1 | |

**What the owner's "A+A" was.** Seanime's player (`mpv-core.tsx`) has an Anime4K quality setting that defaults to `fast`, and in that mode its "Mode A+A" loads Clamp_Highlights, Restore_CNN_M, Upscale_CNN_x2_M, Restore_CNN_S, AutoDownscalePre_x2/x4 and Upscale_CNN_x2_S: **A+A (Fast)**. That is the default here, and it fits. The HQ presets stay selectable; A (HQ) is labelled as tight, A+A (HQ) as too slow for this panel.

**GPU clock under load:** 2,937 MHz (the top level is 3,100 MHz), the APU's reported power 54 W, 59 °C. A+A (HQ) is limited by the GPU itself, not by power limits or the embedding.

**Not measured yet:**
- **Watts on battery.** The laptop was on AC with a full battery, which reports 0 W.
- **Pacing with the screen on.** The display was off (DPMS); these runs swapped without vsync.

**Found on the system, not changed:** while plugged in, the platform profiles read `low-power` (amd-pmf) and `quiet` (hp-wmi), the battery settings. `/usr/local/sbin/powerprofile` didn't switch to AC. The numbers above were taken in that state.

## 3. Microsoft app registration for mclaunch: needs the owner

Registering the Azure app and asking Mojang for Minecraft API access needs the owner's Microsoft account.
