import { theme as antdTheme } from 'antd'
import type { ThemeConfig } from 'antd'

export interface ConsoleTheme {
  version: 1
  color_scheme: 'light' | 'dark' | 'system'
  accent_mode: 'preset' | 'solid' | 'gradient'
  accent_preset: 'yaoguang' | 'blue' | 'purple' | 'teal' | 'rose' | 'indigo' | 'nebula' | 'dusk'
  primary_color: string
  gradient_colors: string[]
  gradient_type: 'linear' | 'radial'
  gradient_angle: number
  sidebar_style: 'light' | 'dark'
  content_width: 'fluid' | 'fixed'
  density: 'comfortable' | 'compact'
  title_style: 'accent' | 'plain' | 'compact'
}

export const ACCENT_PRESETS = {
  yaoguang: ['#7763F1'], blue: ['#2639E9'], purple: ['#873FF2'], teal: ['#087F8C'],
  rose: ['#C52B6B'], indigo: ['#5B3DF5'], nebula: ['#536DFE', '#873FF2'], dusk: ['#19103B', '#873FF2']
} as const
export const DEFAULT_CONSOLE_THEME: Readonly<ConsoleTheme> = {
  version: 1, color_scheme: 'light', accent_mode: 'preset', accent_preset: 'yaoguang',
  primary_color: '#7763F1', gradient_colors: ['#2639E9', '#873FF2'], gradient_type: 'linear', gradient_angle: 135,
  sidebar_style: 'light', content_width: 'fluid', density: 'comfortable', title_style: 'accent'
}
export const isThemeColor = (value: unknown): value is string => typeof value === 'string' && /^#[\da-f]{6}$/i.test(value)

export function normalizeConsoleTheme(input?: unknown): ConsoleTheme {
  const defaults = () => ({ ...DEFAULT_CONSOLE_THEME, gradient_colors: [...DEFAULT_CONSOLE_THEME.gradient_colors] })
  if (!input || typeof input !== 'object' || Array.isArray(input)) return defaults()
  const candidate = { ...defaults(), ...input } as ConsoleTheme
  if (candidate.version !== 1 || !['light','dark','system'].includes(candidate.color_scheme) ||
    !['preset','solid','gradient'].includes(candidate.accent_mode) || !Object.prototype.hasOwnProperty.call(ACCENT_PRESETS, candidate.accent_preset) ||
    !isThemeColor(candidate.primary_color) || !Array.isArray(candidate.gradient_colors) ||
    candidate.gradient_colors.length < 2 || candidate.gradient_colors.length > 3 || !candidate.gradient_colors.every(isThemeColor) ||
    !['linear','radial'].includes(candidate.gradient_type) || !Number.isInteger(candidate.gradient_angle) || candidate.gradient_angle < 0 || candidate.gradient_angle > 360 ||
    !['light','dark'].includes(candidate.sidebar_style) || !['fluid','fixed'].includes(candidate.content_width) ||
    !['comfortable','compact'].includes(candidate.density) || !['accent','plain','compact'].includes(candidate.title_style)) return defaults()
  return Object.fromEntries(Object.keys(DEFAULT_CONSOLE_THEME).map(key => [key,
    key === 'gradient_colors' ? candidate.gradient_colors.map(color => color.toUpperCase()) : key === 'primary_color' ? candidate.primary_color.toUpperCase() : candidate[key as keyof ConsoleTheme]
  ])) as unknown as ConsoleTheme
}

