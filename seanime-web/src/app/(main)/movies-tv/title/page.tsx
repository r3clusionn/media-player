// media-player: a movie or series page in the Movies & TV section. Layout
// follows Seanime's anime entry page: banner, poster and details, then the
// episodes (season tabs) and a stream list from Torrentio / Comet.
import { Stremio_Stream, Stremio_Video, useStremioMeta, useStremioPlay, useStremioStreams } from "@/api/hooks/stremio.hooks"
import { clientIdAtom } from "@/app/websocket-provider"
import { PageWrapper } from "@/components/shared/page-wrapper"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { cn } from "@/components/ui/core/styling"
import { Drawer } from "@/components/ui/drawer"
import { LoadingSpinner } from "@/components/ui/loading-spinner"
import { useSearchParams } from "@/lib/navigation"
import { useAtomValue } from "jotai"
import React from "react"
import { FaStar } from "react-icons/fa"
import { LuPlay, LuZap } from "react-icons/lu"
import { toast } from "sonner"

function episodeLabel(v: Stremio_Video) {
    return v.name || v.title || `Episode ${v.episode}`
}

function StreamList(props: { type: string, videoId: string, onPlayed: () => void }) {
    const { type, videoId, onPlayed } = props
    const { data: streams, isLoading, error } = useStremioStreams(type, videoId)
    const play = useStremioPlay()
    const clientId = useAtomValue(clientIdAtom)
    const [playing, setPlaying] = React.useState<string | null>(null)

    // "Play best" lets the server move on to the next cached stream when one
    // isn't ready on Real-Debrid; a stream picked by hand plays exactly that.
    function start(stream: Stremio_Stream, fallback = false) {
        setPlaying(stream.url)
        play.mutate({ type, videoId, stream, fallback, clientId: clientId || undefined }, {
            onSuccess: () => onPlayed(),
            onError: err => toast.error(err.message),
            onSettled: () => setPlaying(null),
        })
    }

    if (isLoading) return <LoadingSpinner title="Asking Torrentio and Comet..." />
    if (error) return <p className="text-red-300">Could not get streams: {String(error.message || error)}</p>
    if (!streams?.length) return <p className="text-[--muted]">No streams found for this one.</p>

    const best = streams.find(s => s.cached) || streams[0]

    return (
        <div className="space-y-4">
            <Button
                intent="primary"
                leftIcon={<LuPlay />}
                loading={play.isPending && playing === best.url}
                onClick={() => start(best, true)}
                className="w-full"
            >
                Play best {best.cached ? "(cached)" : ""} · {best.quality || "?"} {best.hdr || ""} {best.size ? `· ${best.size}` : ""}
            </Button>
            <div className="space-y-2">
                {streams.slice(0, 80).map(s => (
                    <button
                        key={s.url}
                        type="button"
                        disabled={play.isPending}
                        onClick={() => start(s)}
                        className={cn(
                            "w-full rounded-lg border border-white/5 bg-gray-950/60 p-3 text-left transition hover:border-white/20 hover:bg-gray-900",
                            playing === s.url && "border-[--brand] opacity-80",
                        )}
                    >
                        <div className="flex flex-wrap items-center gap-1.5">
                            {s.cached && <Badge intent="success" size="sm" leftIcon={<LuZap />}>Cached</Badge>}
                            {s.quality && <Badge intent="gray" size="sm">{s.quality}</Badge>}
                            {s.hdr && <Badge intent="warning" size="sm">{s.hdr}</Badge>}
                            {s.size && <Badge intent="gray" size="sm">{s.size}</Badge>}
                            <Badge intent={s.source === "torrentio" ? "info" : "indigo"} size="sm">{s.source === "torrentio" ? "Torrentio" : "Comet"}</Badge>
                        </div>
                        <p className="mt-2 line-clamp-2 break-all text-sm text-gray-300">{s.filename || s.title.split("\n")[0]}</p>
                    </button>
                ))}
            </div>
        </div>
    )
}

