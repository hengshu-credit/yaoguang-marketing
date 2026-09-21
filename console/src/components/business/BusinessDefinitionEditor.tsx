import { useEffect, useRef, useState } from 'react'
import { Alert, Button, Drawer, Form, Input, Modal, Select, Space, Tabs, Tag, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { businessDefinitionDisplayName } from './builtin-event-labels'
import { businessApi, type BusinessDefinition, type BusinessPreviewResult } from '../../services/api/business'
import { SchemaFieldsEditor } from './SchemaFieldsEditor'
import { BusinessDefinitionSettings } from './BusinessDefinitionSettings'
import { BuiltinEventDetails } from './BuiltinEventDetails'
import { reportingIssues, reportingSample } from './event-reporting'
import { reportingIssueMessage } from './reporting-messages'
import { EventReportingGuide } from './EventReportingGuide'

interface Props { workspaceId: string; definition: BusinessDefinition; catalogue: BusinessDefinition[]; readOnly?: boolean; onSaved: (definition: BusinessDefinition) => void; onClose: () => void }

export function BusinessDefinitionEditor(props: Props) {
  return <DefinitionEditor key={`${props.workspaceId}:${props.definition.kind}:${props.definition.key}:${props.definition.revision ?? 0}`} {...props} />
}

function DefinitionEditor({ workspaceId, definition, catalogue, readOnly, onSaved, onClose }: Props) {
  const { t, i18n } = useLingui()
  const [draft, setDraft] = useState<BusinessDefinition>(() => structuredClone(definition))
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [sample, setSample] = useState(() => {
    try { return JSON.stringify(reportingSample(definition, catalogue), null, 2) }
    catch { return definition.kind === 'customer_attribute' ? '{"value": ""}' : '{}' }
  })
  const [sampleChanged, setSampleChanged] = useState(false)
  const [previewing, setPreviewing] = useState(false)
  const [preview, setPreview] = useState<BusinessPreviewResult>()
  const [attempted, setAttempted] = useState(false)
  const [activeTab, setActiveTab] = useState(definition.kind === 'object_type' ? 'schema' : 'settings')
  const [confirmClose, setConfirmClose] = useState(false)
  const [confirmGenerate, setConfirmGenerate] = useState(false)
  const generation = useRef(0)
  const savingRef = useRef(false)
  const alive = useRef(true)
  const protectedDefinition = !!(readOnly || draft.builtin || draft.settings.builtin === true)
  const disabled = protectedDefinition || saving
  const reporting = draft.kind === 'event' || draft.kind === 'event_mapping'
  const dirty = JSON.stringify(draft) !== JSON.stringify(definition)
  const issues = reportingIssues(draft, attempted || draft.status === 'active', catalogue)
  const unsaved = (!protectedDefinition && dirty) || sampleChanged

  useEffect(() => {
    alive.current = true
    return () => { alive.current = false }
  }, [])
  useEffect(() => {
    if (!unsaved) return
    const beforeUnload = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = '' }
    window.addEventListener('beforeunload', beforeUnload)
    return () => window.removeEventListener('beforeunload', beforeUnload)
  }, [unsaved])

  const invalidatePreview = () => { generation.current++; setPreview(undefined); setPreviewing(false) }
  const change = (next: BusinessDefinition) => { if (savingRef.current) return; invalidatePreview(); setDraft(next); setError('') }
  const close = () => { if (savingRef.current) return; if (unsaved) setConfirmClose(true); else onClose() }
  const save = async (status = draft.status) => {
    if (protectedDefinition || savingRef.current) return
    if (!/^[a-zA-Z][a-zA-Z0-9_.-]{0,127}$/.test(draft.key) || !draft.name.trim()) { setError(t`Enter a name and a stable key starting with a letter.`); return }
    const next = { ...draft, name: draft.name.trim(), status }
    const invalid = reportingIssues(next, status === 'active', catalogue)
    if (invalid.length) { setAttempted(status === 'active'); setActiveTab('settings'); setError(reportingIssueMessage(invalid[0], i18n)); return }
    savingRef.current = true; setSaving(true); setError(''); invalidatePreview()
    try {
      const saved = await businessApi.save(workspaceId, next)
      if (alive.current) onSaved(saved)
    } catch (cause) { if (alive.current) setError(cause instanceof Error ? cause.message : t`Failed to save definition`) }
    finally { savingRef.current = false; if (alive.current) setSaving(false) }
  }
  const validate = async () => {
    if (savingRef.current) return
    setError(''); setPreview(undefined); setAttempted(true)
    const invalid = reportingIssues(draft, true, catalogue)
    if (invalid.length) { setActiveTab('settings'); setError(reportingIssueMessage(invalid[0], i18n)); return }
    let data: Record<string, unknown>
    try { const parsed = JSON.parse(sample); if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') throw new Error(); data = parsed }
    catch { setError(t`Enter a valid JSON object for the sample.`); return }
    const ticket = ++generation.current
    setPreviewing(true)
    try {
      const result = await businessApi.preview(workspaceId, structuredClone(draft), data)
      if (alive.current && ticket === generation.current) setPreview(result)
    } catch (cause) { if (alive.current && ticket === generation.current) setError(cause instanceof Error ? cause.message : t`Failed to validate sample`) }
    finally { if (alive.current && ticket === generation.current) setPreviewing(false) }
  }
  const generate = () => {
    try {
      const next = reportingSample(draft, catalogue)
      invalidatePreview(); setSample(JSON.stringify(next, null, 2)); setSampleChanged(false); setError('')
    } catch { setError(t`A sample could not be generated because configured paths overlap or are unsupported. Check the paths or paste your own payload.`) }
    setConfirmGenerate(false)
  }
  if (draft.kind === 'event' && (draft.builtin || draft.settings.builtin === true)) return <BuiltinEventDetails definition={draft} onClose={onClose} />
  const statuses = { draft: t`Draft`, active: t`Active`, archived: t`Archived` }
  return <>
    <Drawer open title={definition.id ? t`Definition details` : t`New definition`} size={920} onClose={close} maskClosable={!saving} keyboard={!saving} extra={<Button disabled={saving} onClick={close}>{t`Close`}</Button>} footer={<div className="flex flex-wrap items-center justify-between gap-3">
      <Typography.Text type="secondary">{saving ? t`Saving...` : dirty ? t`Unsaved changes` : t`No unsaved configuration changes`}</Typography.Text>
      <Space>{!protectedDefinition && <>
        <Button type={reporting && draft.status === 'draft' ? 'default' : 'primary'} loading={saving} onClick={() => void save()}>{reporting && draft.status === 'draft' ? t`Save draft` : t`Save definition`}</Button>
        {reporting && draft.status === 'draft' && <Button type="primary" disabled={saving} onClick={() => void save('active')}>{t`Save and activate`}</Button>}
      </>}</Space>
    </div>}>
      {error && <Alert className="mb-4" type="error" showIcon title={error} />}
      {reporting && <Alert className="mb-4" type="info" showIcon title={t`Configure → validate a sample → activate → integrate`} description={t`Drafts do not accept reports. Activation uses the current configuration; sample validation never writes customer data or triggers marketing.`} />}
      <Form layout="vertical">
        <div className="grid grid-cols-1 gap-x-6 md:grid-cols-2">
          <Form.Item label={t`Definition key`} required extra={t`A stable identifier used in event reports and rules.`}><Input aria-label={t`Definition key`} value={draft.key} disabled={disabled || !!definition.id} onChange={event => change({ ...draft, key: event.target.value })} /></Form.Item>
          <Form.Item label={t`Definition name`} required><Input aria-label={t`Definition name`} value={businessDefinitionDisplayName(draft, i18n)} disabled={disabled} onChange={event => change({ ...draft, name: event.target.value })} /></Form.Item>
          <Form.Item label={t`Status`}>{reporting ? <Tag color={draft.status === 'active' ? 'green' : undefined}>{statuses[draft.status]}</Tag> : <Select aria-label={t`Definition status`} disabled={disabled} value={draft.status} options={Object.entries(statuses).map(([value, label]) => ({ value, label }))} onChange={status => change({ ...draft, status })} />}</Form.Item>
          <Form.Item label={t`Description`}><Input.TextArea rows={2} value={draft.description} disabled={disabled} onChange={event => change({ ...draft, description: event.target.value })} /></Form.Item>
        </div>
      </Form>
      <Tabs activeKey={activeTab} onChange={setActiveTab} items={[
        { key: 'settings', label: t`Configuration`, children: <BusinessDefinitionSettings value={draft} catalogue={catalogue} onChange={change} disabled={disabled} issues={issues} /> },
        ...(draft.kind !== 'object_relation' && draft.kind !== 'event_mapping' ? [{ key: 'schema', label: draft.kind === 'event' ? t`Report data fields` : t`Fields and validation`, children: draft.kind === 'customer_attribute' ? <Form layout="vertical"><Form.Item label={t`Attribute value type`}><Select value={draft.schema.type} disabled={disabled} options={[{ value: 'string', label: t`Text` }, { value: 'number', label: t`Number` }, { value: 'integer', label: t`Integer` }, { value: 'boolean', label: t`Boolean` }, { value: 'array', label: t`Array` }, { value: 'object', label: t`Object` }]} onChange={type => change({ ...draft, schema: { type, ...(type === 'object' ? { properties: {} } : {}), ...(type === 'array' ? { items: { type: 'string' } } : {}) } })} /></Form.Item>{draft.schema.type === 'object' && <SchemaFieldsEditor value={draft.schema} disabled={disabled} onChange={schema => change({ ...draft, schema })} />}</Form> : <SchemaFieldsEditor value={draft.schema} disabled={disabled} onChange={schema => change({ ...draft, schema })} /> }] : []),
        { key: 'preview', label: t`Sample validation`, children: <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          <Typography.Paragraph type="secondary">{t`Validate the current configuration against a sample before saving. This preview does not emit events or update customer data.`}</Typography.Paragraph>
          {reporting && <Alert type="info" showIcon title={draft.kind === 'event_mapping' ? t`Paste the external JSON, before field mapping.` : t`Paste only the event object, not the workspace_id/source/event request wrapper.`} />}
          {draft.kind === 'customer_attribute' && <Typography.Paragraph type="secondary">{t`Put the attribute sample in the value field of the JSON object.`}</Typography.Paragraph>}
          <Button disabled={saving} onClick={() => sampleChanged ? setConfirmGenerate(true) : generate()}>{t`Generate sample from configuration`}</Button>
          <Input.TextArea aria-label={t`Sample JSON`} rows={12} value={sample} disabled={saving} onChange={event => { invalidatePreview(); setSample(event.target.value); setSampleChanged(true); setError('') }} style={{ fontFamily: 'ui-monospace, SFMono-Regular, Consolas, monospace', fontSize: 13 }} />
          <Button aria-label={t`Validate sample`} loading={previewing} disabled={saving} onClick={() => void validate()}>{t`Validate sample`}</Button>
          {preview && <Alert type={preview.valid ? 'success' : 'error'} showIcon title={preview.valid ? t`Sample is valid` : t`Sample validation failed`} description={preview.errors?.length ? <ul>{preview.errors.map((item, index) => <li key={index}><code>{item.field}</code>: {item.message}</li>)}</ul> : t`Validation passed for this configuration and sample only. Source permissions, browser credentials and live ingestion still apply.`} />}
          {preview?.normalized_event && <div><h3 className="mb-2 font-medium">{t`Normalized event`}</h3><pre className="overflow-auto rounded bg-gray-50 p-3 text-xs">{JSON.stringify(preview.normalized_event, null, 2)}</pre></div>}
          {preview?.warnings?.map((warning, index) => <Alert key={index} type="warning" showIcon title={warning.message} description={warning.field} />)}
          {preview?.resolved_customer_id && <Typography.Paragraph>{t`Resolved Customer UUID`}: <code>{preview.resolved_customer_id}</code></Typography.Paragraph>}
          {preview?.attribute_changes && <div><h3 className="mb-2 font-medium">{t`Proposed attribute changes`}</h3><pre className="overflow-auto rounded bg-gray-50 p-3 text-xs">{JSON.stringify(preview.attribute_changes, null, 2)}</pre></div>}
        </div> },
        ...(reporting ? [{ key: 'integration', label: t`Integration instructions`, children: <EventReportingGuide workspaceId={workspaceId} definition={draft} sample={sample} dirty={dirty} /> }] : [])
      ]} />
    </Drawer>
    <Modal open={confirmClose} title={t`Discard unsaved changes?`} okText={t`Discard changes`} cancelText={t`Keep editing`} okButtonProps={{ danger: true }} onCancel={() => setConfirmClose(false)} onOk={onClose}><Typography.Paragraph>{t`Your configuration edits and sample input will be lost.`}</Typography.Paragraph></Modal>
    <Modal open={confirmGenerate} title={t`Replace the edited sample?`} okText={t`Generate sample`} cancelText={t`Keep sample`} onCancel={() => setConfirmGenerate(false)} onOk={generate}><Typography.Paragraph>{t`The new sample uses the current fields and mapping. Your pasted payload will be replaced.`}</Typography.Paragraph></Modal>
  </>
}
