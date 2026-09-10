import { useState } from 'react'
import { Alert, Button, Drawer, Select, Space, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { businessApi, type BusinessDefinition } from '../../services/api/business'
import { SchemaFieldsEditor } from './SchemaFieldsEditor'
import type { BusinessSchema } from './business-schema'

export function EventFieldsExtensionDrawer({ workspaceId, events, initialKeys, onClose, onSaved }: { workspaceId: string; events: BusinessDefinition[]; initialKeys?: string[]; onClose: () => void; onSaved: () => void }) {
 const { t } = useLingui()
 const [keys, setKeys] = useState(initialKeys ?? events.map(event => event.key))
 const [schema, setSchema] = useState<BusinessSchema>({ type: 'object', properties: {} })
 const [saving, setSaving] = useState(false), [error, setError] = useState('')
 const save = async () => { setSaving(true); setError(''); try { await businessApi.extendEvents(workspaceId, keys, schema); onSaved() } catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) } finally { setSaving(false) } }
 return <Drawer open size={920} title={t`Add report fields to existing events`} onClose={onClose} extra={<Button type="primary" loading={saving} disabled={!keys.length || !Object.keys(schema.properties ?? {}).length} onClick={() => void save()}>{t`Apply to selected events`}</Button>}>
  <Space orientation="vertical" className="w-full" size="large">
   <Alert type="info" showIcon title={t`Extend fields without changing event meaning`} description={t`Add optional fields to one or many existing events. Existing fields cannot be replaced here. Reporters must supply the actual values in data; this does not collect device fingerprints or rewrite historical events. Built-in events still require their platform producer.`} />
   {error && <Alert type="error" showIcon title={error} />}
   <Select aria-label={t`Target events`} mode="multiple" className="w-full" value={keys} onChange={setKeys} options={events.map(event => ({ value: event.key, label: `${event.name} (${event.key})` }))} />
   <Button onClick={() => setSchema({ type: 'object', properties: { ...schema.properties, device_fingerprint: { type: 'string', title: t`Device fingerprint`, description: t`Fingerprint supplied by the reporting application` } } })}>{t`Add device fingerprint example`}</Button>
   <SchemaFieldsEditor value={schema} onChange={setSchema} optionalOnly />
   <Typography.Paragraph type="secondary">{t`Example report data: add device_fingerprint to the existing data object. Keep the original event code and report envelope.`}</Typography.Paragraph>
   <pre className="rounded bg-gray-50 p-4">{'"data": {\n  "device_fingerprint": "device-value-from-your-app"\n}'}</pre>
  </Space>
 </Drawer>
}
