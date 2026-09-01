import { afterEach, describe, expect, it, vi } from 'vitest'
import { i18n } from '../../i18n'
import { segmentControlLabel } from './labels'

describe('segment labels during application bootstrap', () => {
  afterEach(() => i18n.activate('en'))

  it('falls back to source text before a locale is activated', () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const consoleWarn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    i18n.activate('')
    consoleError.mockRestore()
    consoleWarn.mockRestore()
    expect(() => segmentControlLabel('days')).not.toThrow()
    expect(segmentControlLabel('days')).toBe('days')
  })
})
