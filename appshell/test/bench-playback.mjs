// Playback benchmark for the open player (start it with run-app.sh and a
// stream, e.g. stream-ep1.sh): picks a shader preset, seeks, then for
// SECONDS keeps the control bar busy with mouse moves (as a user would) and
// measures
//   - mpv's drop counters (decoder, output, late and mistimed frames),
//   - the page's frame intervals (requestAnimationFrame, CEF at 120 Hz),
//   - input round trips through the browser's UI thread (this process's
//     main loop), one mouse move every 50 ms,
//   - GPU busy % from amdgpu,
//   - on the native side (appshell:stats): swaps per second, the longest
//     main-loop stall, how long a page frame waits to reach the screen, and
//     video frames shown and skipped.
//
//   node bench-playback.mjs [preset] [seconds] [seek]
//   preset: none, a, aa, b, bb, c, ca, a-hq, c-hq, aa-hq (Seanime's Anime4K
//   Fast/HQ chains), upm (the 2x M upscaler alone), any of those with "-cfl"
//   (chroma-from-luma first), artcnn, artcnn-ds, artcnn-dn, cfl (the ArtCNN
//   presets)
//   SYNC=audio|display-resample sets mpv's video-sync for the run. STATS=file
//   (mpv's dump-stats, set in the player's mpv.conf) adds the cadence: how many
//   panel refreshes each new video frame stayed on screen.
import { readFileSync, readdirSync, statSync, openSync, readSync, closeSync, existsSync } from "node:fs"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"
import { connect, sleep } from "./cdp.mjs"

const preset = process.argv[2] || "none"
const seconds = Number(process.argv[3] || 30)
const seekTo = Number(process.argv[4] || 300)
const dir = join(dirname(fileURLToPath(import.meta.url)), "../../build/dist/anime4k")
const D = ["AutoDownscalePre_x2", "AutoDownscalePre_x4"]
const chains = {
    none: [],
    a: ["Clamp_Highlights", "Restore_CNN_M", "Upscale_CNN_x2_M", ...D, "Upscale_CNN_x2_S"],
    aa: ["Clamp_Highlights", "Restore_CNN_M", "Upscale_CNN_x2_M", "Restore_CNN_S", ...D, "Upscale_CNN_x2_S"],
    b: ["Clamp_Highlights", "Restore_CNN_Soft_M", "Upscale_CNN_x2_M", ...D, "Upscale_CNN_x2_S"],
    bb: ["Clamp_Highlights", "Restore_CNN_Soft_M", "Upscale_CNN_x2_M", "Restore_CNN_Soft_S", ...D, "Upscale_CNN_x2_S"],
    c: ["Clamp_Highlights", "Upscale_Denoise_CNN_x2_M", ...D, "Upscale_CNN_x2_S"],
    ca: ["Clamp_Highlights", "Upscale_Denoise_CNN_x2_M", "Restore_CNN_M", ...D, "Upscale_CNN_x2_S"],
    upm: ["Upscale_CNN_x2_M"],
    "a-hq": ["Clamp_Highlights", "Restore_CNN_VL", "Upscale_CNN_x2_VL", ...D, "Upscale_CNN_x2_M"],
    "c-hq": ["Clamp_Highlights", "Upscale_Denoise_CNN_x2_VL", ...D, "Upscale_CNN_x2_M"],
    "aa-hq": ["Clamp_Highlights", "Restore_CNN_VL", "Upscale_CNN_x2_VL", "Restore_CNN_M", ...D, "Upscale_CNN_x2_M"],
}
// ArtCNN presets (file names without the Anime4K_ prefix).
const files = {
    artcnn: ["CfL_Prediction", "ArtCNN_C4F16"],
    "artcnn-ds": ["CfL_Prediction", "ArtCNN_C4F16_DS"],
    "artcnn-dn": ["CfL_Prediction", "ArtCNN_C4F16_DN"],
    cfl: ["CfL_Prediction"],
}
const base = preset.replace(/-cfl$/, "")
if (!chains[base] && !files[base]) throw new Error(`unknown preset ${preset}`)
const shaders = [
    ...(base !== preset ? [join(dir, "CfL_Prediction.glsl")] : []),
    ...(chains[base] ? chains[base].map(n => join(dir, `Anime4K_${n}.glsl`)) : files[base].map(n => join(dir, `${n}.glsl`))),
]

const page = await connect(Number(process.env.CDP_PORT || 9333))
const pid = await page.eval(`(async () => {
  let pid = null
  const off = window.mpvPrism.onEvent(e => { pid = e.playerId })
  await new Promise(r => setTimeout(r, 1500)); off()
  return pid
})()`)
if (!pid) throw new Error("no player is open")
const prop = (n) => page.eval(`window.mpvPrism.getProperty(${JSON.stringify(pid)}, ${JSON.stringify(n)})`)
const counters = async () => {
    const o = {}
    for (const n of ["frame-drop-count", "decoder-frame-drop-count", "vo-delayed-frame-count", "mistimed-frame-count"])
        o[n] = Number(await prop(n)) || 0
    return o
}

await page.eval(`window.mpvPrism.setShaders(${JSON.stringify(pid)}, ${JSON.stringify(shaders)})`)
if (process.env.SYNC)
    await page.eval(`window.mpvPrism.setProperty(${JSON.stringify(pid)}, "video-sync", ${JSON.stringify(process.env.SYNC)})`)
