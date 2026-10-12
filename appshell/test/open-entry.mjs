// Fullscreen, then open an anime entry page: node open-entry.mjs <anilistId>
import { connect, sleep } from "./cdp.mjs"
const page = await connect(9333)
await page.eval("window.electron.window.setFullscreen(true)")
await page.navigate(`http://127.0.0.1:43211/entry?id=${process.argv[2] || 154587}`)
await sleep(5000)
page.close()
