import { Alert, Button, Card, Form, Input, Select, Switch } from 'antd'
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import type { BusinessDefinition } from '../../services/api/business'
import { EventReportingSettings } from './EventReportingSettings'
import { setOptionalMappingPath, type ReportingIssue } from './event-reporting'
import { reportingIssueMessage } from './reporting-messages'

interface Props { value: BusinessDefinition; catalogue: BusinessDefinition[]; onChange: (value: BusinessDefinition) => void; disabled?: boolean; issues?: ReportingIssue[] }
type Mapping = { attribute: string; path: string; action: string; null_policy: string; update_policy: string }
const stringValue = (value: unknown) => typeof value === 'string' ? value : undefined

export function BusinessDefinitionSettings({ value, catalogue, onChange, disabled, issues = [] }: Props) {
  const { t, i18n } = useLingui()
  const settings = value.settings
  const set = (key: string, next: unknown) => onChange({ ...value, settings: { ...settings, [key]: next } })
  const objects = [{ value: 'customer', label: t`Customer` }, ...catalogue.filter((item) => item.kind === 'object_type' && item.status !== 'archived').map((item) => ({ value: item.key, label: `${item.name} (${item.key})` }))]
  const attributes = [
    { value: 'customer_id', label: t`Customer UUID` }, { value: 'external_user_id', label: t`External user ID` },
    { value: 'email', label: t`Email address` }, { value: 'phone', label: t`Phone number` }, { value: 'national_id', label: t`National ID` },
    ...catalogue.filter((item) => item.kind === 'customer_attribute' && item.status !== 'archived').map((item) => ({ value: item.key, label: `${item.name} (${item.key})` }))
  ].filter((item, index, all) => all.findIndex((other) => other.value === item.value) === index)
  const select = (key: string, label: string, options: Array<{ value: string; label: string }>, fallback?: string) => <Form.Item label={label}><Select aria-label={label} value={stringValue(settings[key]) ?? fallback} options={options} disabled={disabled} onChange={(next) => set(key, next)} /></Form.Item>
  const bool = (key: string, label: string, fallback = false) => <Form.Item label={label}><Switch aria-label={label} checked={typeof settings[key] === 'boolean' ? settings[key] : fallback} disabled={disabled} onChange={(next) => set(key, next)} /></Form.Item>

  if (value.kind === 'customer_attribute') return <Form layout="vertical">
    <div className="grid grid-cols-1 gap-x-6 md:grid-cols-2">
      {bool('identity', t`Use for customer identity matching`)}{bool('sensitive', t`Sensitive value`)}
      {bool('writable', t`Allow profile updates`, true)}
      {select('normalization', t`Value normalization`, [{ value: 'trim', label: t`Trim whitespace` }, { value: 'lowercase', label: t`Lowercase and trim` }, { value: 'phone', label: t`Phone number normalization` }, { value: 'none', label: t`Preserve original value` }], 'trim')}
    </div>
    <Alert type="info" showIcon title={t`Customer UUID remains the primary key`} description={t`Identity attributes help resolve a customer. An event attribute alone does not authorize merging customers.`} />
  </Form>

  if (value.kind === 'object_type') return <Form layout="vertical">
    <Form.Item label={t`Object identity fields`} extra={t`Select fields that identify this object within its source system. Every object belongs to a Customer UUID.`}>
      <Select mode="multiple" aria-label={t`Object identity fields`} value={Array.isArray(settings.identity_fields) ? settings.identity_fields as string[] : []} options={Object.keys(value.schema.properties ?? {}).map((key) => ({ value: key, label: key }))} disabled={disabled} onChange={(keys) => set('identity_fields', keys)} />
    </Form.Item>
    <Alert type="info" showIcon title={t`A customer can have many objects of this type`} description={t`Events use the configured object type and external object ID to connect a single business process. Object IDs from different sources remain separate.`} />
  </Form>

  if (value.kind === 'object_relation') return <Form layout="vertical">
    <div className="grid grid-cols-1 gap-x-6 md:grid-cols-2">{select('from_type', t`Parent object type`, objects.filter(item => item.value !== 'customer'))}{select('to_type', t`Child object type`, objects.filter(item => item.value !== 'customer' && item.value !== settings.from_type))}</div>
    {select('cardinality', t`Relationship cardinality`, [{ value: 'one_to_one', label: t`One to one` }, { value: 'one_to_many', label: t`One to many` }, { value: 'many_to_many', label: t`Many to many` }], 'one_to_many')}
    <Alert type="info" showIcon title={t`Relations connect configured objects`} description={t`Use this relation when analyzing a parent process and its child processes. Counts keep the selected object as their unit.`} />
  </Form>

  if (value.kind === 'event') return <EventReportingSettings value={value} catalogue={catalogue} onChange={onChange} disabled={disabled} issues={issues} />

  if (value.kind === 'event_mapping') {
    const fields = typeof settings.fields === 'object' && settings.fields ? settings.fields as Record<string, string> : {}
    const mappings = Array.isArray(settings.attribute_mappings) ? settings.attribute_mappings as Mapping[] : []
    const fixedEvent = catalogue.find(item => item.kind === 'event' && item.key === settings.event_code)
    const updateMapping = (index: number, changes: Partial<Mapping>) => set('attribute_mappings', mappings.map((item, position) => position === index ? { ...item, ...changes } : item))
    const envelope = [
      { key: 'report_id', label: t`Report ID path`, required: true },
      { key: 'event_code', label: t`Event code path`, required: !settings.event_code },
      { key: 'event_name', label: t`Event name path`, required: false },
      { key: 'customer_attributes', label: t`Customer attributes path`, required: !mappings.some(mapping => mapping.action === 'lookup' || mapping.action === 'bind') },
      { key: 'data', label: t`Report data object path`, required: true },
      { key: 'triggered_at', label: t`Event trigger time path`, required: true },
      { key: 'expires_at', label: t`Event expiration time path`, required: !!fixedEvent && (!fixedEvent.settings.expiry_mode || fixedEvent.settings.expiry_mode === 'required') },
      { key: 'backfill', label: t`Historical backfill flag path`, required: false }
    ]
    return <Form layout="vertical">
      <Alert className="mb-4" type="info" showIcon title={t`Map the external payload to the event protocol`} description={t`Envelope paths below read the external JSON inside the request's event field, not the whole HTTP request. Optional paths can be cleared.`} />
      <Form.Item label={t`Fixed event code`} extra={t`Choose one event for this mapping, or leave it empty and map event_code from each report.`}>
        <Select aria-label={t`Fixed event code`} allowClear showSearch optionFilterProp="label" disabled={disabled} value={stringValue(settings.event_code) || undefined} options={catalogue.filter(item => item.kind === 'event' && item.status !== 'archived' && !item.builtin && item.settings.builtin !== true).map(item => ({ value: item.key, label: `${item.name} (${item.key})` }))} onChange={event_code => {
          const next = { ...settings }; if (event_code) next.event_code = event_code; else delete next.event_code
          onChange({ ...value, settings: next })
        }} />
      </Form.Item>
      <div className="grid grid-cols-1 gap-x-6 md:grid-cols-2">{envelope.map(({ key, label, required }) => {
        const issue = issues.find(issue => issue.field === `settings.fields.${key}`)
        return <Form.Item label={label} required={required} key={key} validateStatus={issue ? 'error' : undefined} help={issue ? reportingIssueMessage(issue, i18n) : undefined}><Input aria-label={label} disabled={disabled || (key === 'event_code' && !!settings.event_code)} value={fields[key] ?? ''} placeholder={`$.${key}`} onChange={(event) => set('fields', setOptionalMappingPath(fields, key, event.target.value))} /></Form.Item>
      })}</div>
      <Alert className="mb-4" type="info" showIcon title={t`Report time is recorded automatically`} description={t`The platform records the first receipt time. Reusing a report ID with the same content returns the original receipt; different content is rejected.`} />
      <h3 className="mb-3 font-medium">{t`Customer attribute mappings`}</h3>
      <Alert className="mb-3" type="info" showIcon title={t`Attribute paths read the normalized event`} description={t`After the envelope is mapped, use $.data or $.customer_attributes here. These paths do not read the original external payload.`} />
      <div className="space-y-3">{mappings.map((mapping, index) => <Card key={index} size="small" extra={<Button type="text" danger aria-label={t`Remove attribute mapping ${index + 1}`} icon={<DeleteOutlined />} disabled={disabled} onClick={() => set('attribute_mappings', mappings.filter((_, position) => position !== index))} />}>
        <div className="grid grid-cols-1 gap-x-4 md:grid-cols-2">
          <Form.Item label={t`Customer attribute`}><Select aria-label={t`Customer attribute ${index + 1}`} showSearch optionFilterProp="label" value={mapping.attribute} options={attributes} disabled={disabled} onChange={(attribute) => updateMapping(index, { attribute })} /></Form.Item>
          <Form.Item label={t`Source value path`}><Input aria-label={t`Source value path ${index + 1}`} value={mapping.path} disabled={disabled} placeholder="$.data.phone" onChange={(event) => updateMapping(index, { path: event.target.value })} /></Form.Item>
          <Form.Item label={t`Mapping action`}><Select value={mapping.action} disabled={disabled} options={[{ value: 'lookup', label: t`Find customer` }, { value: 'bind', label: t`Bind an identity` }, { value: 'update', label: t`Update customer attribute` }, { value: 'record', label: t`Record event context only` }]} onChange={(action) => updateMapping(index, { action })} /></Form.Item>
          <Form.Item label={t`Explicit null value`}><Select value={mapping.null_policy ?? 'ignore'} disabled={disabled} options={[{ value: 'ignore', label: t`Keep existing value` }, { value: 'clear', label: t`Clear existing value` }]} onChange={(null_policy) => updateMapping(index, { null_policy })} /></Form.Item>
          {mapping.action === 'update' && <Form.Item label={t`Update policy`}><Select value={mapping.update_policy ?? 'overwrite'} disabled={disabled} options={[{ value: 'overwrite', label: t`Replace value` }, { value: 'set_union', label: t`Add unique values` }]} onChange={(update_policy) => updateMapping(index, { update_policy })} /></Form.Item>}
        </div>
      </Card>)}</div>
      <Button className="mt-3" aria-label={t`Add attribute mapping`} icon={<PlusOutlined aria-hidden />} disabled={disabled} onClick={() => set('attribute_mappings', [...mappings, { attribute: '', path: '', action: 'record', null_policy: 'ignore', update_policy: 'overwrite' }])}>{t`Add attribute mapping`}</Button>
    </Form>
  }
  return null
}
