import { Status } from "@/api/generated/types"
import { atom } from "jotai"
import { atomWithStorage } from "jotai/utils"

export const serverStatusAtom = atom<Status | undefined>(undefined)

export const isLoginModalOpenAtom = atom(false)

export const serverAuthTokenAtom = atomWithStorage<string | undefined>("sea-server-auth-token", undefined, undefined, { getOnInit: true })
