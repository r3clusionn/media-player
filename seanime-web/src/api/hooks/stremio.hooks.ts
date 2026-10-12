// media-player: Movies & TV (internal/stremio, routes under /api/v1/stremio).
import { useServerMutation, useServerQuery } from "@/api/client/requests"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

export type Stremio_MetaPreview = {
    id: string
    type: "movie" | "series"
    name: string
    poster?: string
    background?: string
    logo?: string
    description?: string
    releaseInfo?: string
    imdbRating?: string
    genres?: string[]
    runtime?: string
}

export type Stremio_Video = {
    id: string
    name?: string
    title?: string
    season: number
    episode: number
    released?: string
    thumbnail?: string
    overview?: string
}

export type Stremio_Meta = Stremio_MetaPreview & {
    cast?: string[]
    director?: string[]
    videos?: Stremio_Video[]
}

export type Stremio_Stream = {
    source: "torrentio" | "comet"
    name: string
    title: string
    filename?: string
    quality?: string
    hdr?: string
    size?: string
    sizeBytes?: number
    cached: boolean
    url: string
    bingeGroup?: string
}

export type Stremio_TitleRef = {
    mediaId: number
    type: "movie" | "series"
    imdbId: string
    name: string
    poster?: string
    background?: string
    videoId: string
    season?: number
    episode?: number
    episodeTitle?: string
    thumbnail?: string
}

export type Stremio_ContinueItem = {
    title: Stremio_TitleRef
    videoId: string
    season?: number
    episode?: number
    currentTime: number
    duration: number
    nextUp: boolean
}

const KEY = "stremio"

export function useStremioCatalog(type: "movie" | "series", id: string, genre?: string) {
    const q = genre ? `?genre=${encodeURIComponent(genre)}` : ""
    return useServerQuery<Stremio_MetaPreview[]>({
        endpoint: `/api/v1/stremio/catalog/${type}/${id}${q}`,
        method: "GET",
        queryKey: [KEY, "catalog", type, id, genre],
        enabled: true,
        staleTime: 60 * 60 * 1000,
    })
}

export function useStremioSearch(query: string) {
    return useServerQuery<Stremio_MetaPreview[]>({
        endpoint: `/api/v1/stremio/search?q=${encodeURIComponent(query)}`,
        method: "GET",
        queryKey: [KEY, "search", query],
        enabled: query.trim().length > 1,
    })
}

export function useStremioMeta(type: string | undefined, id: string | undefined) {
    return useServerQuery<Stremio_Meta>({
        endpoint: `/api/v1/stremio/meta/${type}/${id}`,
        method: "GET",
        queryKey: [KEY, "meta", type, id],
        enabled: !!type && !!id,
        staleTime: 60 * 60 * 1000,
    })
}

export type Stremio_Source = "torrentio" | "comet"

export function useStremioStreams(type: string | undefined, videoId: string | undefined, source: Stremio_Source) {
    return useServerQuery<Stremio_Stream[]>({
        endpoint: `/api/v1/stremio/streams/${type}/${videoId}?source=${source}`,
        method: "GET",
        queryKey: [KEY, "streams", type, videoId, source],
        enabled: !!type && !!videoId,
        staleTime: 10 * 60 * 1000,
    })
}

export function useStremioContinue() {
    return useServerQuery<Stremio_ContinueItem[]>({
        endpoint: "/api/v1/stremio/continue",
        method: "GET",
        queryKey: [KEY, "continue"],
        enabled: true,
    })
}

export function useStremioPlay() {
    return useServerMutation<boolean, { type: string, videoId: string, stream: Stremio_Stream, fallback?: boolean, clientId?: string }>({
        endpoint: "/api/v1/stremio/play",
        method: "POST",
        mutationKey: [KEY, "play"],
    })
}

export function useStremioPlayNext() {
    return useServerMutation<boolean, { mediaId: number, clientId?: string }>({
        endpoint: "/api/v1/stremio/play-next",
        method: "POST",
        mutationKey: [KEY, "play-next"],
    })
}

export function useStremioForget() {
    const qc = useQueryClient()
    return useServerMutation<boolean, { mediaId: number }>({
        endpoint: "/api/v1/stremio/forget",
        method: "POST",
        mutationKey: [KEY, "forget"],
        onSuccess: async () => {
            await qc.invalidateQueries({ queryKey: [KEY, "continue"] })
            toast.success("Removed from Continue Watching")
        },
    })
}
