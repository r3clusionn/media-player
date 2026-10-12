import Page from "@/app/(main)/movies-tv/page"
import { createFileRoute } from "@tanstack/react-router"

// media-player: Movies & TV
export const Route = createFileRoute("/_main/movies-tv/")({
    component: Page,
})
