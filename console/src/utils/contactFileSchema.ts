import { api } from '../services/api/client'
import type { Contact } from '../services/api/contacts'

export interface ContactFileField {
  key: string
  header: string
  type: 'string' | 'number' | 'integer' | 'boolean' | 'json' | 'datetime' | 'auto'
  read_only: boolean
}

export const contactFileSchema = {
  async load(workspaceId: string): Promise<ContactFileField[]> {
    const result = await api.get<{ fields: ContactFileField[] }>(`/api/imports.fields?${new URLSearchParams({ workspace_id: workspaceId })}`)
    return result.fields
  }
}

const ATTRIBUTE_PREFIX = 'profile.attributes.'

export function matchContactFileField(fields: ContactFileField[], header: string): ContactFileField | undefined {
  if (header === 'external_user_id') return fields.find(field => field.key === 'external_id')
  const exact = fields.find(field => field.key === header || field.header === header || header.endsWith(` (${field.key})`))
  if (exact) return exact
  const lower = header.toLowerCase()
  const candidates = fields.filter(field => {
    const boundary = field.header.lastIndexOf(' (')
    return field.key.toLowerCase() === lower || field.header.toLowerCase() === lower || (boundary >= 0 && field.header.slice(0, boundary).toLowerCase() === lower)
  })
  return candidates.length === 1 ? candidates[0] : undefined
}

export function exportContactField(contact: Contact, field: ContactFileField): string {
  const attributeKey = field.key.slice(ATTRIBUTE_PREFIX.length)
  const attributes = contact.profile?.attributes
  const value = field.key.startsWith(ATTRIBUTE_PREFIX)
    ? attributes && Object.prototype.hasOwnProperty.call(attributes, attributeKey) ? attributes[attributeKey] : undefined
    : field.key === 'profile.status' ? contact.profile?.status
      : field.key === 'profile.tags' ? contact.profile?.tags
        : (contact as unknown as Record<string, unknown>)[field.key]
  if (value === null || value === undefined) return ''
  if (field.type === 'json' || field.type === 'auto' || typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

export function importContactField(contact: Partial<Contact>, field: ContactFileField, raw: unknown): void {
  if (field.read_only || raw === undefined || raw === null || String(raw).trim() === '') return
  const text = String(raw)
  let value: unknown = text
  if (field.type === 'number' || field.type === 'integer') {
    value = Number(text)
    if (!Number.isFinite(value) || (field.type === 'integer' && !Number.isInteger(value))) throw new Error(`${field.header}: invalid ${field.type}`)
  } else if (field.type === 'boolean') {
    if (!/^(true|false|1|0)$/i.test(text.trim())) throw new Error(`${field.header}: invalid boolean`)
    value = /^(true|1)$/i.test(text.trim())
  } else if (field.type === 'json' || field.type === 'auto') {
    try { value = JSON.parse(text) } catch {
      if (field.type === 'json') throw new Error(`${field.header}: invalid JSON`)
    }
  }
  if (field.key.startsWith(ATTRIBUTE_PREFIX)) {
    contact.profile ??= {}
    contact.profile.attributes ??= {}
    // Treat dotted attribute names as literal keys, never as object paths.
    Object.defineProperty(contact.profile.attributes, field.key.slice(ATTRIBUTE_PREFIX.length), { value, enumerable: true, configurable: true, writable: true })
  } else if (field.key === 'profile.status') {
    contact.profile ??= {}
    contact.profile.status = String(value)
  } else if (field.key === 'profile.tags') {
    if (!Array.isArray(value) || value.some(tag => typeof tag !== 'string')) throw new Error(`${field.header}: expected a JSON string array`)
    contact.profile ??= {}
    contact.profile.tags = value
  } else {
    ;(contact as Record<string, unknown>)[field.key] = value
  }
}
