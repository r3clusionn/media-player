// Lists the unmasked, painted elements over points of the video (debugging see-through holes).
import { connect } from './cdp.mjs'
const p = await connect(9333)
console.log(await p.eval(`(()=>{
  const out={vw:innerWidth,vh:innerHeight}
  for (const [x,y] of [[innerWidth*0.25,innerHeight/2],[innerWidth*0.75,innerHeight/2]]) {
    out[Math.round(x)+","+Math.round(y)] = document.elementsFromPoint(x,y).map(e=>{const c=getComputedStyle(e);const r=e.getBoundingClientRect();
      return [e.tagName,String(e.className?.baseVal??e.className).slice(0,40),c.backgroundColor,(e.style.getPropertyValue("-webkit-mask")||"").replace(/linear-gradient\\(#000 0 0\\)/g,"G").slice(0,60),[r.left,r.top,r.width,r.height].map(Math.round).join(","),c.transform==="none"?"":c.transform]}).filter(a=>a[3]||a[2]!=="rgba(0, 0, 0, 0)"||a[0]==="IMG")
  }
  return JSON.stringify(out,null,1)})()`))
p.close()
