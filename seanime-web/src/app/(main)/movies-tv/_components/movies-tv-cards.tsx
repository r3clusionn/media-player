// media-player: cards and rows for the Movies & TV section, styled like
// Seanime's anime cards (rounded posters, hover lift, muted metadata).
import { Stremio_MetaPreview } from "@/api/hooks/stremio.hooks"
import { SeaLink } from "@/components/shared/sea-link"
import { cn } from "@/components/ui/core/styling"
import { Skeleton } from "@/components/ui/skeleton"
import React from "react"
import { FaStar } from "react-icons/fa"

export function titleHref(type: string, id: string) {
    return `/movies-tv/title?type=${type}&id=${encodeURIComponent(id)}`
}

export function MoviesTvPosterCard(props: { meta: Stremio_MetaPreview, className?: string }) {
    const { meta, className } = props
    return (
        <SeaLink href={titleHref(meta.type, meta.id)} className={cn("group/card block w-[10.5rem] md:w-[12rem] flex-none space-y-2", className)}>
            <div className="relative aspect-[2/3] w-full overflow-hidden rounded-xl bg-gray-900 ring-1 ring-white/5 transition duration-200 group-hover/card:-translate-y-1 group-hover/card:ring-white/20">
                {meta.poster ? (
                    <img
                        src={meta.poster}
                        alt=""
                        loading="lazy"
                        className="h-full w-full object-cover transition duration-300 group-hover/card:scale-[1.04]"
                    />
                ) : (
                    <div className="flex h-full items-center justify-center p-3 text-center text-sm text-[--muted]">{meta.name}</div>
                )}
                {!!meta.imdbRating && (
                    <div className="absolute left-2 top-2 flex items-center gap-1 rounded-md bg-black/70 px-1.5 py-0.5 text-xs font-semibold backdrop-blur">
                        <FaStar className="text-yellow-400 text-[0.65rem]" /> {meta.imdbRating}
                    </div>
                )}
            </div>
            <div className="px-0.5">
                <p className="line-clamp-1 text-sm font-semibold">{meta.name}</p>
                <p className="text-xs text-[--muted]">{meta.releaseInfo}</p>
            </div>
        </SeaLink>
    )
}

export function MoviesTvRow(props: { title: string, children?: React.ReactNode, isLoading?: boolean, empty?: boolean }) {
    const { title, children, isLoading, empty } = props
    if (!isLoading && empty) return null
    return (
        <section className="space-y-3">
            <h2 className="text-xl font-bold">{title}</h2>
            <div className="flex gap-4 overflow-x-auto pb-3 [scrollbar-width:thin] snap-x">
                {isLoading
                    ? Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} className="aspect-[2/3] w-[10.5rem] md:w-[12rem] flex-none rounded-xl" />)
                    : children}
            </div>
        </section>
    )
}
