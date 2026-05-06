import { __isElectronDesktop__ } from "@/types/constants"
import type { ParsedLocation } from "@tanstack/react-router"

type ViewTransitionInfo = {
    fromLocation?: ParsedLocation
    toLocation: ParsedLocation
    pathChanged: boolean
    hrefChanged: boolean
    hashChanged: boolean
}

type DenshiViewTransition = false | {
    types: (info: ViewTransitionInfo) => Array<string> | false
}

export function getDenshiViewTransition(): DenshiViewTransition {
    const enabled = supportsDenshiViewTransitions()

    if (typeof document !== "undefined") {
        document.documentElement.toggleAttribute("data-denshi-view-transitions", enabled)
    }

    if (!enabled) return false

    return {
        types: getDenshiViewTransitionTypes,
    }
}

function supportsDenshiViewTransitions() {
    return __isElectronDesktop__ &&
        typeof document !== "undefined" &&
        typeof window !== "undefined" &&
        "startViewTransition" in document &&
        !window.matchMedia("(prefers-reduced-motion: reduce)").matches
}

function getDenshiViewTransitionTypes(info: ViewTransitionInfo) {
    const fromLocation = info.fromLocation
    const fromPath = fromLocation?.pathname || "/"
    const toPath = info.toLocation.pathname

    if (!info.hrefChanged) return false

    if (info.hashChanged && !info.pathChanged && fromLocation?.searchStr === info.toLocation.searchStr) {
        return false
    }

    if (isFixedHeavyPath(fromPath) || isFixedHeavyPath(toPath)) return false

    return ["sea-denshi-route"]
}

function isFixedHeavyPath(pathname: string) {
    return pathname.startsWith("/mediastream") ||
        pathname.startsWith("/webview") ||
        pathname.startsWith("/splashscreen")
}
