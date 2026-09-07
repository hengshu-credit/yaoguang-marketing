export const SIDEBAR_MIN_WIDTH = 64
export const SIDEBAR_MAX_WIDTH = 320
export const SIDEBAR_DEFAULT_WIDTH = 250
export const SIDEBAR_COMPACT_THRESHOLD = 168
export const SIDEBAR_STORAGE_KEY = 'yaoguang:workspace-sidebar'

export interface SidebarState {
  width: number
  lastExpandedWidth: number
}

interface ReadableStorage {
  getItem(key: string): string | null
}

interface WritableStorage {
  setItem(key: string, value: string): void
}

const defaultSidebarState = (): SidebarState => ({
  width: SIDEBAR_DEFAULT_WIDTH,
  lastExpandedWidth: SIDEBAR_DEFAULT_WIDTH
})

export function clampSidebarWidth(width: unknown): number {
  const value = Number(width)
  if (!Number.isFinite(value)) return SIDEBAR_DEFAULT_WIDTH
  return Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, Math.round(value)))
}

function clampExpandedWidth(width: unknown): number {
  return Math.max(SIDEBAR_COMPACT_THRESHOLD, clampSidebarWidth(width))
}

export function resizeSidebarState(state: SidebarState, width: unknown): SidebarState {
  const nextWidth = clampSidebarWidth(width)
  return {
    width: nextWidth,
    lastExpandedWidth:
      nextWidth >= SIDEBAR_COMPACT_THRESHOLD ? nextWidth : state.lastExpandedWidth
  }
}

export function toggleSidebarState(state: SidebarState): SidebarState {
  if (state.width < SIDEBAR_COMPACT_THRESHOLD) {
    return {
      width: Math.max(SIDEBAR_COMPACT_THRESHOLD, state.lastExpandedWidth),
      lastExpandedWidth: Math.max(SIDEBAR_COMPACT_THRESHOLD, state.lastExpandedWidth)
    }
  }
  return { width: SIDEBAR_MIN_WIDTH, lastExpandedWidth: state.width }
}

export function readSidebarState(storage: ReadableStorage | null): SidebarState {
  if (!storage) return defaultSidebarState()
  try {
    const raw = storage.getItem(SIDEBAR_STORAGE_KEY)
    if (!raw) return defaultSidebarState()
    const saved = JSON.parse(raw) as Partial<SidebarState> | null
    return {
      width: clampSidebarWidth(saved?.width),
      lastExpandedWidth: clampExpandedWidth(saved?.lastExpandedWidth)
    }
  } catch {
    return defaultSidebarState()
  }
}

export function writeSidebarState(storage: WritableStorage | null, state: SidebarState): void {
  if (!storage) return
  try {
    storage.setItem(
      SIDEBAR_STORAGE_KEY,
      JSON.stringify({
        width: clampSidebarWidth(state.width),
        lastExpandedWidth: clampExpandedWidth(state.lastExpandedWidth)
      })
    )
  } catch {
    // Storage can be unavailable in privacy-restricted browser contexts.
  }
}
