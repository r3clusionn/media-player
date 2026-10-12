// In the open torrent drawer, clicks the result whose title contains argv[2],
// then waits and lists what appears (file picker or player).
import { connect, sleep } from "./cdp.mjs"
const page = await connect(9333)
const want = process.argv[2]
const pos = await page.eval(`(() => {
  const el = [...document.querySelectorAll('[role=dialog] *')].filter(e => e.children.length === 0 && e.textContent.includes(${JSON.stringify(want)}))[0]
  if (!el) return null
  el.scrollIntoView({ block: 'center' }); const r = el.getBoundingClientRect(); return { x: r.left + 20, y: r.top + r.height / 2 }
})()`)
console.log("result:", JSON.stringify(pos))
if (pos) for (const type of ["mouseMoved", "mousePressed", "mouseReleased"])
  await page.send("Input.dispatchMouseEvent", { type, x: pos.x, y: pos.y, button: "left", clickCount: 1 })
await sleep(Number(process.argv[3] || 12000))
console.log(await page.eval(`JSON.stringify({
  dialogs: [...document.querySelectorAll('[role=dialog]')].length,
  texts: [...document.querySelectorAll('[role=dialog] *')].filter(e => e.children.length === 0).map(e => e.textContent.trim()).filter(t => t && t.length > 2 && t.length < 140).slice(0, 50),
  video: !!document.querySelector('[data-mpv-prism-video]') })`))
page.close()
