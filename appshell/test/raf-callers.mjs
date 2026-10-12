// Counts requestAnimationFrame callers (by stack) for one second.
import { connect, sleep } from "./cdp.mjs"
const page = await connect(9333)
console.log(await page.eval(`(async () => {
  const counts = new Map(), orig = window.requestAnimationFrame
  window.requestAnimationFrame = function (cb) {
    const st = (new Error().stack || '').split('\\n').slice(2, 5).map(s => s.trim().replace(/https?:\\/\\/[^/]+\\/static\\/js\\//, '')).join(' < ')
    counts.set(st, (counts.get(st) || 0) + 1)
    return orig.call(window, cb)
  }
  await new Promise(r => setTimeout(r, 1000))
  window.requestAnimationFrame = orig
  return JSON.stringify([...counts.entries()].sort((a, b) => b[1] - a[1]).slice(0, 6), null, 1)
})()`))
page.close()
