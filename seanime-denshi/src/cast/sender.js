const { EventEmitter } = require("events")
const mdns = require("mdns-js")
const castv2 = require("castv2")
const os = require("os")
const log = require("electron-log/main")

const CAST_NAMESPACE = "urn:x-cast:com.denshi.cast"
const CAST_MEDIA_NAMESPACE = "urn:x-cast:com.google.cast.media"
const CAST_RECEIVER_NAMESPACE = "urn:x-cast:com.google.cast.receiver"
const CAST_CONNECTION_NAMESPACE = "urn:x-cast:com.google.cast.tp.connection"
const CAST_HEARTBEAT_NAMESPACE = "urn:x-cast:com.google.cast.tp.heartbeat"

// TODO: https://cast.google.com/publish/
const CAST_APP_ID = "TODO"

class CastSender extends EventEmitter {
    constructor() {
        super()
        this.devices = new Map()
        this.browser = null
        this.client = null
        this.connectedDevice = null
        this.session = null
        this.mediaSessionId = null
        this.heartbeatInterval = null
        this.currentMediaStatus = null
        this._requestId = 0
        this._pendingRequests = new Map()
    }

    _nextRequestId() {
        return ++this._requestId
    }

    // Returns the machine's LAN IP
    getLanIP() {
        const interfaces = os.networkInterfaces()
        for (const name of Object.keys(interfaces)) {
            for (const iface of interfaces[name]) {
                if (iface.family === "IPv4" && !iface.internal) {
                    return iface.address
                }
            }
        }
        return "127.0.0.1"
    }

    // Rewrite localhost URLs to LAN IP so the Chromecast can reach the server
    rewriteUrlForCast(url, serverPort) {
        const lanIP = this.getLanIP()
        return url
            .replace(/127\.0\.0\.1/g, lanIP)
            .replace(/localhost/g, lanIP)
            .replace(/{{SERVER_URL}}/g, `http://${lanIP}:${serverPort || 43211}`)
    }

    startDiscovery() {
        log.info("[Cast] Starting device discovery")
        this.devices.clear()

        try {
            this.browser = mdns.createBrowser(mdns.tcp("googlecast"))

            this.browser.on("ready", () => {
                log.info("[Cast] mDNS browser ready, discovering...")
                this.browser.discover()
            })

            this.browser.on("update", (service) => {
                if (!service.addresses || service.addresses.length === 0) return

                const deviceId = service.fullname || service.host
                const txtRecord = service.txt || []
                let friendlyName = deviceId

                // Get friendly name from TXT records
                if (Array.isArray(txtRecord)) {
                    for (const record of txtRecord) {
                        if (typeof record === "string" && record.startsWith("fn=")) {
                            friendlyName = record.substring(3)
                        }
                    }
                }

                const device = {
                    id: deviceId,
                    name: friendlyName,
                    host: service.addresses[0],
                    port: service.port || 8009,
                }

                if (!this.devices.has(deviceId)) {
                    log.info(`[Cast] Found device: ${friendlyName} at ${device.host}:${device.port}`)
                    this.devices.set(deviceId, device)
                    this.emit("deviceFound", device)
                }
            })
        } catch (err) {
            log.error("[Cast] Discovery error:", err)
            this.emit("error", { type: "discovery", message: err.message })
        }
    }

    stopDiscovery() {
        if (this.browser) {
            try {
                this.browser.stop()
            } catch (e) {
                // ignore
            }
            this.browser = null
        }
    }

    getDevices() {
        return Array.from(this.devices.values())
    }

    async connect(deviceId) {
        const device = this.devices.get(deviceId)
        if (!device) {
            throw new Error(`Device not found: ${deviceId}`)
        }

        log.info(`[Cast] Connecting to ${device.name} at ${device.host}:${device.port}`)

        return new Promise((resolve, reject) => {
            this.client = new castv2.Client()

            this.client.on("error", (err) => {
                log.error("[Cast] Client error:", err)
                this.emit("error", { type: "connection", message: err.message })
                this._cleanup()
                reject(err)
            })

            this.client.connect({ host: device.host, port: device.port }, () => {
                log.info("[Cast] Connected to device")
                this.connectedDevice = device

                // Set up connection and heartbeat
                this._setupPlatformChannels()

                // Launch the receiver app
                this._launchApp()
                    .then((session) => {
                        this.session = session
                        log.info("[Cast] App launched, session:", session.sessionId)
                        this.emit("sessionUpdate", {
                            connected: true,
                            device: device,
                            sessionId: session.sessionId,
                        })
                        resolve(session)
                    })
                    .catch(reject)
            })
        })
    }

