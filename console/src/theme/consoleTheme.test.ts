import { describe, expect, it } from 'vitest'
import { ACCENT_PRESETS, antdConsoleTheme, applyConsoleTheme, colorContrast, consoleAccent, consolePalette, DEFAULT_CONSOLE_THEME, normalizeConsoleTheme } from './consoleTheme'

describe('console theme contract', () => {
  it.each([false, true])('keeps menu selections readable for preset and custom accents (dark=%s)', dark => {
    for (const color of [...Object.values(ACCENT_PRESETS).map(colors => colors[0]), '#FFFFFF', '#000000', '#FFFF00']) {
      const value = normalizeConsoleTheme({ accent_mode: 'solid', primary_color: color })
      const palette = consolePalette(value, dark)
      const menu = antdConsoleTheme(value, dark).components?.Menu
      expect(menu?.itemSelectedColor).toBe(palette.link)
      expect(menu?.itemBg).toBe(palette.surface)
      expect(colorContrast(palette.link, palette.selected)).toBeGreaterThanOrEqual(4.5)
      expect(colorContrast(palette.link, palette.surface)).toBeGreaterThanOrEqual(4.5)
    }
  })
  it('rejects unsafe colors and copies mutable arrays instead of mutating defaults', () => {
    expect(normalizeConsoleTheme({primary_color:'url(javascript:bad)'})).toEqual(DEFAULT_CONSOLE_THEME)
    expect(normalizeConsoleTheme({version:99})).toEqual(DEFAULT_CONSOLE_THEME)
    const value=normalizeConsoleTheme({primary_color:'#aabbcc',unused:'ignored'})
    expect(value.primary_color).toBe('#AABBCC');expect(value).not.toHaveProperty('unused')
    value.gradient_colors.push('#000000');expect(DEFAULT_CONSOLE_THEME.gradient_colors).toHaveLength(2)
  })
  it('preserves explicit zero gradient angles and uses readable text on pale buttons', () => {
    const value=normalizeConsoleTheme({accent_mode:'solid',primary_color:'#FFFFFF',gradient_angle:0})
    expect(value.gradient_angle).toBe(0)
    const accent=consoleAccent(value)
    expect(colorContrast(accent.controlForeground,accent.primary)).toBeGreaterThanOrEqual(4.5)
    const gradient=consoleAccent(normalizeConsoleTheme({accent_mode:'gradient',gradient_colors:['#FFFFFF','#000000']}))
    expect(gradient.controlBackground).toBe('#FFFFFF')
    expect(gradient.controlForeground).toBe('#111827')
  })
  it('applies and removes only console-owned tokens', () => {
    const root=document.createElement('div');root.style.setProperty('--console-font-family','ExistingFont')
    const cleanup=applyConsoleTheme(root,normalizeConsoleTheme({color_scheme:'dark',title_style:'compact'}),true)
    expect(root.dataset.consoleTheme).toBe('dark');expect(root.style.getPropertyValue('--console-title-size')).toBe('18px')
    cleanup();expect(root.dataset.consoleTheme).toBeUndefined();expect(root.style.getPropertyValue('--console-font-family')).toBe('ExistingFont')
  })
})