await page.eval(`window.mpvPrism.seek(${JSON.stringify(pid)}, ${seekTo}, "absolute")`)
await page.eval(`window.mpvPrism.play(${JSON.stringify(pid)})`)
await sleep(6000) // shader compilation, cache refill

// Page side: rAF intervals for the run.
await page.eval(`(() => {
  window.__bench = { d: [], last: 0, on: true }
  const tick = (t) => { const b = window.__bench; if (b.last) b.d.push(t - b.last); b.last = t; if (b.on) requestAnimationFrame(tick) }
  requestAnimationFrame(tick)
})()`)

const stats = () => page.eval(`window.__appshell.call("appshell:stats").catch(() => null)`)
await stats()
const c0 = await counters()
const statsFile = process.env.STATS
const statsFrom = statsFile && existsSync(statsFile) ? statSync(statsFile).size : 0
const t0 = Date.now()
const rtt = []
const busy = []
let i = 0
const card = ["card0", "card1"].find(c => existsSync(`/sys/class/drm/${c}/device/gpu_busy_percent`))
const busyFile = `/sys/class/drm/${card}/device/gpu_busy_percent`
// Package power (amdgpu's PPT sensor on an APU: the whole chip, in microwatts).
const hwmon = `/sys/class/drm/${card}/device/hwmon`
const powerFile = existsSync(hwmon) ? readdirSync(hwmon).map(h => `${hwmon}/${h}/power1_average`).find(existsSync) : null
const power = []
while (Date.now() - t0 < seconds * 1000) {
    const x = 1200 + (i % 2) * 200, y = 700 + (i % 3) * 60
    const s = performance.now()
    await page.send("Input.dispatchMouseEvent", { type: "mouseMoved", x, y })
    rtt.push(performance.now() - s)
    if (i % 4 === 0) {
        try { busy.push(Number(readFileSync(busyFile, "utf8"))) } catch {}
        try { if (powerFile) power.push(Number(readFileSync(powerFile, "utf8")) / 1e6) } catch {}
    }
    i++
    await sleep(50)
}
const statsTo = statsFile && existsSync(statsFile) ? statSync(statsFile).size : 0
const sync = await prop("video-sync")
const screen = await stats()
const c1 = await counters()
const frames = await page.eval(`(() => { window.__bench.on = false; return window.__bench.d })()`)
const fps = await prop("estimated-vf-fps")
const passes = await prop("vo-passes").catch(() => null)
page.close()

const pct = (a, p) => { const s = [...a].sort((x, y) => x - y); return s[Math.min(s.length - 1, Math.floor(p * s.length))] }
const mean = (a) => a.reduce((x, y) => x + y, 0) / (a.length || 1)
const gpuMs = passes?.fresh ? passes.fresh.reduce((s, p) => s + p.avg, 0) / 1e6 : null
const delta = Object.fromEntries(Object.keys(c0).map(k => [k, c1[k] - c0[k]]))
const out = {
    preset, seconds, sync,
    video: { fps: Number(fps)?.toFixed?.(3), gpuMsPerFrame: gpuMs && +gpuMs.toFixed(2), ...delta },
    page: {
        frames: frames.length, fps: +(1000 / mean(frames)).toFixed(1),
        p50: +pct(frames, 0.5).toFixed(1), p95: +pct(frames, 0.95).toFixed(1), p99: +pct(frames, 0.99).toFixed(1),
        max: +Math.max(...frames).toFixed(1), over25ms: frames.filter(d => d > 25).length,
    },
    input: { n: rtt.length, p50: +pct(rtt, 0.5).toFixed(1), p95: +pct(rtt, 0.95).toFixed(1), max: +Math.max(...rtt).toFixed(1) },
    screen: screen && {
        swapsPerS: +(screen.swaps / screen.seconds).toFixed(1), loopMaxMs: +screen.loopMaxMs.toFixed(1),
        loopOver20ms: screen.loopOver20ms, pageToScreen: screen.pageToScreenMs,
        videoShown: screen.videoShown, videoSkipped: screen.videoSkipped,
    },
    gpuBusy: busy.length ? +mean(busy).toFixed(0) : null,
    packageW: power.length ? +mean(power).toFixed(1) : null,
    cadence: statsTo > statsFrom ? cadence(statsFile, statsFrom, statsTo) : null,
}
console.log(JSON.stringify(out))

// Refreshes each new video frame stayed on screen, from mpv's dump-stats:
// with display sync, a draw that logged a frame-duration drew a new frame;
// with audio sync mpv only flips for new frames. A flip ends when mpv hands
// the frame to the swapchain. 23.976 fps at 120 Hz is 5 throughout.
function cadence(file, from, to) {
    const everyFlip = sync === "audio"
    const buf = Buffer.alloc(to - from)
    const fd = openSync(file, "r")
    readSync(fd, buf, 0, buf.length, from)
    closeSync(fd)
    const fresh = []
    let pending = false
    for (const line of buf.toString().split("\n")) {
        const sp = line.indexOf(" ")
        const t = Number(line.slice(0, sp)), what = line.slice(sp + 1).trim()
        if (what === "start video-draw") pending = false
        else if (what.endsWith(" frame-duration")) pending = true
        else if (what === "end video-flip" && (pending || everyFlip)) { fresh.push(t); pending = false }
    }
    const refresh = 1e9 / 120
    const hist = {}
    for (let k = 1; k < fresh.length; k++) {
        const n = Math.round((fresh[k] - fresh[k - 1]) / refresh)
        hist[n] = (hist[n] || 0) + 1
    }
    return hist
}
