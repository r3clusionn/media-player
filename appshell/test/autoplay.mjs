// Turns on autoplay, seeks the playing episode to its last seconds and reports
// what plays next (media title and path from mpv).
import { connect, sleep } from "./cdp.mjs"
const page = await connect(9333)
const r = await page.eval(`fetch('/api/v1/settings/path', { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ path: 'library.autoPlayNextEpisode', value: true }) }).then(r => r.status)`)
console.log("autoplay setting:", r)
const pid = "seanime-mpv-core-active-0"
const before = await page.eval(`window.mpvPrism.getProperty('${pid}', 'media-title')`)
const dur = await page.eval(`window.mpvPrism.getProperty('${pid}', 'duration')`)
console.log("now playing:", before, "duration", dur)
await page.eval(`window.mpvPrism.seek('${pid}', ${dur - 15}, 'absolute')`)
for (let i = 0; i < 30; i++) {
  await sleep(2000)
  const t = await page.eval(`window.mpvPrism.getProperty('${pid}', 'media-title').catch(() => null)`).catch(() => null)
  if (t && t !== before) { console.log("next:", t, "at", (i + 1) * 2, "s"); break }
}
console.log("path:", await page.eval(`window.mpvPrism.getProperty('${pid}', 'filename').catch(e => e.message)`).catch(e => e.message))
page.close()
