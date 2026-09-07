import { describe, expect, it } from 'vitest'
import { materialTemplates, safeContentPreview, executionSections } from './executionPresentation'

describe('execution history presentation', () => {
 it('keeps each run tied to its stored configuration and material', () => {
  const original = { configuration: { filters: { product: 'A' } }, materials: { templates: [{ id: 'T1', email: { subject: 'Original' } }] } }
  const changed = { configuration: { filters: { product: 'B' } }, materials: { templates: [{ id: 'T1', email: { subject: 'Changed' } }] } }
  expect(materialTemplates(original.materials)[0].email).toEqual({ subject: 'Original' })
  expect(materialTemplates(changed.materials)[0].email).toEqual({ subject: 'Changed' })
  expect(executionSections(original.configuration).filters).toEqual({ product: 'A' })
 })
 it('does not execute captured HTML or load live remote media in history', () => {
  const preview = safeContentPreview('<img src="https://mutable.example/image"><script>alert(1)</script>')
  expect(preview).toContain("default-src 'none'")
  expect(preview).toContain("img-src data:")
 })
})
