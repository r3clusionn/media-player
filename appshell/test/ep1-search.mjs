// Opens Frieren's debrid streaming tab and clicks episode 1, then lists what
// the torrent search drawer shows.
import { connect, sleep } from "./cdp.mjs"
const page = await connect(9333)
const clicked = await page.eval(`(() => {
  const img = [...document.querySelectorAll('img')].find(i => /Episode 1 \\/ /.test(i.parentElement?.parentElement?.textContent || '') && i.getBoundingClientRect().width > 300)
  if (!img) return null
  img.scrollIntoView({ block: 'center' })
  const r = img.getBoundingClientRect(); return { x: r.left + r.width / 2, y: r.top + r.height / 2 }
})()`)
console.log("card:", JSON.stringify(clicked))
if (clicked) for (const type of ["mouseMoved", "mousePressed", "mouseReleased"])
  await page.send("Input.dispatchMouseEvent", { type, x: clicked.x, y: clicked.y, button: "left", clickCount: 1 })
await sleep(9000)
console.log(await page.eval(`JSON.stringify({
  dialog: !!document.querySelector('[role=dialog]'),
  texts: [...document.querySelectorAll('[role=dialog] *')].filter(e => e.children.length === 0).map(e => e.textContent.trim()).filter(t => t && t.length > 3 && t.length < 120).slice(0, 40) })`))
page.close()
