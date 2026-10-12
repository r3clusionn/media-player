// Shows the control bar, clicks the control bar button nearest argv[2] from
// the right (0 = rightmost) and reports what menu appears, over time.
import { connect, sleep } from './cdp.mjs'
const p = await connect(9333)
const idx = Number(process.argv[2] || 3)
const move = (x, y) => p.send('Input.dispatchMouseEvent', { type: 'mouseMoved', x, y })
for (const y of [700, 800, 860, 880]) { await move(720, y); await sleep(120) }
await sleep(500)
const btns = JSON.parse(await p.eval(`JSON.stringify([...document.querySelectorAll("[data-vc-element=control-bar] button")].map(b=>{const r=b.getBoundingClientRect();return [Math.round(r.x+r.width/2),Math.round(r.y+r.height/2)]}))`))
const [x, y] = btns[btns.length - 1 - idx]
console.log('buttons', JSON.stringify(btns), 'clicking', x, y)
await move(x, y); await sleep(200)
for (const type of ['mousePressed', 'mouseReleased']) await p.send('Input.dispatchMouseEvent', { type, x, y, button: 'left', clickCount: 1 })
const probe = `(()=>{const m=document.querySelector("[role=menu],[role=dialog]:not(.UI-Drawer__content),[data-radix-popper-content-wrapper]");if(!m)return "no menu";const r=m.getBoundingClientRect();let w=m;while(w.parentElement&&w.parentElement!==document.body)w=w.parentElement;return "menu "+[r.x,r.y,r.width,r.height].map(Math.round)+" mask="+(w.style.webkitMask?"YES":"no")+" top="+w.className.slice(0,30)})()`
for (const t of [100, 600, 1200]) { await sleep(t === 100 ? 100 : t - 100); console.log(t + 'ms:', await p.eval(probe)) }
console.log(await p.eval(`(()=>{const m=document.querySelector("[role=menu],[data-radix-popper-content-wrapper]");if(!m)return "none";const out=[];for(let n=m;n&&n!==document.body;n=n.parentElement){const c=getComputedStyle(n);out.push(n.tagName+"."+String(n.className).slice(0,35)+" mask="+(n.style.webkitMask?"YES":"-")+" op="+c.opacity+" vis="+c.visibility+" cv="+c.contentVisibility)}return out.join("\\n")})()`))
p.close()
