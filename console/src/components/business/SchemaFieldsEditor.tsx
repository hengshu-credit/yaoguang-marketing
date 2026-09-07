import { useEffect, useState } from 'react'
import { Alert, Button, Card, Checkbox, Collapse, Empty, Input, InputNumber, Select, Space, Switch } from 'antd'
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { removeSchemaField, renameSchemaField, setSchemaField, setSchemaFieldRequired, type BusinessSchema } from './business-schema'

interface Props { value: BusinessSchema; onChange: (value: BusinessSchema) => void; disabled?: boolean; depth?: number }

function FieldKey({ value, disabled, onChange }: { value: string; disabled?: boolean; onChange: (key: string) => void }) {
  const { t } = useLingui()
  const [draft, setDraft] = useState(value)
  useEffect(() => setDraft(value), [value])
  return <Input aria-label={t`Field key ${value}`} value={draft} disabled={disabled} onChange={(event) => setDraft(event.target.value)} onBlur={() => { if (draft !== value) onChange(draft.trim()) }} />
}

export function SchemaFieldsEditor({ value, onChange, disabled, depth = 0 }: Props) {
  const { t } = useLingui()
  const [error, setError] = useState('')
  const types = [{ value: 'string', label: t`Text` }, { value: 'integer', label: t`Integer` }, { value: 'number', label: t`Number` }, { value: 'boolean', label: t`Boolean` }, { value: 'object', label: t`Object` }, { value: 'array', label: t`Array` }]
  const entries = Object.entries(value.properties ?? {})
  const update = (key: string, field: BusinessSchema) => onChange(setSchemaField(value, key, field))
  const add = () => {
    let ordinal = entries.length + 1
    while (Object.prototype.hasOwnProperty.call(value.properties ?? {}, `field_${ordinal}`)) ordinal++
    onChange(setSchemaField(value, `field_${ordinal}`, { type: 'string' }))
  }
  return <div className="space-y-3">
    {error && <Alert type="error" showIcon title={error} closable onClose={() => setError('')} />}
    {!entries.length && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t`No fields configured`} />}
    {entries.map(([key, field]) => {
      const nullable = Array.isArray(field.type) && field.type.includes('null')
      const type = Array.isArray(field.type) ? field.type.find((item) => item !== 'null') ?? 'string' : field.type ?? 'string'
      const numeric = type === 'integer' || type === 'number'
      const controls = <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
        <label><span className="mb-1 block text-xs text-gray-500">{t`Description`}</span><Input value={field.description} disabled={disabled} onChange={(event) => update(key, { ...field, description: event.target.value })} /></label>
        <label><span className="mb-1 block text-xs text-gray-500">{t`Allowed values`}</span><Select mode="tags" className="w-full" value={(field.enum ?? []).map(String)} disabled={disabled || !['string', 'number', 'integer', 'boolean'].includes(type)} onChange={(values: string[]) => {
          const parsed = numeric ? values.map(Number) : type === 'boolean' ? values.map((item) => item === 'true') : values
          if (numeric && parsed.some((item) => !Number.isFinite(item))) { setError(t`Allowed values must match the field type.`); return }
          if (type === 'integer' && parsed.some((item) => !Number.isInteger(item))) { setError(t`Allowed values must be whole numbers.`); return }
          if (type === 'boolean' && values.some((item) => item !== 'true' && item !== 'false')) { setError(t`Boolean values must be true or false.`); return }
          const next = { ...field }; if (parsed.length) next.enum = parsed; else delete next.enum; update(key, next)
        }} /></label>
        {(numeric || type === 'string') && <>
          <label><span className="mb-1 block text-xs text-gray-500">{numeric ? t`Minimum value` : t`Minimum length`}</span><InputNumber className="w-full" value={numeric ? field.minimum : field.minLength} min={numeric ? undefined : 0} disabled={disabled} onChange={(number) => { const next = { ...field }; const property = numeric ? 'minimum' : 'minLength'; if (number === null) delete next[property]; else next[property] = number; update(key, next) }} /></label>
          <label><span className="mb-1 block text-xs text-gray-500">{numeric ? t`Maximum value` : t`Maximum length`}</span><InputNumber className="w-full" value={numeric ? field.maximum : field.maxLength} min={numeric ? undefined : 0} disabled={disabled} onChange={(number) => { const next = { ...field }; const property = numeric ? 'maximum' : 'maxLength'; if (number === null) delete next[property]; else next[property] = number; update(key, next) }} /></label>
        </>}
        {type === 'string' && <label><span className="mb-1 block text-xs text-gray-500">{t`Format`}</span><Select className="w-full" allowClear value={typeof field.format === 'string' ? field.format : undefined} disabled={disabled} options={[{ value: 'date-time', label: t`Date and time` }, { value: 'email', label: t`Email address` }, { value: 'uuid', label: t`UUID` }]} onChange={(format) => { const next = { ...field }; if (format) next.format = format; else delete next.format; update(key, next) }} /></label>}
      </div>
      return <Card key={key} size="small" styles={{ body: { padding: 14 } }}>
        <div className="grid grid-cols-1 gap-3 lg:grid-cols-[1fr_1fr_160px_auto]">
          <label><span className="mb-1 block text-xs text-gray-500">{t`Field key`}</span><FieldKey value={key} disabled={disabled} onChange={(next) => { try { onChange(renameSchemaField(value, key, next)); setError('') } catch { setError(t`Use a unique field key containing letters, numbers and underscores.`) } }} /></label>
          <label><span className="mb-1 block text-xs text-gray-500">{t`Display name`}</span><Input aria-label={t`Display name ${key}`} value={field.title} disabled={disabled} onChange={(event) => update(key, { ...field, title: event.target.value })} /></label>
          <label><span className="mb-1 block text-xs text-gray-500">{t`Field type`}</span><Select aria-label={t`Field type ${key}`} className="w-full" options={types} value={type} disabled={disabled} onChange={(next) => {
            const updated: BusinessSchema = { ...field, type: nullable ? [next, 'null'] : next }
            if (next === 'object' && !updated.properties) updated.properties = {}
            if (next === 'array' && !updated.items) updated.items = { type: 'string' }
            if (next !== 'object') { delete updated.properties; delete updated.required; delete updated.additionalProperties }
            if (next !== 'array') delete updated.items
            if (next !== 'string') { delete updated.minLength; delete updated.maxLength; delete updated.pattern; delete updated.format }
            if (next !== 'number' && next !== 'integer') { delete updated.minimum; delete updated.maximum }
            if (next !== type) delete updated.enum
            update(key, updated)
          }} /></label>
          <Space align="end" className="pb-1"><Switch size="small" aria-label={t`Required ${key}`} checked={(value.required ?? []).includes(key)} disabled={disabled} onChange={(required) => onChange(setSchemaFieldRequired(value, key, required))} /><span className="text-xs">{t`Required`}</span><Button type="text" danger icon={<DeleteOutlined />} aria-label={t`Remove field ${key}`} disabled={disabled} onClick={() => onChange(removeSchemaField(value, key))} /></Space>
        </div>
        <div className="mt-3"><Checkbox checked={nullable} disabled={disabled} onChange={(event) => update(key, { ...field, type: event.target.checked ? [type, 'null'] : type })}>{t`Allow an explicit null value`}</Checkbox></div>
        <Collapse ghost className="mt-2" items={[{ key: 'validation', label: t`Validation and description`, children: controls }]} />
        {type === 'object' && depth < 8 && <Collapse className="mt-3" items={[{ key: 'fields', label: t`Object fields`, children: <SchemaFieldsEditor value={field} onChange={(next) => update(key, next)} disabled={disabled} depth={depth + 1} /> }]} />}
        {type === 'array' && <div className="mt-3 space-y-3"><label><span className="mr-3 text-xs text-gray-500">{t`Array item type`}</span><Select options={types} value={typeof field.items?.type === 'string' ? field.items.type : 'string'} disabled={disabled} onChange={(next) => update(key, { ...field, items: { type: next, ...(next === 'object' ? { properties: {} } : {}) } })} /></label>{field.items?.type === 'object' && depth < 8 && <SchemaFieldsEditor value={field.items} onChange={(next) => update(key, { ...field, items: next })} disabled={disabled} depth={depth + 1} />}</div>}
      </Card>
    })}
    <div className="flex flex-wrap items-center gap-4"><Button aria-label={t`Add field`} icon={<PlusOutlined aria-hidden />} onClick={add} disabled={disabled}>{t`Add field`}</Button><Checkbox checked={value.additionalProperties !== false} disabled={disabled} onChange={(event) => onChange({ ...value, additionalProperties: event.target.checked })}>{t`Allow additional fields`}</Checkbox></div>
  </div>
}
