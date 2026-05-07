import { atomWithStorage } from "jotai/utils"

export type NavigationPreloadMode = "disable" | "default" | "faster" | "viewport"

const NAVIGATION_PRELOAD_MODE_KEY = "sea-ui-settings-navigation-preload-mode"
const DEFAULT_NAVIGATION_PRELOAD_MODE: NavigationPreloadMode = "default"

function normalizeNavigationPreloadMode(value: unknown): NavigationPreloadMode {
    switch (value) {
        case "disable":
        case "default":
        case "faster":
        case "viewport":
            return value
        default:
            return "default"
    }
}

export function isNavigationPreloadingEnabled(mode: NavigationPreloadMode) {
    return mode !== "disable"
}

export function getNavigationRoutePreload(mode: NavigationPreloadMode): false | "intent" | "viewport" {
    switch (mode) {
        case "disable":
            return false
        case "viewport":
            return "viewport"
        default:
            return "intent"
    }
}

export function getNavigationPreloadDelay(mode: NavigationPreloadMode) {
    return mode === "faster" ? 0 : undefined
}

export function getNavigationWarmDelay(mode: NavigationPreloadMode) {
    return mode === "faster" ? 100 : 350
}

export function shouldWarmEntryOnIntent(mode: NavigationPreloadMode) {
    return mode === "default" || mode === "faster"
}

export function shouldWarmEntryOnViewport(mode: NavigationPreloadMode) {
    return mode === "viewport"
}

export const __navigationPreloadModeAtom = atomWithStorage<NavigationPreloadMode>(
    NAVIGATION_PRELOAD_MODE_KEY,
    DEFAULT_NAVIGATION_PRELOAD_MODE,
    undefined,
    { getOnInit: true },
)
