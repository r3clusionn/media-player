// Opens a Movies & TV title, an episode's streams and plays the best one.
// usage: node mt-play.mjs <type> <imdb id> [video id]
import { connect, sleep } from './cdp.mjs'
const [type, id, video] = process.argv.slice(2)
const p = await connect(9333)
await p.eval('window.electron.window.setFullscreen(true)')
await p.navigate(`http://127.0.0.1:43211/movies-tv/title?type=${type}&id=${id}` + (video ? `&video=${encodeURIComponent(video)}` : ''))
await sleep(4000)
if (!video && type === 'movie') await p.eval(`[...document.querySelectorAll("button")].find(b=>b.textContent.trim()==="Watch").click()`)
for (let i = 0; i < 40; i++) {
  await sleep(1000)
  if (await p.eval(`!![...document.querySelectorAll("button")].find(b=>b.textContent.startsWith("Play best"))`)) break
}
console.log(await p.eval(`[...document.querySelectorAll("button")].find(b=>b.textContent.startsWith("Play best"))?.textContent`))
await p.eval(`[...document.querySelectorAll("button")].find(b=>b.textContent.startsWith("Play best")).click()`)
p.close()
