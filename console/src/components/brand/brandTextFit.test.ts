import { describe, expect, it } from 'vitest'
import { fitTextToWidth } from './brandTextFit'

describe('fitTextToWidth', () => {
  it('uses the exact negative spacing needed to keep a translated line on one row', () => {
    const result = fitTextToWidth({
      availableWidth: 168,
      naturalWidth: 174,
      characterCount: 25,
      fontSize: 13
    })

    expect(result.wrap).toBe(false)
    expect(result.letterSpacingPx).toBeCloseTo(-0.25)
  })

  it('uses the exact positive spacing needed to fill the available width', () => {
    const result = fitTextToWidth({
      availableWidth: 168,
      naturalWidth: 160,
      characterCount: 21,
      fontSize: 13
    })

    expect(result.wrap).toBe(false)
    expect(result.letterSpacingPx).toBeCloseTo(0.4)
  })

  it('does not cap positive spacing when a short translation needs to span the line', () => {
    const result = fitTextToWidth({
      availableWidth: 220,
      naturalWidth: 100,
      characterCount: 6,
      fontSize: 13
    })

    expect(result.wrap).toBe(false)
    expect(result.letterSpacingPx).toBe(24)
  })

  it('wraps instead of crushing text beyond the readable spacing limit', () => {
    const result = fitTextToWidth({
      availableWidth: 100,
      naturalWidth: 200,
      characterCount: 21,
      fontSize: 13
    })

    expect(result.wrap).toBe(true)
    expect(result.letterSpacingPx).toBeCloseTo(-0.78)
  })
})
