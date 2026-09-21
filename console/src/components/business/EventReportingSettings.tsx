import { useState } from 'react'
import { Alert, Card, Form, Input, InputNumber, Select, Space, Switch, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { BusinessDefinition } from '../../services/api/business'
import { DURATION_UNITS, durationUnitFor, exactBrowserOrigin, MAX_EVENT_TTL_SECONDS, type DurationUnit, type ReportingIssue } from './event-reporting'
import { reportingIssueMessage } from './reporting-messages'

interface Props { value: BusinessDefinition; catalogue: BusinessDefinition[]; disabled?: boolean; issues?: ReportingIssue[]; onChange: (value: BusinessDefinition) => void }

export function EventReportingSettings({ value, catalogue, disabled, issues = [], onChange }: Props) {
  const { t, i18n } = useLingui()
  const s = value.settings
  const [unit, setUnit] = useState<DurationUnit>(() => durationUnitFor(s.ttl_seconds))
  const update = (patch: Record<string, unknown>, remove: string[] = []) => {
    const settings = { ...s, ...patch }
    remove.forEach(key => delete settings[key])
    onChange({ ...value, settings })
  }
  const feedback = (field: string) => {
    const issue = issues.find(issue => issue.field === `settings.${field}`)
    return issue ? { validateStatus: 'error' as const, help: reportingIssueMessage(issue, i18n) } : {}
  }
  const objects = catalogue.filter(item => item.kind === 'object_type' && item.status !== 'archived')
  const subject = typeof s.subject_type === 'string' && s.subject_type ? s.subject_type : 'customer'
  const selectedObject = objects.find(item => item.key === subject)
  const mode = s.browser_enabled === true ? 'browser' : 'backend'
  const expiry = typeof s.expiry_mode === 'string' && s.expiry_mode ? s.expiry_mode : 'required'
  const restricted = s.allowed_sources !== undefined

  return <Form layout="vertical"><Space orientation="vertical" size="middle" className="w-full">
    <Card size="small" title={t`Reporting access`}>
      <Form.Item label={t`Reporting mode`} {...feedback('browser_enabled')}>
        <Select aria-label={t`Reporting mode`} value={mode} disabled={disabled} onChange={next => update({ browser_enabled: next === 'browser', authoritative: next !== 'browser' })} options={[
          { value: 'backend', label: t`Authenticated backend only` },
          { value: 'browser', label: t`Browser behavior and authenticated backend` }
        ]} />
      </Form.Item>
      <Typography.Paragraph type="secondary">{t`Backend requests use a workspace API credential. Browser requests use the SDK and a short-lived identity credential, never an API key.`}</Typography.Paragraph>
      {mode === 'browser' && <>
        <Form.Item label={t`Allowed browser origins`} required {...feedback('allowed_browser_origins')} extra={t`Enter a website origin such as https://app.example.com. A trailing slash or default port is normalized; a page path is not accepted.`}>
          <Select mode="tags" tokenSeparators={[',', '\n']} aria-label={t`Allowed browser origins`} value={Array.isArray(s.allowed_browser_origins) ? s.allowed_browser_origins as string[] : []} disabled={disabled} onChange={next => update({ allowed_browser_origins: [...new Set(next.map(origin => exactBrowserOrigin(origin) ?? origin.trim()))] })} />
        </Form.Item>
        <Alert type="warning" showIcon title={t`Browser reports are untrusted behavior signals`} description={t`Use authenticated backend reports for business outcomes such as disbursement and repayment.`} />
      </>}
      <Form.Item className="mt-4" label={t`Restrict backend source systems`} extra={t`When disabled, any authenticated source is allowed. Browser access is controlled separately by the origin list.`}>
        <Switch aria-label={t`Restrict backend source systems`} disabled={disabled} checked={restricted} onChange={enabled => update(enabled ? { allowed_sources: [] } : {}, enabled ? [] : ['allowed_sources'])} />
      </Form.Item>
      {restricted && <Form.Item label={t`Allowed source systems`} required {...feedback('allowed_sources')} extra={t`These values match the source field of events.track exactly. An empty restricted list accepts no backend reports.`}>
        <Select mode="tags" aria-label={t`Allowed source systems`} tokenSeparators={[',', '\n']} value={Array.isArray(s.allowed_sources) ? s.allowed_sources as string[] : []} disabled={disabled} onChange={next => update({ allowed_sources: [...new Set(next.map(source => source.trim()).filter(Boolean))] })} />
      </Form.Item>}
    </Card>
    <Card size="small" title={t`Customer and business object`}>
      <Form.Item label={t`Event subject type`} {...feedback('subject_type')} extra={t`Choose Customer for a user action, or a business object for one specific order, contract or application.`}>
        <Select aria-label={t`Event subject type`} showSearch optionFilterProp="label" value={subject} disabled={disabled} onChange={next => update({ subject_type: next })} options={[
          { value: 'customer', label: t`Customer` }, ...objects.map(item => ({ value: item.key, label: `${item.name} (${item.key})${item.status === 'draft' ? ` · ${t`Draft`}` : ''}` })),
          ...(subject !== 'customer' && !selectedObject ? [{ value: subject, label: `${subject} · ${t`Unavailable`}` }] : [])
        ]} />
      </Form.Item>
      {subject !== 'customer' && <>
        {selectedObject?.status !== 'active' && <Alert className="mb-3" type="warning" showIcon title={t`Activate the referenced object type before activating this event.`} />}
        <Form.Item label={t`External object ID path`} required {...feedback('subject_id_path')} extra={t`Path in the normalized event. The value must be the external ID of this particular business record.`}><Input aria-label={t`External object ID path`} value={String(s.subject_id_path ?? '')} placeholder="$.data.order_id" disabled={disabled} onChange={event => update({ subject_id_path: event.target.value })} /></Form.Item>
        <Form.Item label={t`Object attributes path`} {...feedback('subject_attributes_path')} extra={t`Optional: an object in the normalized event used to update the business record.`}><Input aria-label={t`Object attributes path`} value={String(s.subject_attributes_path ?? '')} placeholder="$.data.order" disabled={disabled} onChange={event => update(event.target.value.trim() ? { subject_attributes_path: event.target.value } : {}, event.target.value.trim() ? [] : ['subject_attributes_path'])} /></Form.Item>
      </>}
      <Form.Item label={t`When no customer matches`} extra={t`This applies to backend reporting. Anonymous browser events always remain pending until a verified identity is linked.`}>
        <Select aria-label={t`When no customer matches`} value={s.unmatched_customer || 'pending'} disabled={disabled} onChange={next => update({ unmatched_customer: next })} options={[
          { value: 'pending', label: t`Keep pending for identity resolution` }, { value: 'reject', label: t`Reject the report` }, { value: 'create', label: t`Create a customer` }
        ]} />
      </Form.Item>
      {s.unmatched_customer === 'create' && <Alert type="warning" showIcon title={t`New identities can create customers. Use stable identifiers and check the source before enabling this behavior.`} />}
    </Card>
    <Card size="small" title={t`Event validity`}>
      <Form.Item label={t`Event expiration`} {...feedback('expiry_mode')}>
        <Select aria-label={t`Event expiration`} value={expiry} disabled={disabled} onChange={next => update({ expiry_mode: next })} options={[
          { value: 'never', label: t`No expiration required` }, { value: 'duration', label: t`Default duration after occurrence` }, { value: 'required', label: t`Require expiration in the report` }
        ]} />
      </Form.Item>
      {expiry === 'duration' && <Form.Item label={t`Default validity duration`} required {...feedback('ttl_seconds')}>
        <Space.Compact className="w-full">
          <InputNumber aria-label={t`Default validity duration`} className="w-full" disabled={disabled} value={typeof s.ttl_seconds === 'number' ? s.ttl_seconds / DURATION_UNITS[unit] : null} min={1 / DURATION_UNITS[unit]} max={MAX_EVENT_TTL_SECONDS / DURATION_UNITS[unit]} onChange={next => update(next === null ? {} : { ttl_seconds: next * DURATION_UNITS[unit] }, next === null ? ['ttl_seconds'] : [])} />
          <Select aria-label={t`Duration unit`} value={unit} disabled={disabled} onChange={setUnit} options={[
            { value: 'seconds', label: t`Seconds` }, { value: 'minutes', label: t`Minutes` }, { value: 'hours', label: t`Hours` }, { value: 'days', label: t`Days` }
          ]} />
        </Space.Compact>
      </Form.Item>}
      <Typography.Paragraph type="secondary">{expiry === 'never' ? t`expires_at may be omitted. An explicitly reported expiration still takes effect.` : expiry === 'duration' ? t`The default starts at triggered_at, not receipt time. A supplied expires_at takes precedence.` : t`Each report must provide expires_at, no earlier than triggered_at.`}</Typography.Paragraph>
      <Alert type="info" showIcon title={t`Expired events remain historical facts`} description={t`Expiration prevents new real-time marketing actions; it does not delete the event or erase historical analysis.`} />
    </Card>
  </Space></Form>
}