export default function Page() {
    const searchParams = useSearchParams()
    const type = searchParams.get("type") || undefined
    const id = searchParams.get("id") || undefined
    const initialVideo = searchParams.get("video") || undefined
    const { data: meta, isLoading } = useStremioMeta(type, id)

    const seasons = React.useMemo(() => {
        const set = new Set<number>()
        for (const v of meta?.videos ?? []) set.add(v.season)
        return [...set].sort((a, b) => (a === 0 ? 1 : b === 0 ? -1 : a - b)) // specials last
    }, [meta])
    const [season, setSeason] = React.useState<number | null>(null)
    const [streamsFor, setStreamsFor] = React.useState<string | null>(null)

    React.useEffect(() => {
        if (!meta) return
        if (initialVideo) {
            const v = meta.videos?.find(v => v.id === initialVideo)
            if (v) setSeason(v.season)
            setStreamsFor(initialVideo)
        } else if (season === null && seasons.length) {
            setSeason(seasons.find(s => s > 0) ?? seasons[0])
        }
    }, [meta, initialVideo])

    if (isLoading) return <LoadingSpinner />
    if (!meta || !type || !id) return <PageWrapper className="p-8"><p>Title not found.</p></PageWrapper>

    const episodes = (meta.videos ?? []).filter(v => v.season === season)
    const drawerVideo = streamsFor && type === "series" ? meta.videos?.find(v => v.id === streamsFor) : undefined

    return (
        <div className="relative">
            <div className="absolute inset-x-0 top-0 h-[32rem] overflow-hidden">
                {meta.background && <img src={meta.background} alt="" className="h-full w-full object-cover opacity-40" />}
                <div className="absolute inset-0 bg-gradient-to-t from-[--background] via-[--background]/70 to-transparent" />
            </div>
            <PageWrapper className="relative z-[1] p-4 sm:p-8 space-y-10 pb-10">
                <div className="flex flex-col gap-8 pt-10 md:flex-row">
                    {meta.poster && (
                        <img src={meta.poster} alt="" className="w-44 md:w-56 flex-none self-start rounded-xl shadow-2xl ring-1 ring-white/10" />
                    )}
                    <div className="space-y-4 max-w-4xl">
                        {meta.logo ? <img src={meta.logo} alt={meta.name} className="max-h-24 max-w-md object-contain" /> :
                            <h1 className="text-4xl font-bold">{meta.name}</h1>}
                        <div className="flex flex-wrap items-center gap-3 text-sm text-gray-300">
                            {meta.releaseInfo && <span>{meta.releaseInfo}</span>}
                            {meta.runtime && <span>{meta.runtime}</span>}
                            {meta.imdbRating && <span className="flex items-center gap-1"><FaStar className="text-yellow-400" /> {meta.imdbRating}</span>}
                            {meta.genres?.slice(0, 4).map(g => <Badge key={g} intent="gray" size="md">{g}</Badge>)}
                        </div>
                        {meta.description && <p className="text-gray-300 leading-relaxed line-clamp-5">{meta.description}</p>}
                        {!!meta.cast?.length && <p className="text-sm text-[--muted]">With {meta.cast.slice(0, 5).join(", ")}</p>}
                        {type === "movie" && (
                            <Button intent="primary" size="lg" leftIcon={<LuPlay />} onClick={() => setStreamsFor(id)}>Watch</Button>
                        )}
                    </div>
                </div>

                {type === "series" && seasons.length > 0 && (
                    <section className="space-y-5">
                        <div className="flex flex-wrap gap-2">
                            {seasons.map(s => (
                                <Button key={s} size="sm" intent={s === season ? "primary" : "gray-subtle"} onClick={() => setSeason(s)}>
                                    {s === 0 ? "Specials" : `Season ${s}`}
                                </Button>
                            ))}
                        </div>
                        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
                            {episodes.map(v => (
                                <button
                                    key={v.id}
                                    type="button"
                                    onClick={() => setStreamsFor(v.id)}
                                    className="group/ep space-y-2 text-left"
                                >
                                    <div className="relative aspect-video overflow-hidden rounded-xl bg-gray-900 ring-1 ring-white/5 group-hover/ep:ring-white/20">
                                        {(v.thumbnail || meta.background) &&
                                            <img src={v.thumbnail || meta.background} alt="" loading="lazy" className="h-full w-full object-cover transition duration-300 group-hover/ep:scale-[1.03]" />}
                                        <div className="absolute inset-0 flex items-center justify-center opacity-0 transition group-hover/ep:opacity-100">
                                            <LuPlay className="text-5xl text-white drop-shadow" />
                                        </div>
                                    </div>
                                    <div>
                                        <p className="font-semibold line-clamp-1">{episodeLabel(v)}</p>
                                        <p className="text-sm text-[--muted]">Episode {v.episode}{v.released ? ` · ${new Date(v.released).toLocaleDateString()}` : ""}</p>
                                    </div>
                                </button>
                            ))}
                        </div>
                    </section>
                )}
            </PageWrapper>

            <Drawer
                open={!!streamsFor}
                onOpenChange={open => !open && setStreamsFor(null)}
                size="lg"
                title={drawerVideo ? `${meta.name} · S${drawerVideo.season} E${drawerVideo.episode} · ${episodeLabel(drawerVideo)}` : meta.name}
            >
                {streamsFor && <StreamList type={type} videoId={streamsFor} onPlayed={() => setStreamsFor(null)} />}
            </Drawer>
        </div>
    )
}
