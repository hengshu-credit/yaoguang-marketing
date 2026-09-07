import { Alert, Button, Form, InputNumber, Select } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { BusinessConditionNodeConfig } from '../../../services/api/marketing_tracking'
import { StateRuleEditor } from '../../business_states/StateRuleEditor'
import { useBusinessCatalogue } from '../../business/useBusinessCatalogue'

export function BusinessConditionConfigForm({ workspaceId, value, onChange }: { workspaceId: string; value: BusinessConditionNodeConfig; onChange: (value: BusinessConditionNodeConfig) => void }) {
  const { t } = useLingui()
  const catalogue = useBusinessCatalogue(workspaceId)
  const objects = catalogue.definitions.filter((d) => d.kind === 'object_type' && d.status === 'active')
  const events = catalogue.definitions.filter((d) => d.kind === 'event' && d.status === 'active' && (value.subject_type === 'customer' || d.settings.subject_type === value.subject_type))
  const states = catalogue.definitions.filter((d) => d.kind === 'state' && d.status === 'active')
  const update = (patch: Partial<BusinessConditionNodeConfig>) => onChange({ ...value, ...patch })
  return <div className="space-y-4">
    <Alert type="info" showIcon title={t`Match the enrolled customer or business object`} description={t`Waiting only accepts new events for the same object. State conditions check the currently effective state. Expired and backfilled events do not advance the journey.`} />
    {catalogue.error && <Alert type="error" title={t`Failed to load related definitions`} />}
    <Form.Item label={t`When the condition is checked`}><Select aria-label={t`When the condition is checked`} value={value.mode} onChange={(mode) => update({ mode })} options={[{ value: 'wait', label: t`Wait until matched` }, { value: 'exit_if', label: t`Exit when matched` }, { value: 'continue_if', label: t`Continue only when matched` }]} /></Form.Item>
    <Form.Item label={t`Business object type`}><Select value={value.subject_type} onChange={(subject_type) => update({ subject_type, events: [], condition: undefined })} options={[{ value: 'customer', label: t`Customer` }, ...objects.map((d) => ({ value: d.key, label: catalogue.displayName(d) }))]} /></Form.Item>
    <Form.Item label={t`Matching events`}><Select mode="multiple" aria-label={t`Matching events`} value={value.events} onChange={(events) => update({ events })} options={events.map((d) => ({ value: d.key, label: catalogue.displayName(d) }))} /></Form.Item>
    <Form.Item label={t`Additional condition`}>{value.condition ? <><StateRuleEditor value={value.condition} onChange={(condition) => update({ condition })} objects={objects} events={events.map(item => ({ key: item.key, name: catalogue.displayName(item) }))} states={states} /><Button onClick={() => update({ condition: undefined })}>{t`Remove condition`}</Button></> : <Button onClick={() => update({ condition: { kind: 'state', state_key: '' } })}>{t`Add condition`}</Button>}</Form.Item>
    {value.mode === 'wait' && <><Form.Item label={t`Wait timeout in seconds`}><InputNumber min={1} max={31536000} precision={0} value={value.timeout_seconds} onChange={(timeout_seconds) => update({ timeout_seconds: timeout_seconds ?? 1 })} /></Form.Item><Form.Item label={t`When the wait expires`}><Select value={value.on_timeout} onChange={(on_timeout) => update({ on_timeout })} options={[{ value: 'exit', label: t`Exit the journey` }, { value: 'continue', label: t`Continue to the next node` }]} /></Form.Item></>}
  </div>
}
