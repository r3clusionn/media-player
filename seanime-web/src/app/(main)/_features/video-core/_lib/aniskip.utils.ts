type AniSkipEntryLike = {
    interval: {
        startTime: number
        endTime: number
    }
}

const isValidAniSkipTime = (value: number) => Number.isFinite(value) && value >= 0

export function normalizeAniSkipEntry<T extends AniSkipEntryLike>(entry: T | null | undefined): T | null {
    if (!entry) return null

    const { startTime, endTime } = entry.interval
    if (!isValidAniSkipTime(startTime) || !isValidAniSkipTime(endTime) || endTime <= startTime) {
        return null
    }

    return entry
}

export function normalizeAniSkipData<T extends AniSkipEntryLike>(skipData: {
    op: T | null;
    ed: T | null
} | undefined): {
    op: T | null;
    ed: T | null
} {
    const op = normalizeAniSkipEntry(skipData?.op)
    let ed = normalizeAniSkipEntry(skipData?.ed)

    if (op && ed && ed.interval.startTime <= op.interval.endTime) {
        ed = null
    }

    return { op, ed }
}