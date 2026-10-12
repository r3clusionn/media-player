// Pauses/unpauses and toggles fullscreen repeatedly; after each step reports
// opaque elements over the video (anything but the video should be clear).
import { connect, sleep } from './cdp.mjs'
const p = await connect(9333)
const id = 'seanime-mpv-core-active-0'
const covered = () => p.eval(`(()=>{const bad=[];for(const [x,y] of [[innerWidth*0.25,innerHeight/2],[innerWidth*0.75,innerHeight/2]]){for(const e of document.elementsFromPoint(x,y)){const c=getComputedStyle(e);if(c.backgroundColor!=="rgba(0, 0, 0, 0)"&&!e.closest("[data-mpv-prism-video-host] button"))bad.push(e.tagName+"."+String(e.className).slice(0,30)+" "+c.backgroundColor)}}return bad.length?bad.join(" | "):"clear"})()`)
const step = async (name, fn, wait) => { await fn(); await sleep(wait); console.log(name.padEnd(16), await covered()) }
for (let i = 1; i <= 3; i++) {
  await step(`pause ${i}`, () => p.eval(`window.mpvPrism.setProperty("${id}","pause",true)`), 2500)
  await step(`unpause ${i}`, () => p.eval(`window.mpvPrism.setProperty("${id}","pause",false)`), 2500)
}
for (let i = 1; i <= 3; i++) {
  await step(`unfullscreen ${i}`, () => p.eval('window.electron.window.setFullscreen(false)'), 2500)
  await step(`fullscreen ${i}`, () => p.eval('window.electron.window.setFullscreen(true)'), 2500)
  await step(`  pause`, () => p.eval(`window.mpvPrism.setProperty("${id}","pause",true)`), 2000)
  await step(`  unpause`, () => p.eval(`window.mpvPrism.setProperty("${id}","pause",false)`), 2000)
}
p.close()
