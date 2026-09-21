import type { BusinessDefinition } from '../../services/api/business'
import type { BusinessSchema } from './business-schema'

export const MAX_EVENT_TTL_SECONDS = 315360000
export const DURATION_UNITS = { seconds: 1, minutes: 60, hours: 3600, days: 86400 } as const
export type DurationUnit = keyof typeof DURATION_UNITS
export interface ReportingIssue { field: string; code: 'ttl' | 'mode' | 'browser_conflict' | 'browser_object' | 'origins' | 'sources' | 'path' | 'required_mapping' | 'event_code' | 'attribute_mapping' }

export function durationUnitFor(seconds: unknown): DurationUnit {
  if (typeof seconds !== 'number' || seconds <= 0) return 'seconds'
  return (['days', 'hours', 'minutes', 'seconds'] as const).find(unit => seconds % DURATION_UNITS[unit] === 0) ?? 'seconds'
}

export function isObjectPath(path: unknown): path is string {
  if (path === '$') return true
  if (typeof path !== 'string' || !path.startsWith('$.') || path.length > 1024) return false
  const parts = path.slice(2).split('.')
  return parts.length <= 16 && parts.every(part => !!part && !/[[\]*"'\s]/.test(part) && !['__proto__', 'constructor', 'prototype'].includes(part))
}
const normalizedPath = (path: unknown) => isObjectPath(path) && (path.startsWith('$.data.') || path.startsWith('$.customer_attributes.'))

export function exactBrowserOrigin(value: string): string | undefined {
  try {
    const url = new URL(value.trim())
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.pathname !== '/' || url.search || url.hash || /[?#*]/.test(value)) return undefined
    return url.origin
  } catch { return undefined }
}

export function setOptionalMappingPath(fields: Record<string, unknown>, key: string, path: string) {
  const next = { ...fields }
  if (path.trim()) next[key] = path
  else delete next[key]
  return next
}

export function reportingIssues(definition: BusinessDefinition, complete = true, catalogue: BusinessDefinition[] = []): ReportingIssue[] {
  if (definition.status === 'archived' && !complete) return []
  const s = definition.settings
  const issues: ReportingIssue[] = []
  const add = (field: string, code: ReportingIssue['code']) => issues.push({ field, code })
  if (definition.kind === 'event') {
    const mode = s.expiry_mode ?? 'required'
    if (!['never', 'required', 'duration', ''].includes(String(mode))) add('settings.expiry_mode', 'mode')
    if (mode === 'duration' && (complete || s.ttl_seconds != null) &&
      (typeof s.ttl_seconds !== 'number' || !Number.isInteger(s.ttl_seconds) || s.ttl_seconds < 1 || s.ttl_seconds > MAX_EVENT_TTL_SECONDS)) add('settings.ttl_seconds', 'ttl')
    const subject = s.subject_type || 'customer'
    if (subject !== 'customer') {
      if ((complete || s.subject_id_path) && !normalizedPath(s.subject_id_path)) add('settings.subject_id_path', 'path')
      if (s.subject_attributes_path !== undefined && !(normalizedPath(s.subject_attributes_path) || s.subject_attributes_path === '$.data' || s.subject_attributes_path === '$.customer_attributes')) add('settings.subject_attributes_path', 'path')
    }
    if (s.browser_enabled === true) {
      if (s.authoritative === true) add('settings.browser_enabled', 'browser_conflict')
      if (subject !== 'customer') add('settings.subject_type', 'browser_object')
      const origins = s.allowed_browser_origins
      if ((complete && (!Array.isArray(origins) || !origins.length)) ||
        (origins != null && (!Array.isArray(origins) || origins.some(origin => typeof origin !== 'string' || exactBrowserOrigin(origin) !== origin)))) add('settings.allowed_browser_origins', 'origins')
    }
    if (s.allowed_sources !== undefined && (!Array.isArray(s.allowed_sources) || (complete && !s.allowed_sources.length) || s.allowed_sources.some(value => typeof value !== 'string' || !value.trim() || new TextEncoder().encode(value).length > 128 || value !== value.trim()))) add('settings.allowed_sources', 'sources')
  }
  if (definition.kind === 'event_mapping') {
    const fields = (s.fields ?? {}) as Record<string, unknown>
    for (const [key, path] of Object.entries(fields)) if (!isObjectPath(path)) add(`settings.fields.${key}`, 'path')
    if (complete) {
      for (const key of ['report_id', 'data', 'triggered_at']) if (!fields[key]) add(`settings.fields.${key}`, 'required_mapping')
      if (!s.event_code && !fields.event_code) add('settings.fields.event_code', 'event_code')
      const rules = Array.isArray(s.attribute_mappings) ? s.attribute_mappings : []
      if (!fields.customer_attributes && !rules.some(mapping => mapping.action === 'lookup' || mapping.action === 'bind')) add('settings.fields.customer_attributes', 'required_mapping')
      const event = catalogue.find(item => item.kind === 'event' && item.key === s.event_code)
      if (event && (!event.settings.expiry_mode || event.settings.expiry_mode === 'required') && !fields.expires_at) add('settings.fields.expires_at', 'required_mapping')
    }
    if (s.event_code && !/^[A-Za-z][A-Za-z0-9_.-]{0,127}$/.test(String(s.event_code))) add('settings.event_code', 'event_code')
    if (Array.isArray(s.attribute_mappings)) s.attribute_mappings.forEach((mapping, index) => {
      if (!mapping.attribute || !normalizedPath(mapping.path)) add(`settings.attribute_mappings.${index}`, 'attribute_mapping')
    })
  }
  return issues
}

// Examples are editable suggestions, not fake server validation. Prefer declared
// defaults/enums, bound recursion, and never replace the operator's pasted sample.
export function schemaExample(schema: BusinessSchema, now: string, depth = 0): unknown {
  if (depth > 16) throw new Error('schema_depth')
  if (schema.default !== undefined) return structuredClone(schema.default)
  if (schema.enum?.length) return structuredClone(schema.enum[0])
  const type = Array.isArray(schema.type) ? schema.type.find(type => type !== 'null') ?? 'null' : schema.type
  if (type === 'object' || schema.properties) return Object.fromEntries(Object.entries(schema.properties ?? {}).map(([key, field]) => [key, schemaExample(field, now, depth + 1)]))
  if (type === 'array') return [schemaExample(schema.items ?? { type: 'string' }, now, depth + 1)]
  if (type === 'boolean') return false
  if (type === 'null') return null
  if (type === 'number' || type === 'integer') {
    const minimum = schema.minimum ?? (schema.maximum !== undefined ? Math.min(1, schema.maximum) : 1)
    return type === 'integer' ? Math.ceil(minimum) : minimum
  }
  if (schema.format === 'date-time') return now
  if (schema.format === 'email') return 'customer@example.com'
  if (schema.format === 'uuid') return '00000000-0000-4000-8000-000000000001'
  return 'example'.padEnd(Math.min(schema.minLength ?? 0, 256), 'x').slice(0, schema.maxLength ?? 256)
}

function putPath(target: Record<string, unknown>, path: string, value: unknown) {
  if (!isObjectPath(path)) throw new Error('unsupported_path')
  if (path === '$') {
    if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('overlapping_paths')
    for (const [key, entry] of Object.entries(value)) putPath(target, `$.${key}`, entry)
    return
  }
  const keys = path.slice(2).split('.')
  let node = target
  for (const key of keys.slice(0, -1)) {
    if (!Object.prototype.hasOwnProperty.call(node, key)) node[key] = {}
    const child = node[key]
    if (!child || typeof child !== 'object' || Array.isArray(child)) throw new Error('overlapping_paths')
    node = child as Record<string, unknown>
  }
  const key = keys[keys.length - 1]
  if (Object.prototype.hasOwnProperty.call(node, key) && JSON.stringify(node[key]) !== JSON.stringify(value)) throw new Error('overlapping_paths')
  node[key] = structuredClone(value)
}

function getPath(target: Record<string, unknown>, path: string): unknown {
  if (!isObjectPath(path)) return undefined
  return path === '$' ? target : path.slice(2).split('.').reduce<unknown>((node, key) =>
    node && typeof node === 'object' && Object.prototype.hasOwnProperty.call(node, key) ? (node as Record<string, unknown>)[key] : undefined, target)
}

export function reportingSample(definition: BusinessDefinition, catalogue: BusinessDefinition[], now = new Date().toISOString()): Record<string, unknown> {
  if (definition.kind === 'customer_attribute') return { value: schemaExample(definition.schema, now) }
  if (definition.kind !== 'event' && definition.kind !== 'event_mapping') return schemaExample({ type: 'object', ...definition.schema }, now) as Record<string, unknown>
  const event = definition.kind === 'event' ? definition : catalogue.find(item => item.kind === 'event' && item.key === definition.settings.event_code)
  const s = event?.settings ?? { expiry_mode: 'required' }
  const normalized: Record<string, unknown> = {
    report_id: 'example-occurrence-001', event_code: event?.key || 'your.event',
    customer_attributes: { external_user_id: 'example-customer-001' },
    data: schemaExample({ type: 'object', ...event?.schema }, now), triggered_at: now
  }
  if (!s.expiry_mode || s.expiry_mode === 'required') normalized.expires_at = new Date(Date.parse(now) + 86400000).toISOString()
  if (s.subject_type && s.subject_type !== 'customer') {
    if (typeof s.subject_id_path === 'string' && getPath(normalized, s.subject_id_path) === undefined) putPath(normalized, s.subject_id_path, 'example-object-001')
    const object = catalogue.find(item => item.kind === 'object_type' && item.key === s.subject_type)
    if (typeof s.subject_attributes_path === 'string' && s.subject_attributes_path && getPath(normalized, s.subject_attributes_path) === undefined) putPath(normalized, s.subject_attributes_path, schemaExample(object?.schema ?? { type: 'object' }, now))
  }
  if (definition.kind === 'event') return normalized
  const fields = (definition.settings.fields ?? {}) as Record<string, string>
  if (Array.isArray(definition.settings.attribute_mappings)) for (const mapping of definition.settings.attribute_mappings) {
    if (getPath(normalized, mapping.path) === undefined) {
      const attribute = catalogue.find(item => item.kind === 'customer_attribute' && item.key === mapping.attribute)
      putPath(normalized, mapping.path, schemaExample(attribute?.schema ?? { type: 'string' }, now))
    }
  }
  const raw: Record<string, unknown> = {}
  for (const [field, path] of Object.entries(fields).sort((a, b) => a[1].length - b[1].length)) {
    if (field === 'event_code' && definition.settings.event_code) continue
    if (normalized[field] !== undefined) putPath(raw, path, normalized[field])
  }
  return raw
}

export function reportingRequest(workspaceId: string, definition: BusinessDefinition, event: Record<string, unknown>, source: string) {
  return { workspace_id: workspaceId, source, ...(definition.kind === 'event_mapping' ? { mapping_key: definition.key } : {}), event }
}
