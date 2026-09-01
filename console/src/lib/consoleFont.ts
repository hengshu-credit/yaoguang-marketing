import type { ConsoleFontSettings } from '../services/api/workspace'

export const CONSOLE_FONT_FALLBACK =
  'system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans", "Noto Sans CJK SC", "PingFang SC", "Microsoft YaHei", Arial, sans-serif'

const UPLOADED_FONT_ALIAS = 'YaoguangWorkspaceUploadedFont'
const FAMILY_PATTERN = /^[\p{L}\p{N} ._-]+$/u

export interface ConsoleFontFace {
  load: () => Promise<ConsoleFontFace>
}

export interface ConsoleFontCollection {
  add: (font: ConsoleFontFace) => unknown
  delete: (font: ConsoleFontFace) => unknown
}

export interface ConsoleFontRuntime {
  root: HTMLElement
  fonts: ConsoleFontCollection
  createFontFace: (family: string, source: string) => ConsoleFontFace
  onError?: () => void
}

function normalizeFamily(family?: string): string {
  const normalized = family?.trim() ?? ''
  if (
    normalized.length === 0 ||
    Array.from(normalized).length > 128 ||
    !FAMILY_PATTERN.test(normalized)
  ) {
    return ''
  }
  return normalized
}

export function consoleFontStack(family?: string): string {
  const normalized = normalizeFamily(family)
  return normalized ? `"${normalized}", ${CONSOLE_FONT_FALLBACK}` : CONSOLE_FONT_FALLBACK
}

function browserRuntime(overrides: Partial<ConsoleFontRuntime>): ConsoleFontRuntime {
  return {
    root: overrides.root ?? document.documentElement,
    fonts: overrides.fonts ?? (document.fonts as unknown as ConsoleFontCollection),
    createFontFace:
      overrides.createFontFace ??
      ((family, source) => new FontFace(family, source) as unknown as ConsoleFontFace),
    onError: overrides.onError
  }
}

export function applyConsoleFont(
  settings: ConsoleFontSettings | undefined,
  overrides: Partial<ConsoleFontRuntime> = {}
): () => void {
  const runtime = browserRuntime(overrides)
  let disposed = false
  let registeredFace: ConsoleFontFace | undefined
  const setFont = (value: string) => {
    runtime.root.style.setProperty('--console-font-family', value)
  }

  setFont(CONSOLE_FONT_FALLBACK)

  if (!settings?.url) {
    setFont(consoleFontStack(settings?.family))
  } else {
    const face = runtime.createFontFace(UPLOADED_FONT_ALIAS, `url(${JSON.stringify(settings.url)})`)
    void face
      .load()
      .then((loadedFace) => {
        if (disposed) return
        runtime.fonts.add(loadedFace)
        registeredFace = loadedFace
        setFont(consoleFontStack(UPLOADED_FONT_ALIAS))
      })
      .catch(() => {
        if (!disposed) runtime.onError?.()
      })
  }

  return () => {
    disposed = true
    if (registeredFace) runtime.fonts.delete(registeredFace)
    setFont(CONSOLE_FONT_FALLBACK)
  }
}
