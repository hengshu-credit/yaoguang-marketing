import { useState } from 'react'
import { Alert, Descriptions, Form, Input, Space, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { BusinessDefinition } from '../../services/api/business'
import { reportingRequest } from './event-reporting'

export function EventReportingGuide({ workspaceId, definition, sample, dirty }: { workspaceId: string; definition: BusinessDefinition; sample: string; dirty: boolean }) {
  const { t } = useLingui()
  const sources = Array.isArray(definition.settings.allowed_sources) ? definition.settings.allowed_sources as string[] : []
  const [source, setSource] = useState(sources[0] || 'business-app')
  let event: Record<string, unknown> | undefined
  try { const parsed = JSON.parse(sample); if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) event = parsed } catch { /* The sample panel reports the parse error. */ }
  const copyable = (label: string, value: string) => <div>
    <Typography.Paragraph strong copyable={{ text: value }}>{label}</Typography.Paragraph>
    <pre aria-label={label} className="max-h-96 overflow-auto rounded border border-gray-200 bg-gray-50 p-4 text-xs">{value}</pre>
  </div>
  const sdk = event ? { event_code: event.event_code, data: event.data, ...(event.expires_at ? { expires_at: event.expires_at } : {}) } : undefined

  return <Space orientation="vertical" size="middle" className="w-full">
    {(dirty || definition.status !== 'active') && <Alert type="warning" showIcon title={t`This is a configuration example, not a live endpoint test`} description={t`Save and activate the event and any field mapping before sending. Examples use the current unsaved configuration and sample.`} />}
    <Descriptions size="small" column={1} bordered items={[
      { key: 'endpoint', label: t`Backend endpoint`, children: <Typography.Text code>POST /api/events.track</Typography.Text> },
      { key: 'auth', label: t`Authentication`, children: t`Server-side Bearer API credential with Contacts write permission. Never put it in browser code.` },
      { key: 'time', label: t`Occurrence and receipt time`, children: t`triggered_at is when the action happened. reported_at is assigned by the server and must not be mapped.` },
      { key: 'id', label: t`Report ID`, children: t`Use a new report_id for each real occurrence. Retry the same occurrence with exactly the same ID, source and payload.` }
    ]} />
    <Form layout="vertical"><Form.Item label={t`Example source system`} extra={t`This changes the copied request only, not the saved source restriction.`}><Input aria-label={t`Example source system`} value={source} onChange={event => setSource(event.target.value)} /></Form.Item></Form>
    {sources.length > 0 && !sources.includes(source) && <Alert type="warning" title={t`The example source is not in the allowed source list.`} />}
    {event ? copyable(t`Backend request JSON`, JSON.stringify(reportingRequest(workspaceId, definition, event, source), null, 2)) : <Alert type="info" title={t`Generate or paste a valid sample JSON object on the sample tab first.`} />}
    {definition.kind === 'event' && definition.settings.browser_enabled === true && <>
      <Alert type="info" showIcon title={t`Browser SDK reporting`} description={t`Initialize the SDK for this workspace on an allowed origin. Your backend issues a browserEvents.issueCredential credential for the verified customer and session; call identifyCustomer before identifying reports. Anonymous events remain pending. The SDK generates the occurrence ID and time once and preserves them on retry.`} />
      {sdk && copyable(t`Browser SDK example`, `await NotifuseAnalytics.trackEvent(${JSON.stringify(sdk, null, 2)});`)}
    </>}
    {definition.kind === 'event_mapping' && <Alert type="info" title={t`The external JSON belongs inside event; mapping_key selects this mapping. Do not apply the mapping twice.`} />}
  </Space>
}
