// Clicks episode argv[2] on the open entry page; with argv[3] = "auto", first
// turns on the Auto-select switch.
import { connect, sleep } from "./cdp.mjs"
const page = await connect(9333)
const ep = Number(process.argv[2] || 1)
if (process.argv[3] === "auto") {
  const on = await page.eval(`(() => {
    const sw = [...document.querySelectorAll('[role=switch]')].find(s => document.querySelector('label[for="' + s.id + '"]')?.textContent.trim() === 'Auto-select')
    if (!sw) return 'no switch'
    if (sw.getAttribute('aria-checked') !== 'true') sw.click()
    return 'on'
  })()`)
  console.log("auto-select:", on)
  await sleep(800)
}
const p = await page.eval(`(() => {
  const img = [...document.querySelectorAll('img')].find(i => (i.parentElement?.parentElement?.textContent || '').includes('Episode ${ep} / ') && i.getBoundingClientRect().width > 200)
  if (!img) return null
  img.scrollIntoView({ block: 'center' }); const r = img.getBoundingClientRect(); return { x: r.left + r.width / 2, y: r.top + r.height / 2 }
})()`)
console.log("episode card:", JSON.stringify(p))
if (p) for (const type of ["mouseMoved", "mousePressed", "mouseReleased"]) await page.send("Input.dispatchMouseEvent", { type, x: p.x, y: p.y, button: "left", clickCount: 1 })
page.close()
