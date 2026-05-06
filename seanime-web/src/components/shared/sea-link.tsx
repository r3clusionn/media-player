import { cn } from "@/components/ui/core/styling"
import { preloadMediaEntry } from "@/lib/entry-preloader"
import { __navigationPreloadingDisabledAtom } from "@/lib/navigation-preload-settings"
import { Link } from "@tanstack/react-router"
import { useAtomValue } from "jotai/react"
import React from "react"

type SeaLinkProps = React.ComponentPropsWithRef<"a"> & { href: string | undefined, resetScroll?: boolean }

export const SeaLink = React.forwardRef<HTMLAnchorElement, SeaLinkProps>((props, ref) => {
    const {
        href,
        children,
        className,
        onClick,
        onFocus,
        onMouseDown,
        onMouseEnter,
        onMouseLeave,
        onTouchStart,
        resetScroll = true,
        ...rest
    } = props

    // const navigate = useNavigate()

    const isExternal = href?.startsWith("http") || href?.startsWith("mailto")
    const disableNavigationPreloading = useAtomValue(__navigationPreloadingDisabledAtom)

    const hoverPreloadTimer = React.useRef<number | undefined>(undefined)

    const warmEntry = React.useCallback(() => {
        if (disableNavigationPreloading) return
        preloadMediaEntry(href)
    }, [disableNavigationPreloading, href])

    const clearHoverPreload = React.useCallback(() => {
        if (!hoverPreloadTimer.current) return
        window.clearTimeout(hoverPreloadTimer.current)
        hoverPreloadTimer.current = undefined
    }, [])

    React.useEffect(() => clearHoverPreload, [clearHoverPreload])

    const handleMouseEnter = React.useCallback((event: React.MouseEvent<HTMLAnchorElement>) => {
        clearHoverPreload()
        hoverPreloadTimer.current = window.setTimeout(() => {
            hoverPreloadTimer.current = undefined
            warmEntry()
        }, 350)
        onMouseEnter?.(event)
    }, [clearHoverPreload, warmEntry, onMouseEnter])

    const handleMouseLeave = React.useCallback((event: React.MouseEvent<HTMLAnchorElement>) => {
        clearHoverPreload()
        onMouseLeave?.(event)
    }, [clearHoverPreload, onMouseLeave])

    const handleFocus = React.useCallback((event: React.FocusEvent<HTMLAnchorElement>) => {
        warmEntry()
        onFocus?.(event)
    }, [warmEntry, onFocus])

    const handleTouchStart = React.useCallback((event: React.TouchEvent<HTMLAnchorElement>) => {
        warmEntry()
        onTouchStart?.(event)
    }, [warmEntry, onTouchStart])

    const handleMouseDown = React.useCallback((event: React.MouseEvent<HTMLAnchorElement>) => {
        clearHoverPreload()
        warmEntry()
        onMouseDown?.(event)
    }, [clearHoverPreload, warmEntry, onMouseDown])

    if (!href || isExternal) {
        return (
            <a
                ref={ref}
                href={href}
                className={cn("cursor-pointer", className)}
                onClick={onClick}
                onFocus={onFocus}
                onMouseDown={onMouseDown}
                onMouseEnter={onMouseEnter}
                onMouseLeave={onMouseLeave}
                onTouchStart={onTouchStart}
                {...rest}
            >
                {children}
            </a>
        )
    }

    const [pathname, searchString] = href.split("?")
    const searchParams: Record<string, any> = {}

    if (searchString) {
        const urlSearchParams = new URLSearchParams(searchString)
        urlSearchParams.forEach((value, key) => {
            const numValue = Number(value)
            const isNumeric = !isNaN(numValue) && value.trim() !== ""
            searchParams[key] = isNumeric ? numValue : value
        })
    }

    return (
        <Link
            to={pathname}
            search={Object.keys(searchParams).length > 0 ? () => searchParams : undefined}
            preload={disableNavigationPreloading ? false : "intent"}
            className={cn("cursor-pointer", className)}
            resetScroll={resetScroll}
            onClick={onClick}
            onFocus={handleFocus}
            onMouseDown={handleMouseDown}
            onMouseEnter={handleMouseEnter}
            onMouseLeave={handleMouseLeave}
            onTouchStart={handleTouchStart}
            {...rest}
        >
            {children}
        </Link>
    )

    // return (
    //     <a
    //         ref={ref}
    //         href={href}
    //         className={cn("cursor-pointer", className)}
    //         {...rest}
    //         onClick={(e) => {
    //             if (e.metaKey || e.altKey || e.ctrlKey || e.shiftKey || e.button !== 0) {
    //                 if (onClick) onClick(e)
    //                 return
    //             }
    //
    //             e.preventDefault()
    //
    //             if (onClick) onClick(e)
    //
    //             navigate({
    //                 to: pathname,
    //                 search: () => searchParams,
    //                 replace: false,
    //             }).then(() => {
    //                 if (resetScroll) window.scrollTo(0, 0)
    //             })
    //         }}
    //     >
    //         {children}
    //     </a>
    // )
})
