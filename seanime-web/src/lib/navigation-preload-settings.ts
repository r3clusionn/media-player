import { atomWithStorage } from "jotai/utils"

export const __navigationPreloadingDisabledAtom = atomWithStorage("sea-ui-settings-disable-navigation-preloading",
    false,
    undefined,
    { getOnInit: true })
