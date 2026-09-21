import { useRef, useState } from 'react'
import { Alert, Button, Drawer, Modal, Select, Space, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { businessApi, type BusinessDefinition } from '../../services/api/business'
import { SchemaFieldsEditor } from './SchemaFieldsEditor'
import type { BusinessSchema } from './business-schema'
import { schemaExample } from './event-reporting'

export function EventFieldsExtensionDrawer({ workspaceId, events, initialKeys, onClose, onSaved }: { workspaceId: string; events: BusinessDefinition[]; initialKeys?: string[]; onClose: () => void; onSaved: () => void }) {
  const { t } = useLingui()
  const [keys, setKeys] = useState(initialKeys ?? [])
  const [schema, setSchema] = useState<BusinessSchema>({ type: 'object', properties: {} })
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [confirmClose, setConfirmClose] = useState(false)
  const busy = useRef(false)
  const fields = Object.keys(schema.properties ?? {})
  const selected = events.filter(event => keys.includes(event.key))
  const existing = selected.filter(event => fields.some(key => Object.prototype.hasOwnProperty.call(event.schema.properties ?? {}, key)))
  const close = () => { if (!busy.current) { if (fields.length) setConfirmClose(true); else onClose() } }
  const save = async () => {
    if (busy.current || !keys.length || !fields.length) return
    busy.current = true; setSaving(true); setError('')
    try { await businessApi.extendEvents(workspaceId, keys, schema); onSaved() }
    catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
    finally { busy.current = false; setSaving(false) }
  }
  const sample = JSON.stringify({ data: schemaExample(schema, new Date().toISOString()) }, null, 2)
  return <>
    <Drawer open size={920} title={t`Add report fields to existing events`} onClose={close} maskClosable={!saving} keyboard={!saving} extra={<Button type="primary" loading={saving} disabled={!keys.length || !fields.length} onClick={() => void save()}>{t`Apply to selected events`}</Button>}>
      <Space orientation="vertical" className="w-full" size="large">
        <Alert type="info" showIcon title={t`Extend fields without changing event meaning`} description={t`Add optional fields to one or many existing events. Existing fields cannot be replaced here. Reporters must supply the actual values in data; this does not collect device fingerprints or rewrite historical events. Built-in events still require their platform producer.`} />
        {error && <Alert type="error" showIcon title={error} />}
        <Select aria-label={t`Target events`} placeholder={t`Select the events to extend`} mode="multiple" optionFilterProp="label" className="w-full" value={keys} disabled={saving} onChange={setKeys} options={events.map(event => ({ value: event.key, label: `${event.name} (${event.key})` }))} />
        <Space wrap><Button disabled={saving} onClick={() => setKeys(events.map(event => event.key))}>{t`Select all events`}</Button><Button disabled={saving} onClick={() => setKeys([])}>{t`Clear selection`}</Button><Typography.Text type="secondary">{t`${keys.length} events selected`}</Typography.Text></Space>
        {existing.length > 0 && <Alert type="warning" showIcon title={t`Some selected events already declare these fields`} description={<>{existing.map(event => event.name).join(', ')}. {t`Identical definitions can be applied again; conflicting definitions reject the whole batch without changing any event.`}</>} />}
        <Button disabled={saving || fields.includes('device_fingerprint')} onClick={() => setSchema({ ...schema, properties: { ...schema.properties, device_fingerprint: { type: 'string', title: t`Device fingerprint`, description: t`Fingerprint supplied by the reporting application` } } })}>{t`Add device fingerprint example`}</Button>
        <SchemaFieldsEditor value={schema} onChange={setSchema} optionalOnly disabled={saving} />
        <Typography.Paragraph type="secondary">{t`The example below follows the fields currently configured. Merge data into your existing report; it is not a complete event request.`}</Typography.Paragraph>
        <Typography.Paragraph copyable={{ text: sample }}>{t`Report data example`}</Typography.Paragraph>
        <pre aria-label={t`Report data example`} className="overflow-auto rounded bg-gray-50 p-4">{sample}</pre>
      </Space>
    </Drawer>
    <Modal open={confirmClose} title={t`Discard unsaved changes?`} okText={t`Discard changes`} cancelText={t`Keep editing`} onCancel={() => setConfirmClose(false)} onOk={onClose} />
  </>
}
