// Lists running CSS/Web animations and media elements that may keep Chromium repainting.
import { connect, sleep } from "./cdp.mjs"
const page = await connect(9333)
console.log(await page.eval(`JSON.stringify({
  animations: document.getAnimations().filter(a => a.playState === 'running').slice(0, 15).map(a => {
    const t = a.effect && a.effect.target
    return [a.animationName || a.constructor.name, t ? t.tagName + '.' + [...t.classList].slice(0, 3).join('.') : '', a.effect?.getTiming?.().iterations]
  }),
  videos: [...document.querySelectorAll('video')].map(v => ({ paused: v.paused, srcObject: !!v.srcObject, src: (v.src||'').slice(0, 40), rs: v.readyState, attrs: [...v.attributes].map(a=>a.name).join(',') })),
  canvases: document.querySelectorAll('canvas').length })`))
page.close()
