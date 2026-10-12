// Prints page console messages and exceptions for argv[2] ms (default 20000),
// optionally after clicking episode 1 (argv[3] = "ep1").
import { connect, sleep } from "./cdp.mjs"
const page = await connect(9333)
page.on((m) => {
  if (m.method === "Runtime.consoleAPICalled") {
    const t = m.params.args.map(a => a.value ?? a.description ?? a.type).join(" ")
    if (!/Download the React DevTools|\[HMR\]/.test(t)) console.log(`[${m.params.type}]`, t.slice(0, 300))
  } else if (m.method === "Runtime.exceptionThrown") {
    console.log("[exception]", (m.params.exceptionDetails.exception?.description || m.params.exceptionDetails.text).slice(0, 400))
  }
})
if (process.argv[3] === "ep1") {
  const p = await page.eval(`(() => { const img = [...document.querySelectorAll('img')].find(i => (i.parentElement?.parentElement?.textContent || '').includes('Episode 1 / ') && i.getBoundingClientRect().width > 300); if (!img) return null; img.scrollIntoView({block:'center'}); const r = img.getBoundingClientRect(); return {x: r.left + r.width/2, y: r.top + r.height/2} })()`)
  if (p) for (const type of ["mouseMoved", "mousePressed", "mouseReleased"]) await page.send("Input.dispatchMouseEvent", { type, x: p.x, y: p.y, button: "left", clickCount: 1 })
}
await sleep(Number(process.argv[2] || 20000))
page.close()
