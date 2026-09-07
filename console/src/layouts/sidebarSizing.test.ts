import { describe, expect, it, vi } from 'vitest'
import {
  SIDEBAR_COMPACT_THRESHOLD,
  SIDEBAR_DEFAULT_WIDTH,
  SIDEBAR_MAX_WIDTH,
  SIDEBAR_MIN_WIDTH,
  clampSidebarWidth,
  readSidebarState,
  resizeSidebarState,
  toggleSidebarState,
  writeSidebarState
} from './sidebarSizing'

describe('sidebarSizing', () => {
  it('clamps drag widths to the supported range', () => {
    expect(clampSidebarWidth(-100)).toBe(SIDEBAR_MIN_WIDTH)
    expect(clampSidebarWidth(284.6)).toBe(285)
    expect(clampSidebarWidth(1000)).toBe(SIDEBAR_MAX_WIDTH)
    expect(clampSidebarWidth(Number.NaN)).toBe(SIDEBAR_DEFAULT_WIDTH)
  })

  it('keeps the last expanded width while the sidebar is dragged into compact mode', () => {
    const expanded = resizeSidebarState(
      { width: SIDEBAR_DEFAULT_WIDTH, lastExpandedWidth: SIDEBAR_DEFAULT_WIDTH },
      300
    )
    const compact = resizeSidebarState(expanded, SIDEBAR_COMPACT_THRESHOLD - 1)

    expect(compact).toEqual({
      width: SIDEBAR_COMPACT_THRESHOLD - 1,
      lastExpandedWidth: 300
    })
    expect(toggleSidebarState(compact)).toEqual({ width: 300, lastExpandedWidth: 300 })
  })

  it('persists and restores a clamped session state', () => {
    const storage = {
      getItem: vi.fn().mockReturnValue(JSON.stringify({ width: 999, lastExpandedWidth: 12 })),
      setItem: vi.fn()
    }

    expect(readSidebarState(storage)).toEqual({
      width: SIDEBAR_MAX_WIDTH,
      lastExpandedWidth: SIDEBAR_COMPACT_THRESHOLD
    })

    writeSidebarState(storage, { width: 286, lastExpandedWidth: 280 })
    expect(storage.setItem).toHaveBeenCalledWith(
      'yaoguang:workspace-sidebar',
      JSON.stringify({ width: 286, lastExpandedWidth: 280 })
    )
  })
})
