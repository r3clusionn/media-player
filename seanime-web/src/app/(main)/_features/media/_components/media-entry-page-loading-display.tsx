import { TRANSPARENT_SIDEBAR_BANNER_IMG_STYLE } from "@/app/(main)/_features/custom-ui/styles"
import { cn } from "@/components/ui/core/styling"
import { Skeleton } from "@/components/ui/skeleton"
import { useThemeSettings } from "@/lib/theme/theme-hooks"
import React from "react"

export function MediaEntryPageLoadingDisplay() {
    const ts = useThemeSettings()
    const showBannerSkeleton = !ts.libraryScreenCustomBackgroundImage

    return (
        <div data-media-entry-page-loading-display className="relative min-h-[31rem] overflow-hidden">
            {showBannerSkeleton && (
                <div className="__header h-[30rem] fixed left-0 top-0 w-full">
                    <div
                        className={cn(
                            "h-[30rem] w-full flex-none object-cover object-center absolute top-0 overflow-hidden",
                            !ts.disableSidebarTransparency && TRANSPARENT_SIDEBAR_BANNER_IMG_STYLE,
                        )}
                    >
                        <div className="w-full absolute z-[1] top-0 h-[12rem] bg-gradient-to-b from-[--background] to-transparent via" />
                        <Skeleton className="h-full absolute w-full rounded-none opacity-70" />
                        <div className="w-full absolute bottom-0 h-[22rem] bg-gradient-to-t from-[--background] via-[--background]/80 via-30% to-transparent" />
                    </div>
                </div>
            )}

            <div className="relative z-[5] space-y-8 px-6 sm:px-8 pt-6">
                <div className="flex flex-col lg:flex-row gap-8">
                    <Skeleton className="mx-auto lg:m-0 aspect-[6/8] h-auto w-full max-w-[150px] sm:max-w-[200px] lg:max-w-[230px] rounded-[--radius-md]" />

                    <div className="flex-1 space-y-3 pt-1">
                        <div className="space-y-2">
                            <Skeleton className="mx-auto lg:mx-0 h-9 w-2/3 max-w-[36rem] rounded-xl" />
                            <Skeleton className="mx-auto lg:mx-0 h-5 w-1/2 max-w-[24rem] rounded-xl opacity-60" />
                        </div>

                        <div className="flex gap-3 justify-center lg:justify-start">
                            <Skeleton className="h-6 w-28 rounded-xl" />
                            <Skeleton className="h-6 w-20 rounded-xl opacity-70" />
                        </div>

                        <div className="flex gap-3 justify-center lg:justify-start">
                            <Skeleton className="h-7 w-14 rounded-xl" />
                            <Skeleton className="h-7 w-10 rounded-xl opacity-70" />
                        </div>

                    </div>
                </div>

                {/* action buttons row: icon buttons + text buttons */}
                <div className="flex gap-3 items-center flex-wrap justify-center lg:justify-start">
                    <Skeleton className="h-8 w-8 rounded-xl" />
                    <Skeleton className="h-7 w-16 rounded-xl opacity-80" />
                    <Skeleton className="h-8 w-8 rounded-xl opacity-70" />
                    <Skeleton className="h-8 w-8 rounded-xl opacity-70" />
                    {/* <Skeleton className="h-8 w-8 rounded-xl opacity-60" />
                     <Skeleton className="h-8 w-8 rounded-xl opacity-60" /> */}
                </div>

                {/* tabs: Library / Torrent / Debrid / Online */}
                <div className="flex gap-2 justify-center lg:justify-start">
                    <Skeleton className="h-9 w-28 rounded-xl" />
                    <Skeleton className="h-9 w-36 rounded-xl opacity-70" />
                    <Skeleton className="h-9 w-32 rounded-xl opacity-60" />
                </div>
            </div>
        </div>
    )
}
