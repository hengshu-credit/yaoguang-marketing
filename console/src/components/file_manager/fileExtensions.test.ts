import { describe, expect, it } from 'vitest'
import GetContentType from './fileExtensions'

describe('GetContentType font files', () => {
  it('returns the browser font MIME types for WOFF files', () => {
    expect(GetContentType('font.woff')).toBe('font/woff')
    expect(GetContentType('font.woff2')).toBe('font/woff2')
  })
})
