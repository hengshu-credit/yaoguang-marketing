import { Alert, Button, Form, InputNumber, Select, Space, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { MarketingTrackingPlan } from '../../services/api/marketing_tracking'
import { useBusinessCatalogue } from './useBusinessCatalogue'

export function MarketingTrackingPlanEditor({ workspaceId, value, onChange, disabled = false }: { workspaceId: string; value?: MarketingTrackingPlan | null; onChange?: (value: MarketingTrackingPlan | null) => void; disabled?: boolean }) {
  const { t } = useLingui()
  if (!value) return <Space orientation="vertical"><Typography.Paragraph type="secondary">{t`Choose the events, states and goals to observe for this task.`}</Typography.Paragraph><Button disabled={disabled} onClick={() => onChange?.({ subject_type: 'customer', events: [], states: [], goal_events: [], window_seconds: 604800 })}>{t`Configure conversion tracking`}</Button></Space>
  return <TrackingPlanFields workspaceId={workspaceId} value={value} onChange={onChange} disabled={disabled} />
}

function TrackingPlanFields({ workspaceId, value, onChange, disabled }: { workspaceId: string; value: MarketingTrackingPlan; onChange?: (value: MarketingTrackingPlan | null) => void; disabled: boolean }) {
  const { t } = useLingui()
  const catalogue = useBusinessCatalogue(workspaceId)
  const objects = catalogue.definitions.filter((item) => item.kind === 'object_type' && item.status === 'active')
  const events = catalogue.definitions.filter((item) => item.kind === 'event' && item.status === 'active' && (value.subject_type === 'customer' || item.settings.subject_type === value.subject_type))
  const states = catalogue.definitions.filter((item) => item.kind === 'state' && item.status === 'active' && item.settings.subject_type === value.subject_type)
  const set = (patch: Partial<MarketingTrackingPlan>) => onChange?.({ ...value, ...patch })
  return <div className="space-y-4">
    <Alert type="info" showIcon title={t`Tracking is specific to this task`} description={t`Other events continue to be collected. Repeated events keep their individual occurrences; customer and object counts use the selected unit.`} />
    {catalogue.error && <Alert type="error" title={t`Failed to load related definitions`} />}
    <Form.Item label={t`Tracking unit`}><Select aria-label={t`Tracking unit`} value={value.subject_type} disabled={disabled} loading={catalogue.loading} options={[{ value: 'customer', label: t`Customer` }, ...objects.map((item) => ({ value: item.key, label: catalogue.displayName(item) }))]} onChange={(subject_type) => set({ subject_type, events: [], states: [], goal_events: [] })} /></Form.Item>
    <Form.Item label={t`Tracked events`}><Select mode="multiple" aria-label={t`Tracked events`} disabled={disabled} value={value.events} options={events.map((item) => ({ value: item.key, label: catalogue.displayName(item) }))} onChange={(events) => set({ events })} /></Form.Item>
    <Form.Item label={t`Tracked states`}><Select mode="multiple" aria-label={t`Tracked states`} disabled={disabled} value={value.states} options={states.map((item) => ({ value: item.key, label: catalogue.displayName(item) }))} onChange={(states) => set({ states })} /></Form.Item>
    <Form.Item label={t`Goal events`}><Select mode="multiple" aria-label={t`Goal events`} disabled={disabled} value={value.goal_events ?? []} options={events.map((item) => ({ value: item.key, label: catalogue.displayName(item) }))} onChange={(goal_events) => set({ goal_events })} /></Form.Item>
    <Form.Item label={t`Observation window in seconds`}><InputNumber aria-label={t`Observation window in seconds`} min={1} max={31536000} precision={0} className="w-full" disabled={disabled} value={value.window_seconds} onChange={(window_seconds) => set({ window_seconds: window_seconds ?? 1 })} /></Form.Item>
    <Button disabled={disabled} onClick={() => onChange?.(null)}>{t`Remove task tracking`}</Button>
  </div>
}
