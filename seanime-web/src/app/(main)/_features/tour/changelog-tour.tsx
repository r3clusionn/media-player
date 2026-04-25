import { useSeaCommand } from "@/app/(main)/_features/sea-command/sea-command.tsx"
import { SeaImage } from "@/components/shared/sea-image"
import { useAtom } from "jotai"
import { atomWithStorage } from "jotai/utils"
import React from "react"
import { useWindowSize } from "react-use"
import { useServerStatus } from "../../_hooks/use-server-status"
import { __settings_tabAtom } from "../../settings/_components/settings-page.atoms"
import { __scanner_modalIsOpen } from "../anime-library/_containers/scanner-modal"
import { tourHelpers, useTour } from "./tour"
import { TourStep } from "./tour"

export const seenChangelogAtom = atomWithStorage<string | null>("sea-seen-changelog", null, undefined, { getOnInit: true })

function useSetupTour(): Record<string, () => TourStep[]> {
    const serverStatus = useServerStatus()
    const [, openScannerModal] = useAtom(__scanner_modalIsOpen)
    const [settingsTab, setSettingsTab] = useAtom(__settings_tabAtom)
    const { setSeaCommandOpen, setSeaCommandInput } = useSeaCommand()

    const get3_5_0 = (): TourStep[] => {
        return [
            {
                id: "changelog-1",
                content: (
                    <div>
                        <h4 className="text-xl font-bold text-white">What's New in 3.5.0?</h4>
                        <p>Let's take a look at some of the new features.</p>
                    </div>
                ),
                route: "/",
                nextLabel: "Start",
                ignoreOutsideClick: true,
            },
            {
                id: "scanner",
                target: "[data-home-toolbar-scan-button]",
                title: "New Scanner",
                content: "The scanner's internal logic has been completely overhauled. It now uses a more context-aware algorithm which is more accurate.",
                route: "/",
                advanceOnTargetClick: true,
                ignoreOutsideClick: true,
                condition: () => !!serverStatus?.settings?.library?.libraryPath?.length,
                conditionFailBehavior: "modal",
            },
            {
                id: "scanner-2",
                target: "[data-scanner-modal-content]",
                title: "New Scanner",
                content: "The scanner now supports Anime Offline Database for matching data.",
                route: "/",
                prepare: () => {
                    openScannerModal(true)
                },
                advanceOnTargetClick: true,
                ignoreOutsideClick: true,
                condition: () => !!serverStatus?.settings?.library?.libraryPath?.length,
                conditionFailBehavior: "skip",
            },
            {
                id: "scanner-3",
                target: "[data-settings-anime-library='advanced-accordion-trigger']",
                title: "Scanner Configuration",
                content: "You can now fine-tune the scanner's matching behavior. Check out the documentation for more information.",
                route: "/settings",
                prepare: async () => {
                    setSettingsTab("library")
                    await tourHelpers.waitForSelector("[data-settings-anime-library='advanced-accordion-trigger']")
                    await tourHelpers.click("[data-settings-anime-library='advanced-accordion-trigger']", 200)
                },
                advanceOnTargetClick: false,
                ignoreOutsideClick: true,
            },
            {
                id: "issue-recorder",
                target: "[data-open-issue-recorder-button]",
                title: "Issue Recorder",
                // content: "The issue recorder has been improved and will now record the UI.",
                content: <div>
                    <SeaImage
                        src="https://github.com/5rahim/hibike/blob/main/changelog/3_5-issue-recorder.gif?raw=true"
                        alt="Issue Recorder"
                        width="100%"
                        height="auto"
                        className="rounded-md"
                        allowGif
                    />
                    <p className="mt-2">The issue recorder has improved and can now record the UI, making bug reports more insightful.</p>
                </div>,
                route: "/settings",
                prepare: async () => {
                    setSettingsTab("seanime")
                },
                advanceOnTargetClick: false,
                ignoreOutsideClick: true,
                popoverWidth: 500,
            },
            {
                id: "transcode-new-player",
                target: "[data-tab-trigger='mediastream']",
                title: "Transcode Player",
                content: "Transcoding/Direct Play now uses the custom Seanime player used by Seanime Denshi and Online Streaming.",
                route: "/settings",
                prepare: async () => {
                    setSettingsTab("mediastream")
                },
                advanceOnTargetClick: false,
                ignoreOutsideClick: true,
            },
            {
                id: "search",
                target: "[data-vertical-menu-item='Search']",
                title: "Search",
                content: "The search menu item now opens the search page. You can still quickly search from any page by pressing 'S'.",
                route: "/search",
                advanceOnTargetClick: false,
                ignoreOutsideClick: true,
            },
            {
                id: "entry",
                title: "New Player Features",
                content: <div>
                    <SeaImage
                        src="https://github.com/5rahim/hibike/blob/main/changelog/3_5-videocore-characters.png?raw=true"
                        alt="Character Lookup"
                        width="100%"
                        height="auto"
                        className="rounded-md"
                    />
                    <p className="mt-2">Press 'H' to quickly look up characters while watching. Press 'Z' to toggle Stats for Nerds.</p>
                </div>,
                route: "/",
                advanceOnTargetClick: false,
                ignoreOutsideClick: false,
                popoverWidth: 500,
            },
        ]
    }

    const get3_7_0 = (): TourStep[] => {
        return [
            {
                id: "changelog-1",
                content: (
                    <div>
                        <h4 className="text-xl font-bold text-white">What's New in 3.7.0?</h4>
                        <p>Let's take a look at some of the new features.</p>
                    </div>
                ),
                route: "/",
                nextLabel: "Start",
                ignoreOutsideClick: true,
            },
            {
                id: "security",
                title: "Security Improvements",
                content: "3.7.0 includes several security improvements, including secure modes. Check out the documentation for more information.",
                route: "/",
                advanceOnTargetClick: true,
                ignoreOutsideClick: true,
            },
            {
                id: "search",
                target: "[data-advanced-search-options-tags='true']",
                title: "Tags",
                content: "The search page now supports searching by tags.",
                route: "/search",
                advanceOnTargetClick: false,
                ignoreOutsideClick: true,
            },
            {
                id: "search",
                target: ".sea-command-content",
                title: "Adult Entries in Global Search",
                content: "Global search no longer filters out adult entries if you have adult content enabled. (Reminder: Press 's' to open global search)",
                route: "/search",
                advanceOnTargetClick: false,
                ignoreOutsideClick: true,
                prepare: async () => {
                    setSeaCommandOpen(true)
                    setTimeout(() => {
                        setSeaCommandInput("/search ")
                    }, 200)
                    // wait 500ms
                    return new Promise(resolve => setTimeout(resolve, 500))
                },
            },
            {
                id: "changelog-2",
                title: "Bug Fixes",
                content: "Several bugs have been fixed in this release, including some related to Seanime Denshi and plugins. Read the full changelog for more details.",
                route: "/",
                ignoreOutsideClick: true,
            },
        ]
    }

    return {
        "3.5.0": get3_5_0,
        "3.7.0": get3_7_0,
    }
}

