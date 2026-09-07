export interface BusinessSchema {
  type?: string | string[]
  title?: string
  description?: string
  properties?: Record<string, BusinessSchema>
  required?: string[]
  items?: BusinessSchema
  enum?: unknown[]
  additionalProperties?: boolean | BusinessSchema
  minimum?: number
  maximum?: number
  minLength?: number
  maxLength?: number
  [key: string]: unknown
}

export function setSchemaField(schema: BusinessSchema, key: string, field: BusinessSchema): BusinessSchema {
  return { ...schema, type: 'object', properties: Object.fromEntries([...Object.entries(schema.properties ?? {}).filter(([name]) => name !== key), [key, field]]) }
}

export function setSchemaFieldRequired(schema: BusinessSchema, key: string, required: boolean): BusinessSchema {
  const names = new Set(schema.required ?? [])
  if (required) names.add(key)
  else names.delete(key)
  return { ...schema, required: [...names] }
}

export function removeSchemaField(schema: BusinessSchema, key: string): BusinessSchema {
  return { ...schema, properties: Object.fromEntries(Object.entries(schema.properties ?? {}).filter(([name]) => name !== key)), required: (schema.required ?? []).filter((name) => name !== key) }
}

export function renameSchemaField(schema: BusinessSchema, previous: string, next: string): BusinessSchema {
  if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(next) || ['__proto__', 'constructor', 'prototype'].includes(next)) throw new Error('invalid_field_key')
  if (previous === next) return schema
  if (Object.prototype.hasOwnProperty.call(schema.properties ?? {}, next)) throw new Error('duplicate_field_key')
  return { ...schema, properties: Object.fromEntries(Object.entries(schema.properties ?? {}).map(([name, value]) => [name === previous ? next : name, value])), required: (schema.required ?? []).map((name) => name === previous ? next : name) }
}
