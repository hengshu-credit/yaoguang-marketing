import { useState } from 'react'
import { Alert, Button, Drawer, Form, Input, Select, Space, Tabs, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { businessDefinitionDisplayName } from './builtin-event-labels'
import { businessApi, type BusinessDefinition, type BusinessPreviewResult } from '../../services/api/business'
import { SchemaFieldsEditor } from './SchemaFieldsEditor'
import { BusinessDefinitionSettings } from './BusinessDefinitionSettings'
import { BuiltinEventDetails } from './BuiltinEventDetails'

interface Props { workspaceId: string; definition: BusinessDefinition; catalogue: BusinessDefinition[]; readOnly?: boolean; onSaved: (definition: BusinessDefinition) => void; onClose: () => void }

export function BusinessDefinitionEditor({ workspaceId, definition, catalogue, readOnly, onSaved, onClose }: Props) {
  const { t, i18n } = useLingui()
  const [draft, setDraft] = useState<BusinessDefinition>(() => structuredClone(definition))
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [sample, setSample] = useState(definition.kind === 'customer_attribute' ? '{"value": ""}' : '{}')
  const [previewing, setPreviewing] = useState(false)
  const [preview, setPreview] = useState<BusinessPreviewResult>()
  const disabled = readOnly || draft.builtin || draft.settings.builtin === true
  const change = (next: BusinessDefinition) => { setDraft(next); setPreview(undefined) }
  const save = async () => {
    if (disabled) return
    if (!/^[a-zA-Z][a-zA-Z0-9_.-]{0,127}$/.test(draft.key) || !draft.name.trim()) { setError(t`Enter a name and a stable key starting with a letter.`); return }
    setSaving(true); setError('')
    try { onSaved(await businessApi.save(workspaceId, { ...draft, name: draft.name.trim() })) }
    catch (cause) { setError(cause instanceof Error ? cause.message : t`Failed to save definition`) }
    finally { setSaving(false) }
  }
  const validate = async () => {
    setError(''); setPreview(undefined)
    let data: Record<string, unknown>
    try { const parsed = JSON.parse(sample); if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') throw new Error(); data = parsed }
    catch { setError(t`Enter a valid JSON object for the sample.`); return }
    setPreviewing(true)
    try { setPreview(await businessApi.preview(workspaceId, draft, data)) }
    catch (cause) { setError(cause instanceof Error ? cause.message : t`Failed to validate sample`) }
    finally { setPreviewing(false) }
  }
  if (draft.kind === 'event' && (draft.builtin || draft.settings.builtin === true)) return <BuiltinEventDetails definition={draft} onClose={onClose} />
  return <Drawer open title={definition.id ? t`Definition details` : t`New definition`} size={920} onClose={onClose} maskClosable={!saving} extra={<Space><Button onClick={onClose}>{t`Close`}</Button>{!disabled && <Button type="primary" loading={saving} onClick={() => void save()}>{t`Save definition`}</Button>}</Space>}>
    {error && <Alert className="mb-4" type="error" showIcon title={error} />}
    {(draft.builtin || draft.settings.builtin === true) && <Alert className="mb-4" type="info" showIcon title={t`Built-in definition`} description={t`Built-in event meanings are protected. Create a custom event to define different behavior.`} />}
    <Form layout="vertical">
      <div className="grid grid-cols-1 gap-x-6 md:grid-cols-2">
        <Form.Item label={t`Definition key`} required extra={t`A stable identifier used in event reports and rules.`}><Input aria-label={t`Definition key`} value={draft.key} disabled={disabled || !!definition.id} onChange={(event) => change({ ...draft, key: event.target.value })} /></Form.Item>
        <Form.Item label={t`Definition name`} required><Input aria-label={t`Definition name`} value={businessDefinitionDisplayName(draft, i18n)} disabled={disabled} onChange={(event) => change({ ...draft, name: event.target.value })} /></Form.Item>
        <Form.Item label={t`Status`}><Select aria-label={t`Definition status`} disabled={disabled} value={draft.status} options={[{ value: 'draft', label: t`Draft` }, { value: 'active', label: t`Active` }, { value: 'archived', label: t`Archived` }]} onChange={(status) => change({ ...draft, status })} /></Form.Item>
        <Form.Item label={t`Description`}><Input.TextArea rows={2} value={draft.description} disabled={disabled} onChange={(event) => change({ ...draft, description: event.target.value })} /></Form.Item>
      </div>
    </Form>
    <Tabs defaultActiveKey={draft.kind === 'object_type' ? 'schema' : 'settings'} items={[
      { key: 'settings', label: t`Configuration`, children: <BusinessDefinitionSettings value={draft} catalogue={catalogue} onChange={change} disabled={disabled} /> },
      ...(draft.kind !== 'object_relation' && draft.kind !== 'event_mapping' ? [{ key: 'schema', label: draft.kind === 'event' ? t`Report data fields` : t`Fields and validation`, children: draft.kind === 'customer_attribute' ? <Form layout="vertical"><Form.Item label={t`Attribute value type`}><Select value={draft.schema.type} disabled={disabled} options={[{ value: 'string', label: t`Text` }, { value: 'number', label: t`Number` }, { value: 'integer', label: t`Integer` }, { value: 'boolean', label: t`Boolean` }, { value: 'array', label: t`Array` }, { value: 'object', label: t`Object` }]} onChange={(type) => change({ ...draft, schema: { type, ...(type === 'object' ? { properties: {} } : {}), ...(type === 'array' ? { items: { type: 'string' } } : {}) } })} /></Form.Item>{draft.schema.type === 'object' && <SchemaFieldsEditor value={draft.schema} disabled={disabled} onChange={(schema) => change({ ...draft, schema })} />}</Form> : <SchemaFieldsEditor value={draft.schema} disabled={disabled} onChange={(schema) => change({ ...draft, schema })} /> }] : []),
      { key: 'preview', label: t`Sample validation`, children: <div className="space-y-4">
        <Typography.Paragraph type="secondary">{t`Validate the current configuration against a sample before saving. This preview does not emit events or update customer data.`}</Typography.Paragraph>
        {draft.kind === 'customer_attribute' && <Typography.Paragraph type="secondary">{t`Put the attribute sample in the value field of the JSON object.`}</Typography.Paragraph>}
        <Input.TextArea aria-label={t`Sample JSON`} rows={10} value={sample} onChange={(event) => { setSample(event.target.value); setPreview(undefined) }} className="font-mono" />
        <Button aria-label={t`Validate sample`} loading={previewing} onClick={() => void validate()}>{t`Validate sample`}</Button>
        {preview && <Alert type={preview.valid ? 'success' : 'error'} showIcon title={preview.valid ? t`Sample is valid` : t`Sample validation failed`} description={preview.errors?.length ? <ul>{preview.errors.map((item, index) => <li key={index}><code>{item.field}</code>: {item.message}</li>)}</ul> : undefined} />}
        {preview?.normalized_event && <div><h3 className="mb-2 font-medium">{t`Normalized event`}</h3><pre className="overflow-auto rounded bg-gray-50 p-3 text-xs">{JSON.stringify(preview.normalized_event, null, 2)}</pre></div>}
        {preview?.resolved_customer_id && <Typography.Paragraph>{t`Resolved Customer UUID`}: <code>{preview.resolved_customer_id}</code></Typography.Paragraph>}
        {preview?.attribute_changes && <div><h3 className="mb-2 font-medium">{t`Proposed attribute changes`}</h3><pre className="overflow-auto rounded bg-gray-50 p-3 text-xs">{JSON.stringify(preview.attribute_changes, null, 2)}</pre></div>}
      </div> }
    ]} />
  </Drawer>
}
