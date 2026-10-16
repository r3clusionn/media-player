// Plays a local file full-window through window.mpvPrism, fullscreen, with a
// white label over it (for the HDR check: the page layer drawn as PQ at the
// SDR white). usage: node hdr-play.mjs <file> [seconds]
import { connect, sleep } from './cdp.mjs'
const [file, secs] = process.argv.slice(2)
const p = await connect(Number(process.env.CDP_PORT || 9333))
await p.eval(`(() => {
  const host = document.createElement("div")
  host.id = "hdr-test"
  host.setAttribute("data-mpv-prism-video-host", "")
  host.style.cssText = "position:fixed;inset:0;z-index:99999;background:black"
  const v = document.createElement("video")
  v.setAttribute("data-mpv-prism-video", "")
  v.style.cssText = "width:100%;height:100%;display:none"
  const label = document.createElement("div")
  label.textContent = "media-player HDR test: this label should look like normal white text"
  label.style.cssText = "position:absolute;left:40px;bottom:40px;padding:12px 20px;font:28px sans-serif;color:white;background:rgba(0,0,0,0.5);border-radius:8px"
  host.append(v, label)
  document.body.append(host)
})()`)
await p.eval(`window.mpvPrism.create("hdrtest", {}, null)`)
await p.eval(`window.mpvPrism.startVideoPresenter("hdrtest", "hdrtest-p", {width: 1920, height: 1080})`)
await p.eval(`window.mpvPrism.load("hdrtest", ${JSON.stringify(file)})`)
await p.eval(`window.mpvPrism.setProperty("hdrtest", "loop-file", "inf")`)
await p.eval(`window.mpvPrism.play("hdrtest")`)
await p.eval('window.electron.window.setFullscreen(true)')
await sleep(4000)
for (const n of ["video-params/gamma", "target-trc", "target-prim", "target-peak", "hdr-reference-white", "target-colorspace-hint"])
  console.log(n, "=", await p.eval(`window.mpvPrism.getProperty("hdrtest", ${JSON.stringify(n)})`))
if (secs) {
  await sleep(Number(secs) * 1000)
  await p.eval('window.electron.window.setFullscreen(false)')
  await sleep(3000)
  console.log("after fullscreen off: target-trc =", await p.eval(`window.mpvPrism.getProperty("hdrtest", "target-trc")`))
}
p.close()