export function useChangelogTourListener() {
    const serverStatus = useServerStatus()
    const [seenChangelog, setSeenChangelog] = useAtom(seenChangelogAtom)
    const { start } = useTour()
    const tours = useSetupTour()
    const { width } = useWindowSize()
    const isMobile = width < 768

    const toursRef = React.useRef(tours)
    toursRef.current = tours

    const started = React.useRef(false)
    const timeout = React.useRef<NodeJS.Timeout | null>(null)

    React.useEffect(() => {
        if (!serverStatus?.showChangelogTour) return
        if (serverStatus.isOffline) return
        if (isMobile) return
        if (started.current) return

        if (seenChangelog === serverStatus.showChangelogTour) return

        started.current = true

        const tourId = serverStatus.showChangelogTour

        if (timeout.current) clearTimeout(timeout.current)
        timeout.current = setTimeout(() => {
            const getSteps = toursRef.current[tourId]
            if (getSteps) {
                start(getSteps(), tourId, () => {
                    console.log("tour completed")
                    setSeenChangelog(tourId)
                })
            }
        }, 1000)

        return () => {
            if (timeout.current) clearTimeout(timeout.current)
        }
    }, [serverStatus?.showChangelogTour, serverStatus?.isOffline, seenChangelog, start, setSeenChangelog, isMobile])

    return null
}
