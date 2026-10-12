// CPU-profiles the page for argv[2] ms and prints the hottest functions.
import { connect, sleep } from "./cdp.mjs"
const page = await connect(9333)
await page.send("Profiler.enable")
await page.send("Profiler.start")
await sleep(Number(process.argv[2] || 3000))
const { profile } = await page.send("Profiler.stop")
const self = new Map()
const dt = profile.timeDeltas, samples = profile.samples
const byId = new Map(profile.nodes.map(n => [n.id, n]))
for (let i = 0; i < samples.length; i++) {
  const n = byId.get(samples[i]); const f = n.callFrame
  const key = `${f.functionName || "(anon)"} ${f.url.split("/").pop()}:${f.lineNumber}`
  self.set(key, (self.get(key) || 0) + (dt[i] || 0))
}
const total = [...self.values()].reduce((a, b) => a + b, 0)
for (const [k, v] of [...self.entries()].sort((a, b) => b[1] - a[1]).slice(0, 15))
  console.log((100 * v / total).toFixed(1).padStart(5) + "%", k.slice(0, 140))
page.close()
