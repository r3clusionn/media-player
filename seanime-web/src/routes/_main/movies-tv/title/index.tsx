import Page from "@/app/(main)/movies-tv/title/page"
import { createFileRoute } from "@tanstack/react-router"

// media-player: a movie or series page (search params: type, id, video)
export const Route = createFileRoute("/_main/movies-tv/title/")({
    component: Page,
})