    // Set up the transport-level connection and heartbeat
    _setupPlatformChannels() {
        this._connectionChannel = this.client.createChannel(
            "sender-0", "receiver-0",
            CAST_CONNECTION_NAMESPACE, "JSON"
        )
        this._connectionChannel.send({ type: "CONNECT" })

        this._heartbeatChannel = this.client.createChannel(
            "sender-0", "receiver-0",
            CAST_HEARTBEAT_NAMESPACE, "JSON"
        )

        this._heartbeatChannel.on("message", (data) => {
            if (data.type === "PING") {
                this._heartbeatChannel.send({ type: "PONG" })
            }
        })

        // Heartbeat every 5s
        this.heartbeatInterval = setInterval(() => {
            try {
                this._heartbeatChannel.send({ type: "PING" })
            } catch (e) {
                // Lost connection
                this._cleanup()
            }
        }, 5000)

        this._receiverChannel = this.client.createChannel(
            "sender-0", "receiver-0",
            CAST_RECEIVER_NAMESPACE, "JSON"
        )

        this._receiverChannel.on("message", (data) => {
            if (data.type === "RECEIVER_STATUS") {
                this._handleReceiverStatus(data.status)
            }
        })
    }

    _handleReceiverStatus(status) {
        if (status && status.applications) {
            const app = status.applications.find(a => a.appId === CAST_APP_ID)
            if (app) {
                this._transportId = app.transportId
                this._setupAppChannels(app.transportId)
            }
        }
    }

    _setupAppChannels(transportId) {
        if (this._appChannelsSetup) return
        this._appChannelsSetup = true

        // App connection
        this._appConnectionChannel = this.client.createChannel(
            "sender-0", transportId,
            CAST_CONNECTION_NAMESPACE, "JSON"
        )
        this._appConnectionChannel.send({ type: "CONNECT" })

        this._mediaChannel = this.client.createChannel(
            "sender-0", transportId,
            CAST_MEDIA_NAMESPACE, "JSON"
        )

        this._mediaChannel.on("message", (data) => {
            this._handleMediaMessage(data)
        })

        // Subtitles/fonts channel
        this._customChannel = this.client.createChannel(
            "sender-0", transportId,
            CAST_NAMESPACE, "JSON"
        )

        this._customChannel.on("message", (data) => {
            this._handleCustomMessage(data)
        })
    }

    _launchApp() {
        return new Promise((resolve, reject) => {
            const requestId = this._nextRequestId()

            const handler = (data) => {
                if (data.requestId === requestId) {
                    this._receiverChannel.removeListener("message", handler)

                    if (data.type === "RECEIVER_STATUS" && data.status?.applications) {
                        const app = data.status.applications.find(a => a.appId === CAST_APP_ID)
                        if (app) {
                            resolve({
                                sessionId: app.sessionId,
                                transportId: app.transportId,
                            })
                            return
                        }
                    }
                    reject(new Error("Failed to launch app"))
                }
            }

            this._receiverChannel.on("message", handler)

            this._receiverChannel.send({
                type: "LAUNCH",
                appId: CAST_APP_ID,
                requestId: requestId,
            })

            setTimeout(() => {
                this._receiverChannel.removeListener("message", handler)
                reject(new Error("App launch timed out"))
            }, 15000)
        })
    }

    _handleMediaMessage(data) {
        if (data.type === "MEDIA_STATUS" && data.status && data.status.length > 0) {
            const status = data.status[0]
            this.mediaSessionId = status.mediaSessionId
            this.currentMediaStatus = status

            this.emit("mediaStatus", {
                mediaSessionId: status.mediaSessionId,
                playerState: status.playerState,
                currentTime: status.currentTime,
                duration: status.media?.duration,
                volume: status.volume,
                idleReason: status.idleReason,
            })
        }
    }

    _handleCustomMessage(data) {
        log.info("[Cast] Custom message from receiver:", data.type)

        switch (data.type) {
            case "receiverReady":
                this.emit("receiverReady")
                break
            case "subtitleTrackChanged":
                this.emit("subtitleTrackChanged", data.payload)
                break
            case "error":
                log.error("[Cast] Receiver error:", data.payload)
                this.emit("error", { type: "receiver", message: data.payload })
                break
        }
    }

    // Load media on the Chromecast
    // MKV files are spoofed as video/mp4 so the HTML5 video element accepts them
    loadMedia({ streamUrl, contentType, title, subtitle, imageUrl, duration, serverPort }) {
        if (!this._mediaChannel) {
            throw new Error("Not connected to a Cast device")
        }

        // Rewrite URL for LAN access
        const castUrl = this.rewriteUrlForCast(streamUrl, serverPort)

        // Spoof MKV as MP4
        let castContentType = contentType
        if (contentType === "video/x-matroska" || contentType === "video/webm") {
            log.info("[Cast] Spoofing MIME type from", contentType, "to video/mp4")
            castContentType = "video/mp4"
        }

        const requestId = this._nextRequestId()

        const loadRequest = {
            type: "LOAD",
            requestId: requestId,
            media: {
                contentId: castUrl,
                contentType: castContentType,
                streamType: "BUFFERED",
                metadata: {
                    type: 0,
                    metadataType: 0,
                    title: title || "Unknown",
                    subtitle: subtitle || "",
                    images: imageUrl ? [{ url: imageUrl }] : [],
                },
                ...(duration ? { duration } : {}),
            },
            autoplay: true,
            currentTime: 0,
        }

        log.info("[Cast] Loading media:", castUrl, "as", castContentType)
        this._mediaChannel.send(loadRequest)

        return requestId
    }

