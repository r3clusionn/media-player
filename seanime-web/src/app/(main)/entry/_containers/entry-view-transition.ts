export const ENTRY_VIEW_SHELL_TRANSITION = {
    layout: true,
    initial: { opacity: 0 },
    animate: { opacity: 1 },
    exit: { opacity: 0 },
    transition: {
        duration: 0.18,
        ease: "easeOut",
    },
}

export const ENTRY_VIEW_TRANSITION = {
    layout: true,
    initial: { opacity: 0, scale: 0.992 },
    animate: { opacity: 1, scale: 1 },
    exit: { opacity: 0, scale: 0.992 },
    transition: {
        duration: 0.18,
        ease: "easeOut",
    },
}