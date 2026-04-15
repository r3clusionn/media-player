// Sender -> Receiver messages via custom namespace
export type CastSenderMessage =
    | { type: "subtitleEvents"; payload: CastSubtitleEvent[] }
    | { type: "setTracks"; payload: CastTrackInfo[] }
    | { type: "switchTrack"; payload: { trackNumber: number } }
    | { type: "fonts"; payload: string[] }
    | { type: "subtitleHeader"; payload: string }
    | { type: "disableSubtitles" }

// Receiver -> Sender messages via custom namespace
export type CastReceiverMessage =
    | { type: "receiverReady" }
    | { type: "subtitleTrackChanged"; payload: { trackNumber: number } }
    | { type: "error"; payload: string }

export interface CastSubtitleEvent {
    trackNumber: number
    startTime: number
    duration: number
    text: string
    codecID: string
    uid?: string
    layer?: number
    style?: string
    extraData?: Record<string, string>
}

export interface CastTrackInfo {
    number: number
    name: string
    language: string
    languageIETF?: string
    codecID: string
    codecPrivate?: string
    default: boolean
    forced: boolean
}

export interface CastDevice {
    id: string
    name: string
    host: string
    port: number
}

export interface CastSessionState {
    connected: boolean
    device: CastDevice | null
    sessionId: string | null
}

export interface CastMediaStatus {
    mediaSessionId: number
    playerState: "IDLE" | "BUFFERING" | "PLAYING" | "PAUSED"
    currentTime: number
    duration?: number
    volume?: { level: number; muted: boolean }
    idleReason?: string
}

// IPC cast API exposed to the renderer
export interface ElectronCastAPI {
    discover: () => Promise<void>
    stopDiscovery: () => Promise<void>
    getDevices: () => Promise<CastDevice[]>
    connect: (deviceId: string) => Promise<CastSessionState>
    disconnect: () => Promise<void>
    getStatus: () => Promise<{ connected: boolean; device: CastDevice | null; sessionId: string | null; mediaStatus: CastMediaStatus | null }>
    loadMedia: (opts: {
        streamUrl: string
        contentType: string
        title?: string
        subtitle?: string
        imageUrl?: string
        duration?: number
        serverPort?: number
    }) => Promise<number>
    play: () => Promise<void>
    pause: () => Promise<void>
    seek: (time: number) => Promise<void>
    stop: () => Promise<void>
    setVolume: (level: number) => Promise<void>
    setMuted: (muted: boolean) => Promise<void>
    sendSubtitleEvents: (events: CastSubtitleEvent[]) => Promise<void>
    sendSubtitleTracks: (tracks: CastTrackInfo[]) => Promise<void>
    switchSubtitleTrack: (trackNumber: number) => Promise<void>
    sendFonts: (fontUrls: string[], serverPort?: number) => Promise<void>
    sendSubtitleHeader: (header: string) => Promise<void>
    disableSubtitles: () => Promise<void>
    getLanIP: () => Promise<string>
}







