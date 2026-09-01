import { describe, expect, it, vi } from 'vitest'
import {
  CONSOLE_FONT_FALLBACK,
  applyConsoleFont,
  consoleFontStack,
  type ConsoleFontFace,
  type ConsoleFontRuntime
} from './consoleFont'

function createRuntime(load: () => Promise<ConsoleFontFace>) {
  const root = document.createElement('div')
  const face: ConsoleFontFace = { load }
  const fonts = {
    add: vi.fn(),
    delete: vi.fn(() => true)
  }
  const createFontFace = vi.fn(() => face)
  const onError = vi.fn()
  const runtime: ConsoleFontRuntime = { root, fonts, createFontFace, onError }
  return { runtime, root, face, fonts, createFontFace, onError }
}

describe('console font runtime', () => {
  it('uses the multilingual fallback when no workspace font is configured', () => {
    const { runtime, root } = createRuntime(async () => ({ load: vi.fn() }))

    const cleanup = applyConsoleFont(undefined, runtime)

    expect(root.style.getPropertyValue('--console-font-family')).toBe(CONSOLE_FONT_FALLBACK)
    cleanup()
  })

  it('quotes one valid named family before the fallback stack', () => {
    expect(consoleFontStack('  Noto Sans SC  ')).toBe(
      `"Noto Sans SC", ${CONSOLE_FONT_FALLBACK}`
    )
  })

  it('rejects a CSS expression instead of applying it as a named family', () => {
    expect(consoleFontStack('Arial", serif')).toBe(CONSOLE_FONT_FALLBACK)
  })

  it('loads and registers an uploaded font before applying its internal alias', async () => {
    let resolveLoad!: (font: ConsoleFontFace) => void
    const loaded = new Promise<ConsoleFontFace>((resolve) => {
      resolveLoad = resolve
    })
    const { runtime, root, face, fonts, createFontFace } = createRuntime(() => loaded)

    applyConsoleFont(
      { family: 'Brand Font', url: 'https://cdn.example.com/brand.woff2', file_name: 'brand.woff2' },
      runtime
    )

    expect(root.style.getPropertyValue('--console-font-family')).toBe(CONSOLE_FONT_FALLBACK)
    expect(createFontFace).toHaveBeenCalledWith(
      'YaoguangWorkspaceUploadedFont',
      'url("https://cdn.example.com/brand.woff2")'
    )

    resolveLoad(face)
    await loaded
    await Promise.resolve()

    expect(fonts.add).toHaveBeenCalledWith(face)
    expect(root.style.getPropertyValue('--console-font-family')).toBe(
      `"YaoguangWorkspaceUploadedFont", ${CONSOLE_FONT_FALLBACK}`
    )
  })

  it('cleanup deletes a registered uploaded face and restores fallback', async () => {
    const { runtime, root, face, fonts } = createRuntime(async () => face)
    const cleanup = applyConsoleFont(
      { family: 'Brand Font', url: 'https://cdn.example.com/brand.ttf', file_name: 'brand.ttf' },
      runtime
    )
    await Promise.resolve()
    await Promise.resolve()

    cleanup()

    expect(fonts.delete).toHaveBeenCalledWith(face)
    expect(root.style.getPropertyValue('--console-font-family')).toBe(CONSOLE_FONT_FALLBACK)
  })

  it('ignores an uploaded font that finishes after cleanup', async () => {
    let resolveLoad!: (font: ConsoleFontFace) => void
    const loaded = new Promise<ConsoleFontFace>((resolve) => {
      resolveLoad = resolve
    })
    const { runtime, root, face, fonts } = createRuntime(() => loaded)
    const cleanup = applyConsoleFont(
      { family: 'Old Font', url: 'https://cdn.example.com/old.otf', file_name: 'old.otf' },
      runtime
    )

    cleanup()
    resolveLoad(face)
    await loaded
    await Promise.resolve()

    expect(fonts.add).not.toHaveBeenCalled()
    expect(root.style.getPropertyValue('--console-font-family')).toBe(CONSOLE_FONT_FALLBACK)
  })

  it('keeps fallback and reports one warning when an uploaded font fails', async () => {
    const { runtime, root, onError } = createRuntime(async () => {
      throw new Error('font rejected')
    })

    applyConsoleFont(
      { family: 'Broken Font', url: 'https://cdn.example.com/broken.woff', file_name: 'broken.woff' },
      runtime
    )
    await Promise.resolve()
    await Promise.resolve()

    expect(root.style.getPropertyValue('--console-font-family')).toBe(CONSOLE_FONT_FALLBACK)
    expect(onError).toHaveBeenCalledTimes(1)
  })
})
