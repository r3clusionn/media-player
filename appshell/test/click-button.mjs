// Clicks the visible button whose text is argv[2], waits argv[3] ms, lists dialog texts.
import { connect, sleep } from "./cdp.mjs"
const page = await connect(Number(process.env.CDP_PORT || 9333))
const label = process.argv[2]
const pos = await page.eval(`(() => {
  const b = [...document.querySelectorAll('button')].filter(b => b.textContent.trim() === ${JSON.stringify(label)} && b.getBoundingClientRect().width > 0).pop()
  if (!b) return null
  const r = b.getBoundingClientRect(); return { x: r.left + r.width / 2, y: r.top + r.height / 2 }
})()`)
console.log("button:", JSON.stringify(pos))
if (pos) for (const type of ["mouseMoved", "mousePressed", "mouseReleased"])
  await page.send("Input.dispatchMouseEvent", { type, x: pos.x, y: pos.y, button: "left", clickCount: 1 })
await sleep(Number(process.argv[3] || 8000))
console.log(await page.eval(`JSON.stringify({
  texts: [...document.querySelectorAll('[role=dialog] *')].filter(e => e.children.length === 0).map(e => e.textContent.trim()).filter(t => t && t.length > 2 && t.length < 140).slice(-60),
  video: !!document.querySelector('[data-mpv-prism-video]') })`))
page.close()
