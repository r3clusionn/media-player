// The page alone (no video) as Chromium renders it: cdp-shot.mjs out.png
import { connect } from "./cdp.mjs"
import { writeFileSync } from "fs"
const page = await connect(9333)
const r = await page.send("Page.captureScreenshot", { format: "png" })
writeFileSync(process.argv[2], Buffer.from(r.data, "base64"))
page.close()
