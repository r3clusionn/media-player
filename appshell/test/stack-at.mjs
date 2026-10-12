// Lists the elements under a point (default: window centre) with their backgrounds.
import { connect } from "./cdp.mjs"
const page = await connect(9333)
console.log(await page.eval(`JSON.stringify(document.elementsFromPoint(innerWidth/2, innerHeight/2).map(e => {
  const cs = getComputedStyle(e)
  return [e.tagName.toLowerCase() + (e.id ? '#' + e.id : '') + '.' + [...e.classList].slice(0, 4).join('.'), cs.backgroundColor, cs.backgroundImage.slice(0, 40), cs.opacity, cs.webkitMask ? cs.webkitMask.slice(0, 30) : '']
}), null, 0)`))
page.close()
