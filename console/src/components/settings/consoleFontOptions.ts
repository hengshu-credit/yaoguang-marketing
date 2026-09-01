import type { StorageObject } from '../file_manager/interfaces'

export const FONT_FAMILY_PRESETS = [
  '',
  'AlimamaFangYuanTiVF',
  'PingFang SC',
  'Microsoft YaHei',
  'Noto Sans',
  'Noto Sans SC',
  'Arial',
  'Helvetica'
] as const

export const SUPPORTED_FONT_EXTENSION = /\.(ttf|otf|woff|woff2)$/i

export function isSupportedConsoleFontFile(item: StorageObject): boolean {
  return !item.is_folder && SUPPORTED_FONT_EXTENSION.test(item.name)
}
