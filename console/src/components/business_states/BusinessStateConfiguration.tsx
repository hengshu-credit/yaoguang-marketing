import { useEffect, useRef, useState } from 'react'
import { Alert, Button, Checkbox, Collapse, Form, Input, InputNumber, Select, Space, Table, Tabs, Tag, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useBusinessCatalogue } from '../business/useBusinessCatalogue'
import { businessApi, type BusinessDefinition } from '../../services/api/business'
import { contactFileSchema, type ContactFileField } from '../../utils/contactFileSchema'
import { subjectStatesApi, type SubjectStateInterval } from '../../services/api/subject_states'
import type { StateRule } from '../../services/api/funnels'
import { StateRuleEditor, type StateAttributeOption } from './StateConditionTree'
import { normalizeStateRule } from './state-rule-utils'
import { StateCoveragePanel } from './StateCoveragePanel'

const emptyRule = (): StateRule => ({ kind: 'all', children: [{ kind: 'event', event: '', minimum: 1 }] })

export function BusinessStatesSettings({ workspaceId, canManage = false }: { workspaceId: string; canManage?: boolean }) {
 const { t } = useLingui()
 const [refresh, setRefresh] = useState(0), [record, setRecord] = useState<BusinessDefinition>()
 const [key, setKey] = useState(''), [name, setName] = useState(''), [subjectType, setSubjectType] = useState('customer')
 const [group, setGroup] = useState(''), [exclusive, setExclusive] = useState(false), [priority, setPriority] = useState(0)
 const [rule, setRule] = useState<StateRule>(emptyRule), [status, setStatus] = useState<'active' | 'draft' | 'archived'>('active')
 const [error, setError] = useState(''), [busy, setBusy] = useState(false), [saved, setSaved] = useState(false)
 const [fields, setFields] = useState<ContactFileField[]>([])
 const [dirty, setDirty] = useState(false)
 const inspectionGeneration = useRef(0)
 const [subject, setSubject] = useState(''), [inspecting, setInspecting] = useState(false), [result, setResult] = useState<SubjectStateInterval[]>()
 const catalogue = useBusinessCatalogue(workspaceId, refresh)
 useEffect(() => { let current = true; contactFileSchema.load(workspaceId).then(value => { if (current) setFields(value) }).catch(() => { /* Free-form paths remain available when metadata cannot be read. */ }); return () => { current = false } }, [workspaceId, refresh])
 const objects = catalogue.definitions.filter(definition => definition.kind === 'object_type' && definition.status === 'active')
 const events = catalogue.definitions.filter(definition => definition.kind === 'event' && definition.status === 'active')
 const states = catalogue.definitions.filter(definition => definition.kind === 'state')
 const objectAttributes = Object.fromEntries(objects.map(object => [object.key, Object.entries(object.schema.properties ?? {}).map(([key, field]) => ({ key, name: field.title || key, type: typeof field.type === 'string' ? field.type : 'string' }))]))
 const attributes: StateAttributeOption[] = subjectType === 'customer' ? fields.filter(field => !['email', 'external_id', 'created_at', 'updated_at', 'language', 'timezone', 'profile.tags', 'profile.status'].includes(field.key)).map(field => ({ key: field.key.replace(/^profile\.attributes\./, ''), name: field.header, type: field.type })) : objectAttributes[subjectType] ?? []
 const clearFeedback = () => { setDirty(true); inspectionGeneration.current++; setInspecting(false); setSaved(false); setResult(undefined); setError('') }
 const reset = () => { setRecord(undefined); setKey(''); setName(''); setSubjectType('customer'); setGroup(''); setExclusive(false); setPriority(0); setStatus('active'); setRule(emptyRule()); setSubject(''); clearFeedback() }
 const save = async () => {
  if (!canManage) return
  setBusy(true); setError(''); setSaved(false); setResult(undefined)
  try {
   const definition = await businessApi.save(workspaceId, { ...record, kind: 'state', key, name, schema: { type: 'object' }, settings: { subject_type: subjectType, group: exclusive ? group : '', exclusive, priority: exclusive ? priority : 0, rule: normalizeStateRule(rule) }, status })
   setRecord(definition); setRefresh(value => value + 1); setSaved(true); setDirty(false)
  } catch (cause) { setError(cause instanceof Error ? cause.message : t`Unable to save the state. Check the rule, dependencies and permissions.`) }
  finally { setBusy(false) }
 }
 const inspect = async () => {
  const generation = ++inspectionGeneration.current
  setInspecting(true); setError(''); setResult(undefined)
  try { const response = await subjectStatesApi.inspect(workspaceId, subjectType, subject); if (generation === inspectionGeneration.current) setResult(response.states) }
  catch (cause) { if (generation === inspectionGeneration.current) setError(cause instanceof Error ? cause.message : String(cause)) }
  finally { if (generation === inspectionGeneration.current) setInspecting(false) }
 }
 const labels = { true: t`Satisfied`, false: t`Not satisfied`, unknown: t`Unknown` }
 return <div className="space-y-5">
  <div><Typography.Title level={3}>{t`Customer and object states`}</Typography.Title><Typography.Paragraph type="secondary">{t`States are calculated labels, such as active customer or overdue contract. They are used in customer details, automation conditions and state coverage. They do not replace the customer profile status field.`}</Typography.Paragraph></div>
  <Alert type="info" showIcon title={t`How state rules take effect`} description={t`Activate a rule, then customer attribute changes and reported events trigger background calculation. True means matched, false means not matched, and unknown means evidence is missing. Check a real customer or object below after saving; the check does not send marketing messages.`} />
  <Tabs items={[
   { key: 'rules', label: t`Business state rules`, children: <div className="space-y-5">
    {(error || catalogue.error) && <Alert type="error" showIcon title={error || t`Unable to load the business catalogue.`} />}
    {saved && <Alert type="success" showIcon title={t`State rule saved`} description={t`Active rules are queued for recalculation. Inspect a customer or object below to see the saved rule result now.`} />}
    <Space wrap><Select aria-label={t`Existing state`} placeholder={t`Existing state`} style={{ minWidth: 260 }} loading={catalogue.loading} value={record?.key} options={states.map(state => ({ value: state.key, label: `${state.name} (${state.key})` }))} onChange={selected => {
     const state = states.find(definition => definition.key === selected)
     if (state) { clearFeedback(); setRecord(state); setKey(state.key); setName(state.name); setSubjectType(String(state.settings.subject_type)); setRule(normalizeStateRule(state.settings.rule as StateRule)); setGroup(String(state.settings.group ?? '')); setExclusive(!!state.settings.exclusive); setPriority(Number(state.settings.priority ?? 0)); setStatus(state.status); setSubject(''); setDirty(false) }
    }} /><Button onClick={reset}>{t`New state`}</Button></Space>
    <Form layout="vertical" disabled={!canManage || busy}>
     <div className="grid grid-cols-1 gap-x-5 md:grid-cols-2">
      <Form.Item label={t`State name`} required><Input aria-label={t`State name`} value={name} onChange={event => { setName(event.target.value); clearFeedback() }} /></Form.Item>
      <Form.Item label={t`State key`} required extra={t`A stable identifier used by automation rules.`}><Input aria-label={t`State key`} disabled={!canManage || !!record} value={key} onChange={event => { setKey(event.target.value); clearFeedback() }} /></Form.Item>
      <Form.Item label={t`State object`}><Select aria-label={t`State object`} value={subjectType} options={[{ value: 'customer', label: t`Customer` }, ...objects.map(object => ({ value: object.key, label: object.name }))]} onChange={value => { setSubjectType(value); setRule(emptyRule()); setSubject(''); clearFeedback() }} /></Form.Item>
      <Form.Item label={t`Status`}><Select aria-label={t`State status`} value={status} options={[{ value: 'active', label: t`Active` }, { value: 'draft', label: t`Draft` }, { value: 'archived', label: t`Archived` }]} onChange={value => { setStatus(value); clearFeedback() }} /></Form.Item>
     </div>
     <Collapse items={[{ key: 'exclusive', label: t`Optional: mutually exclusive states`, children: <Space orientation="vertical" className="w-full"><Typography.Paragraph>{t`Leave this off for independent states. In the same exclusive group, only the highest-priority matched state remains true.`}</Typography.Paragraph><Checkbox checked={exclusive} onChange={event => { setExclusive(event.target.checked); clearFeedback() }}>{t`Exclusive group`}</Checkbox>{exclusive && <Space wrap><Input aria-label={t`State group`} placeholder={t`State group`} value={group} onChange={event => { setGroup(event.target.value); clearFeedback() }} /><InputNumber aria-label={t`Conflict priority`} value={priority} onChange={value => { setPriority(value ?? 0); clearFeedback() }} /></Space>}</Space> }]} />
    </Form>
    <StateRuleEditor subjectType={subjectType} value={rule} onChange={value => { setRule(value); clearFeedback() }} objects={objects} events={events.map(event => ({ key: event.key, name: catalogue.displayName(event) }))} states={states.filter(state => state.key !== key)} attributes={attributes} objectAttributes={objectAttributes} relations={catalogue.definitions.filter(definition => definition.kind === 'object_relation' && definition.status === 'active')} readOnly={!canManage || busy} />
    <Button type="primary" onClick={() => void save()} loading={busy} disabled={!canManage || !key || !name || (exclusive && !group.trim())}>{t`Save state`}</Button>
    <div className="rounded-lg border border-slate-200 p-4 space-y-3"><Typography.Title level={5}>{t`Inspect a saved state on real data`}</Typography.Title><Typography.Paragraph type="secondary">{t`Save the rule first. For customers enter an email, external user ID or UUID; for business objects enter the object UUID. Results come from saved active rules.`}</Typography.Paragraph><Space.Compact className="w-full"><Input aria-label={t`Customer or object identifier`} value={subject} onChange={event => { setSubject(event.target.value); inspectionGeneration.current++; setInspecting(false); setResult(undefined) }} /><Button loading={inspecting} disabled={dirty || busy || !subject.trim() || !record || status !== 'active'} onClick={() => void inspect()}>{t`Inspect state`}</Button></Space.Compact>
     {result && <Table rowKey="state_key" dataSource={result} pagination={false} scroll={{ x: 580 }} columns={[{ title: t`State`, dataIndex: 'state_key', render: value => states.find(state => state.key === value)?.name || value }, { title: t`Result`, dataIndex: 'value', render: (value: keyof typeof labels) => <Tag color={value === 'true' ? 'green' : value === 'false' ? 'default' : 'orange'}>{labels[value]}</Tag> }, { title: t`Rule revision`, dataIndex: 'rule_revision' }, { title: t`Reason`, dataIndex: 'reason' }, { title: t`Computed at`, dataIndex: 'computed_at' }]} />}
    </div>
   </div> },
   { key: 'coverage', label: t`State coverage and intersection`, children: <StateCoveragePanel workspaceId={workspaceId} /> }
  ]} />
 </div>
}
