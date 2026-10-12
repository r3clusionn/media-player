// media-player: Movies & TV home: search, Continue Watching / Next Up, and
// catalogs from Cinemeta. Streams come from Torrentio (Comet as a backup)
// through Real-Debrid and play in Seanime's own player.
import { Stremio_ContinueItem, useStremioCatalog, useStremioContinue, useStremioForget, useStremioSearch } from "@/api/hooks/stremio.hooks"
import { MoviesTvPosterCard, MoviesTvRow, titleHref } from "@/app/(main)/movies-tv/_components/movies-tv-cards"
import { PageWrapper } from "@/components/shared/page-wrapper"
import { SeaLink } from "@/components/shared/sea-link"
import { IconButton } from "@/components/ui/button"
import { TextInput } from "@/components/ui/text-input"
import { useDebounce } from "@/hooks/use-debounce"
import React from "react"
import { FiSearch } from "react-icons/fi"
import { LuX } from "react-icons/lu"

function ContinueCard({ item }: { item: Stremio_ContinueItem }) {
    const forget = useStremioForget()
    const t = item.title
    const progress = item.duration > 0 ? Math.min(100, (item.currentTime / item.duration) * 100) : 0
    const label = t.type === "series" ? `S${item.season} E${item.episode}` : (t.name)
    const image = (t.type === "series" && !item.nextUp ? t.thumbnail : undefined) || t.background || t.poster
    return (
        <div className="group/card relative w-[18rem] md:w-[20rem] flex-none snap-start">
            <SeaLink href={titleHref(t.type, t.imdbId) + (t.type === "series" ? `&video=${encodeURIComponent(item.videoId)}` : "")} className="block space-y-2">
                <div className="relative aspect-video w-full overflow-hidden rounded-xl bg-gray-900 ring-1 ring-white/5 transition duration-200 group-hover/card:ring-white/20">
                    {image && <img src={image} alt="" loading="lazy" className="h-full w-full object-cover transition duration-300 group-hover/card:scale-[1.03]" />}
                    <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/90 via-black/40 to-transparent p-3 pt-10">
                        <p className="line-clamp-1 font-semibold">{t.name}</p>
                        <p className="text-xs text-gray-300">{item.nextUp ? `Next up · ${label}` : label}</p>
                    </div>
                    {!item.nextUp && (
                        <div className="absolute inset-x-0 bottom-0 h-1 bg-white/10">
                            <div className="h-full bg-[--brand]" style={{ width: `${progress}%` }} />
                        </div>
                    )}
                </div>
            </SeaLink>
            <IconButton
                icon={<LuX />}
                size="xs"
                intent="gray-basic"
                className="absolute right-2 top-2 opacity-0 group-hover/card:opacity-100 bg-black/60"
                onClick={() => forget.mutate({ mediaId: t.mediaId })}
                aria-label="Remove from Continue Watching"
            />
        </div>
    )
}

export default function Page() {
    const [query, setQuery] = React.useState("")
    const debounced = useDebounce(query, 400)
    const search = useStremioSearch(debounced)
    const cont = useStremioContinue()
    const popularMovies = useStremioCatalog("movie", "top")
    const popularSeries = useStremioCatalog("series", "top")
    const ratedMovies = useStremioCatalog("movie", "imdbRating")
    const ratedSeries = useStremioCatalog("series", "imdbRating")

    const searching = debounced.trim().length > 1

    return (
        <PageWrapper className="p-4 sm:p-8 space-y-10 pb-10">
            <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
                <div className="space-y-1">
                    <h1 className="text-3xl font-bold">Movies & TV</h1>
                    <p className="text-[--muted]">From Torrentio and Comet through Real-Debrid</p>
                </div>
                <div className="w-full lg:w-[28rem]">
                    <TextInput
                        value={query}
                        onValueChange={setQuery}
                        placeholder="Search movies and series"
                        leftIcon={<FiSearch />}
                        autoFocus={false}
                    />
                </div>
            </div>

            {searching ? (
                <MoviesTvRow title={`Results for "${debounced}"`} isLoading={search.isLoading} empty={!search.data?.length}>
                    {search.data?.map(m => <MoviesTvPosterCard key={m.type + m.id} meta={m} />)}
                </MoviesTvRow>
            ) : (
                <>
                    <MoviesTvRow title="Continue watching" isLoading={cont.isLoading} empty={!cont.data?.length}>
                        {cont.data?.map(item => <ContinueCard key={item.title.mediaId} item={item} />)}
                    </MoviesTvRow>
                    <MoviesTvRow title="Popular movies" isLoading={popularMovies.isLoading} empty={!popularMovies.data?.length}>
                        {popularMovies.data?.map(m => <MoviesTvPosterCard key={m.id} meta={m} />)}
                    </MoviesTvRow>
                    <MoviesTvRow title="Popular series" isLoading={popularSeries.isLoading} empty={!popularSeries.data?.length}>
                        {popularSeries.data?.map(m => <MoviesTvPosterCard key={m.id} meta={m} />)}
                    </MoviesTvRow>
                    <MoviesTvRow title="Top rated movies" isLoading={ratedMovies.isLoading} empty={!ratedMovies.data?.length}>
                        {ratedMovies.data?.map(m => <MoviesTvPosterCard key={m.id} meta={m} />)}
                    </MoviesTvRow>
                    <MoviesTvRow title="Top rated series" isLoading={ratedSeries.isLoading} empty={!ratedSeries.data?.length}>
                        {ratedSeries.data?.map(m => <MoviesTvPosterCard key={m.id} meta={m} />)}
                    </MoviesTvRow>
                </>
            )}
        </PageWrapper>
    )
}