export function colorContrast(first: string, second: string) {
  const luminance = (hex: string) => [1,3,5].map(index => parseInt(hex.slice(index,index + 2),16) / 255)
    .map(value => value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4)
    .reduce((sum,value,index) => sum + value * [0.2126,0.7152,0.0722][index],0)
  const [a,b] = [luminance(first),luminance(second)]
  return (Math.max(a,b) + 0.05) / (Math.min(a,b) + 0.05)
}
export function mixColor(first: string, second: string, ratio: number) {
  return '#' + [1,3,5].map(index => Math.round(parseInt(first.slice(index,index + 2),16) * (1-ratio) + parseInt(second.slice(index,index + 2),16) * ratio).toString(16).padStart(2,'0')).join('')
}
export function consoleAccent(value: ConsoleTheme) {
  const colors = value.accent_mode === 'preset' ? [...ACCENT_PRESETS[value.accent_preset]] : value.accent_mode === 'solid' ? [value.primary_color] : value.gradient_colors
  const foreground = ['#FFFFFF','#111827'].sort((a,b) => Math.min(...colors.map(color => colorContrast(b,color))) - Math.min(...colors.map(color => colorContrast(a,color))))[0]
  const stops = colors.map((color,index) => `${color} ${index / (colors.length - 1) * 100}%`).join(', ')
  const background = colors.length === 1 ? colors[0] : value.gradient_type === 'radial' ? `radial-gradient(circle at center, ${stops})` : `linear-gradient(${value.gradient_angle}deg, ${stops})`
  // A gradient may span both white and black; use a solid control fill then,
  // while keeping the user's gradient on decorative accents and previews.
  const controlBackground = colors.every(color => colorContrast(foreground,color) >= 4.5) ? background : colors[0]
  const controlForeground = colorContrast('#FFFFFF',colors[0]) >= colorContrast('#111827',colors[0]) ? '#FFFFFF' : '#111827'
  return { primary: colors[0], background, foreground, controlBackground, controlForeground: controlBackground === background ? foreground : controlForeground }
}
export function consolePalette(value: ConsoleTheme, dark: boolean) {
  const accent = consoleAccent(value)
  const page = dark ? '#101828' : '#F5F7FB'
  const surface = dark ? '#182235' : '#FFFFFF'
  let link = accent.primary
  for (let i = 0; colorContrast(link,surface) < 4.5 && i < 20; i++) link = mixColor(link, dark ? '#FFFFFF' : '#111827', 0.12)
  return {
    ...accent, link, page, surface, elevated: dark ? '#202D44' : '#FFFFFF', muted: dark ? '#233149' : '#F1F4F9',
    border: dark ? '#3A4961' : '#DCE3ED', text: dark ? '#E9EEF7' : '#172033', secondary: dark ? '#B6C2D5' : '#56647A',
    sidebar: value.sidebar_style === 'dark' ? '#151D32' : surface,
    sidebarText: value.sidebar_style === 'dark' ? '#DFE6F3' : (dark ? '#E9EEF7' : '#172033'),
    selected: mixColor(accent.primary,surface,dark ? 0.76 : 0.9)
  }
}

export function antdConsoleTheme(value: ConsoleTheme, dark: boolean): ThemeConfig {
  const p = consolePalette(value,dark)
  return {
    algorithm: [dark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm, ...(value.density === 'compact' ? [antdTheme.compactAlgorithm] : [])],
    token: { colorPrimary: p.primary, colorLink: p.link, fontFamily: 'var(--console-font-family)', colorBgLayout: p.page, colorBgContainer: p.surface, colorBgElevated: p.elevated, colorText: p.text, colorTextSecondary: p.secondary, colorTextHeading: p.text, colorBorder: p.border, colorBorderSecondary: p.border, borderRadius: 6 },
    components: {
      Layout: { bodyBg: p.page, headerBg: p.surface, siderBg: p.sidebar, lightSiderBg: p.sidebar },
      Card: { headerFontSize: 15, borderRadiusLG: 8 },
      Table: { headerBg: p.muted, rowHoverBg: p.selected, headerSortActiveBg: p.selected, bodySortBg: p.muted, headerColor: p.text, cellFontSize: 13, cellFontSizeMD: 13, cellFontSizeSM: 12 },
      Menu: { darkItemBg: p.sidebar, darkItemSelectedBg: p.primary, itemSelectedBg: p.selected },
      Typography: { titleMarginTop: 0 }
    }
  }
}

export function applyConsoleTheme(root: HTMLElement, value: ConsoleTheme, dark: boolean) {
  const p = consolePalette(value,dark)
  root.dataset.consoleTheme = dark ? 'dark' : 'light'
  root.dataset.consoleSidebar = value.sidebar_style
  root.dataset.consoleDensity = value.density
  root.dataset.consoleWidth = value.content_width
  root.dataset.consoleTitle = value.title_style
  const properties: Record<string,string> = {
    '--console-primary':p.primary,'--console-link':p.link,'--console-accent':p.background,'--console-button-bg':p.controlBackground,'--console-button-text':p.controlForeground,
    '--console-bg':p.page,'--console-surface':p.surface,'--console-elevated':p.elevated,'--console-muted':p.muted,'--console-border':p.border,'--console-text':p.text,'--console-text-secondary':p.secondary,
    '--console-sidebar':p.sidebar,'--console-sidebar-text':p.sidebarText,'--console-selected':p.selected,'--color-primary':p.link,
    '--console-title-size':value.title_style === 'compact' ? '18px' : '22px', '--console-page-padding':value.density === 'compact' ? '16px' : '24px'
  }
  Object.entries(properties).forEach(([name,value]) => root.style.setProperty(name,value))
  return () => {
    Object.keys(properties).forEach(name => root.style.removeProperty(name))
    for (const name of ['consoleTheme','consoleSidebar','consoleDensity','consoleWidth','consoleTitle']) delete root.dataset[name]
  }
}