    play() {
        if (!this._mediaChannel || !this.mediaSessionId) return
        this._mediaChannel.send({
            type: "PLAY",
            mediaSessionId: this.mediaSessionId,
            requestId: this._nextRequestId(),
        })
    }

    pause() {
        if (!this._mediaChannel || !this.mediaSessionId) return
        this._mediaChannel.send({
            type: "PAUSE",
            mediaSessionId: this.mediaSessionId,
            requestId: this._nextRequestId(),
        })
    }

    seek(time) {
        if (!this._mediaChannel || !this.mediaSessionId) return
        this._mediaChannel.send({
            type: "SEEK",
            mediaSessionId: this.mediaSessionId,
            currentTime: time,
            requestId: this._nextRequestId(),
        })
    }

    stop() {
        if (!this._mediaChannel || !this.mediaSessionId) return
        this._mediaChannel.send({
            type: "STOP",
            mediaSessionId: this.mediaSessionId,
            requestId: this._nextRequestId(),
        })
    }

    setVolume(level) {
        if (!this._receiverChannel) return
        this._receiverChannel.send({
            type: "SET_VOLUME",
            volume: { level: Math.max(0, Math.min(1, level)) },
            requestId: this._nextRequestId(),
        })
    }

    setMuted(muted) {
        if (!this._receiverChannel) return
        this._receiverChannel.send({
            type: "SET_VOLUME",
            volume: { muted: !!muted },
            requestId: this._nextRequestId(),
        })
    }

    getMediaStatus() {
        if (!this._mediaChannel) return
        this._mediaChannel.send({
            type: "GET_STATUS",
            requestId: this._nextRequestId(),
        })
    }

    // Subtitle & Font Relay

    sendSubtitleEvents(events) {
        if (!this._customChannel) return
        this._customChannel.send({
            type: "subtitleEvents",
            payload: events,
        })
    }

    sendSubtitleTracks(tracks) {
        if (!this._customChannel) return
        this._customChannel.send({
            type: "setTracks",
            payload: tracks,
        })
    }

    switchSubtitleTrack(trackNumber) {
        if (!this._customChannel) return
        this._customChannel.send({
            type: "switchTrack",
            payload: { trackNumber },
        })
    }

    // Send font URLs to the receiver, rewritten for LAN access
    sendFonts(fontUrls, serverPort) {
        if (!this._customChannel) return
        const rewrittenUrls = fontUrls.map(url => this.rewriteUrlForCast(url, serverPort))
        this._customChannel.send({
            type: "fonts",
            payload: rewrittenUrls,
        })
    }

    sendSubtitleHeader(header) {
        if (!this._customChannel) return
        this._customChannel.send({
            type: "subtitleHeader",
            payload: header,
        })
    }

    disableSubtitles() {
        if (!this._customChannel) return
        this._customChannel.send({
            type: "disableSubtitles",
        })
    }

    disconnect() {
        log.info("[Cast] Disconnecting")

        try {
            if (this._receiverChannel) {
                this._receiverChannel.send({
                    type: "STOP",
                    requestId: this._nextRequestId(),
                })
            }
        } catch (e) {
            // ignore
        }

        this._cleanup()

        this.emit("sessionUpdate", {
            connected: false,
            device: null,
            sessionId: null,
        })
    }

    _cleanup() {
        if (this.heartbeatInterval) {
            clearInterval(this.heartbeatInterval)
            this.heartbeatInterval = null
        }

        if (this.client) {
            try {
                this.client.close()
            } catch (e) {
                // ignore
            }
            this.client = null
        }

        this.connectedDevice = null
        this.session = null
        this.mediaSessionId = null
        this.currentMediaStatus = null
        this._appChannelsSetup = false
        this._mediaChannel = null
        this._customChannel = null
        this._connectionChannel = null
        this._heartbeatChannel = null
        this._receiverChannel = null
        this._appConnectionChannel = null
        this._transportId = null
    }

    getStatus() {
        return {
            connected: !!this.connectedDevice,
            device: this.connectedDevice,
            sessionId: this.session?.sessionId || null,
            mediaStatus: this.currentMediaStatus,
        }
    }

    destroy() {
        this.stopDiscovery()
        this.disconnect()
        this.removeAllListeners()
    }
}

module.exports = { CastSender, CAST_NAMESPACE, CAST_APP_ID }



































