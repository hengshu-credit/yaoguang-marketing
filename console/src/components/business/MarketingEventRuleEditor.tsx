import { Alert, Button, Checkbox, Form, InputNumber, Select } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { MarketingEventRule } from '../../services/api/marketing_tracking'
import { useBusinessCatalogue } from './useBusinessCatalogue'
import { StateRuleEditor } from '../business_states/StateRuleEditor'

export function MarketingEventRuleEditor({ workspaceId, value, onChange, disabled = false }: { workspaceId: string; value: MarketingEventRule; onChange: (value: MarketingEventRule) => void; disabled?: boolean }) {
  const { t } = useLingui()
  const catalogue = useBusinessCatalogue(workspaceId)
  const objects = catalogue.definitions.filter((item) => item.kind === 'object_type' && item.status === 'active')
  const events = catalogue.definitions.filter((item) => item.kind === 'event' && item.status === 'active' && (value.subject_type === 'customer' || item.settings.subject_type === value.subject_type))
  const states = catalogue.definitions.filter((item) => item.kind === 'state' && item.status === 'active' && item.settings.subject_type === value.subject_type)
  const set = (patch: Partial<MarketingEventRule>) => onChange({ ...value, ...patch })
  return <div className="space-y-3">
    {catalogue.error && <Alert type="error" title={t`Failed to load related definitions`} />}
    <Form.Item label={t`Trigger object`} required><Select aria-label={t`Trigger object`} disabled={disabled} value={value.subject_type} options={[{ value: 'customer', label: t`Customer` }, ...objects.map((item) => ({ value: item.key, label: catalogue.displayName(item) }))]} onChange={(subject_type) => set({ subject_type, events: [], condition: undefined })} /></Form.Item>
    <Form.Item label={t`Trigger events`} required extra={t`Any selected event can trigger entry. Each occurrence has its own report ID.`}><Select mode="multiple" aria-label={t`Trigger events`} disabled={disabled} value={value.events} options={events.map((item) => ({ value: item.key, label: catalogue.displayName(item) }))} onChange={(events) => set({ events })} /></Form.Item>
    <Checkbox disabled={disabled} checked={value.once_per_object ?? false} onChange={(event) => set({ once_per_object: event.target.checked })}>{t`Enter only once per selected object`}</Checkbox>
    <Form.Item label={t`Maximum chain depth`}><InputNumber aria-label={t`Maximum chain depth`} min={1} max={32} precision={0} value={value.max_causation_depth ?? 8} disabled={disabled} onChange={(max_causation_depth) => set({ max_causation_depth: max_causation_depth ?? 8 })} /></Form.Item>
    <Form.Item label={t`Excluded event sources`}><Select mode="tags" aria-label={t`Excluded event sources`} disabled={disabled} value={value.excluded_sources ?? []} onChange={(excluded_sources) => set({ excluded_sources })} /></Form.Item>
    {value.condition ? <><StateRuleEditor value={value.condition} onChange={(condition) => set({ condition })} objects={objects} events={events.map(item => ({ key: item.key, name: catalogue.displayName(item) }))} states={states} readOnly={disabled} /><Button disabled={disabled} onClick={() => set({ condition: undefined })}>{t`Remove event condition`}</Button></> : <Button disabled={disabled} onClick={() => set({ condition: { kind: 'attribute', field: '', operator: 'eq', operand: '' } })}>{t`Add event condition`}</Button>}
  </div>
}
