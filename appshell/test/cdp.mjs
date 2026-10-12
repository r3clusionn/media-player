// Minimal Chrome DevTools Protocol client for driving media-player in tests.
// Start the app with APPSHELL_REMOTE_DEBUG=<port>; Node's built-in WebSocket
// is enough, nothing to install.
//
//   import { connect } from "./cdp.mjs"
//   const page = await connect(9333)
//   await page.eval("document.title")

export async function connect(port, { timeoutMs = 20000 } = {}) {
    if (process.env.CDP_PORT) port = Number(process.env.CDP_PORT)
    const deadline = Date.now() + timeoutMs
    let target
    while (!target) {
        try {
            const list = await (await fetch(`http://127.0.0.1:${port}/json`)).json()
            target = list.find(t => t.type === "page")
        } catch {}
        if (!target) {
            if (Date.now() > deadline) throw new Error("no page on the DevTools port")
            await sleep(200)
        }
    }
    const ws = new WebSocket(target.webSocketDebuggerUrl)
    await new Promise((res, rej) => { ws.onopen = res; ws.onerror = rej })
    let nextId = 1
    const pending = new Map()
    const listeners = []
    ws.onmessage = (m) => {
        const msg = JSON.parse(m.data)
        if (msg.id && pending.has(msg.id)) {
            const { resolve, reject } = pending.get(msg.id)
            pending.delete(msg.id)
            msg.error ? reject(new Error(msg.error.message)) : resolve(msg.result)
        } else if (msg.method) {
            for (const l of listeners) l(msg)
        }
    }
    const send = (method, params = {}) => new Promise((resolve, reject) => {
        const id = nextId++
        pending.set(id, { resolve, reject })
        ws.send(JSON.stringify({ id, method, params }))
    })
    await send("Runtime.enable")
    await send("Page.enable")

    const page = {
        send,
        on: (fn) => listeners.push(fn),
        close: () => ws.close(),
        // Evaluates an expression (awaits promises) and returns its value.
        async eval(expr) {
            const r = await send("Runtime.evaluate", { expression: expr, awaitPromise: true, returnByValue: true })
            if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || r.exceptionDetails.text)
            return r.result.value
        },
        async waitFor(expr, { timeoutMs = 15000, intervalMs = 200 } = {}) {
            const end = Date.now() + timeoutMs
            for (;;) {
                const v = await page.eval(expr).catch(() => undefined)
                if (v) return v
                if (Date.now() > end) throw new Error(`timed out waiting for: ${expr}`)
                await sleep(intervalMs)
            }
        },
        // Clicks the first element matching a CSS selector, or whose text
        // contains `text` (when given), like a user would.
        async click(selector, text) {
            const ok = await page.eval(`(() => {
                const els = [...document.querySelectorAll(${JSON.stringify(selector)})]
                const el = ${text ? `els.find(e => e.textContent.includes(${JSON.stringify(text)}))` : "els[0]"}
                if (!el) return false
                el.scrollIntoView({ block: "center" })
                const r = el.getBoundingClientRect()
                return { x: r.left + r.width / 2, y: r.top + r.height / 2 }
            })()`)
            if (!ok) throw new Error(`nothing to click: ${selector} ${text || ""}`)
            for (const type of ["mouseMoved", "mousePressed", "mouseReleased"]) {
                await send("Input.dispatchMouseEvent", { type, x: ok.x, y: ok.y, button: "left", clickCount: 1 })
            }
            return true
        },
        async navigate(url) {
            await send("Page.navigate", { url })
        },
    }
    return page
}

export const sleep = (ms) => new Promise(r => setTimeout(r, ms))
