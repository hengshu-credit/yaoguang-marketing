import { describe, expect, it } from 'vitest'
import { removeSchemaField, renameSchemaField, setSchemaField, setSchemaFieldRequired } from './business-schema'

describe('business schema field editing', () => {
  it('renames a required field without losing nested validation or changing the original', () => {
    const original = { type: 'object', required: ['phone'], additionalProperties: false, properties: { phone: { type: 'string', minLength: 8 }, profile: { type: 'object', properties: { active: { type: 'boolean' } } } } }
    const changed = renameSchemaField(original, 'phone', 'mobile')
    expect(changed.required).toEqual(['mobile'])
    expect(changed.properties?.mobile).toEqual({ type: 'string', minLength: 8 })
    expect(changed.properties?.profile).toEqual(original.properties.profile)
    expect(changed.additionalProperties).toBe(false)
    expect(original.required).toEqual(['phone'])
    expect(original.properties.phone).toBeDefined()
  })

  it('rejects a duplicate key instead of overwriting another field', () => {
    expect(() => renameSchemaField({ properties: { email: { type: 'string' }, phone: { type: 'string' } } }, 'phone', 'email')).toThrow()
  })

  it('removes required membership when deleting a field and preserves other fields', () => {
    const result = removeSchemaField({ required: ['email', 'phone'], properties: { email: { type: 'string' }, phone: { type: 'string' } } }, 'phone')
    expect(result.required).toEqual(['email'])
    expect(result.properties).toEqual({ email: { type: 'string' } })
  })

  it('makes a field required once and can later make it optional', () => {
    const first = setSchemaFieldRequired({ properties: { email: { type: 'string' } } }, 'email', true)
    expect(setSchemaFieldRequired(first, 'email', true).required).toEqual(['email'])
    expect(setSchemaFieldRequired(first, 'email', false).required).toEqual([])
  })

  it('keeps array item validation and does not add shared mutable state', () => {
    const source = { type: 'object', properties: {} }
    const result = setSchemaField(source, 'contacts', { type: 'array', items: { type: 'object', required: ['kind'], properties: { kind: { type: 'string', enum: ['relative', 'colleague'] } } } })
    expect(result.properties?.contacts.items?.required).toEqual(['kind'])
    expect(source.properties).toEqual({})
  })
})
