// Reports the mpv-prism presenter element, the active player and its state.
import { connect, sleep } from "./cdp.mjs"
const page = await connect(9333)
console.log(await page.eval(`(async () => {
  let pid = null
  const off = window.mpvPrism.onEvent(e => { pid = e.playerId })
  await new Promise(r => setTimeout(r, 1500)); off()
  const v = document.querySelector('[data-mpv-prism-video]')
  const r = v && v.getBoundingClientRect()
  const host = v && v.closest('[data-mpv-prism-video-host]')
  const props = {}
  if (pid) for (const n of ['time-pos','pause','duration','hwdec-current','width','height','glsl-shaders','vid','aid','sid']) props[n] = await window.mpvPrism.getProperty(pid, n).catch(e => 'ERR ' + e.message)
  return JSON.stringify({ pid, video: !!v, rect: r && [r.left, r.top, r.width, r.height].map(Math.round),
    hostBg: host && getComputedStyle(host).backgroundColor, bodyBg: getComputedStyle(document.body).backgroundColor,
    holes: document.querySelectorAll('[style*="mask"]').length, props })
})()`))
page.close()
