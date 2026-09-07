import { describe, expect, it } from 'vitest'
import { archivedMediaDataUrls } from './executionMediaPreview'
describe('archived media dependency previews', () => {
 it('rewrites relative CSS font references to archived data without network access', async () => {
  const result = await archivedMediaDataUrls(new Map([
   ['https://cdn.example.org/css/main.css', new Blob(["@font-face { src:url('../font.woff2') }"], { type: 'text/css' })],
   ['https://cdn.example.org/font.woff2', new Blob(['original-font'], { type: 'font/woff2' })]
  ]))
  const css = atob(result['https://cdn.example.org/css/main.css'].split(',')[1])
  expect(css).toContain('data:font/woff2;base64,')
  expect(css).not.toContain('../font.woff2')
 })
})
