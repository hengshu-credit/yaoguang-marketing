import { AutoComplete, Button, ConfigProvider, Input, InputNumber, Select, Space, Typography } from 'antd'
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import type { StateRule, StateValue } from '../../services/api/funnels'
import type { CatalogueItem } from '../funnels/FunnelEditor'
import { normalizeStateRule } from './state-rule-utils'

export interface StateAttributeOption { key: string; name: string; type?: string }
type ScopedItem = CatalogueItem & { settings?: Record<string, unknown> }
interface Props { value: StateRule; onChange: (rule: StateRule) => void; objects: ScopedItem[]; events: CatalogueItem[]; states: ScopedItem[]; depth?: number; allowedKinds?: StateRule['kind'][]; readOnly?: boolean; relations?: ScopedItem[]; attributes?: StateAttributeOption[]; objectAttributes?: Record<string, StateAttributeOption[]>; subjectType?: string }

export function StateRuleEditor({ value, onChange, objects, events, states, depth = 0, allowedKinds, readOnly = false, relations = [], attributes = [], objectAttributes = {}, subjectType }: Props) {
 const { t } = useLingui()
 const set = (patch: Partial<StateRule>) => { if (!readOnly) onChange(normalizeStateRule({ ...value, ...patch })) }
 const kinds = [{ value: 'event', label: t`Event occurred` }, { value: 'attribute', label: t`Attribute condition` }, { value: 'state', label: t`State condition` }, { value: 'object_any', label: t`Any related object` }, { value: 'all', label: t`All conditions` }, { value: 'any', label: t`Any condition` }, { value: 'not', label: t`Not condition` }].filter(item => !allowedKinds || allowedKinds.includes(item.value as StateRule['kind']))
 const grouped = ['all', 'any', 'not', 'object_any'].includes(value.kind)
 const multiple = value.kind === 'all' || value.kind === 'any'
 const newLeaf = (): StateRule => allowedKinds && !allowedKinds.includes('event') ? { kind: 'attribute', field: '', operator: 'eq', operand: '' } : { kind: 'event', event: '', minimum: 1 }
 const changeKind = (kind: StateRule['kind']) => {
  if (readOnly) return
  if (['all', 'any', 'not', 'object_any'].includes(kind)) onChange({ kind, children: grouped ? (['not', 'object_any'].includes(kind) ? [value.children?.[0] ?? newLeaf()] : value.children) : [value] })
  else onChange({ kind, ...(kind === 'attribute' ? { operator: 'eq', operand: '' } : kind === 'state' ? { value: 'true' as StateValue } : { minimum: 1 }) })
 }
 const selectedField = attributes.find(field => field.key === value.field)
 const type = selectedField?.type ?? 'auto'
 const scopedStates = states.filter(state => !subjectType || !state.settings?.subject_type || state.settings.subject_type === subjectType)
 const scopedRelations = relations.filter(relation => !subjectType || !relation.settings || (relation.settings.from_type === subjectType && (!value.subject_type || relation.settings.to_type === value.subject_type)))
 const valueEditor = type === 'number' || type === 'integer' ? <InputNumber aria-label={t`Comparison value`} style={{ width: '100%' }} precision={type === 'integer' ? 0 : undefined} value={typeof value.operand === 'number' ? value.operand : undefined} onChange={operand => set({ operand })} />
  : type === 'boolean' ? <Select aria-label={t`Comparison value`} className="w-full" value={typeof value.operand === 'boolean' ? String(value.operand) : undefined} options={[{ value: 'true', label: t`Yes` }, { value: 'false', label: t`No` }]} onChange={operand => set({ operand: operand === 'true' })} />
  : <Input aria-label={t`Comparison value`} value={typeof value.operand === 'string' ? value.operand : value.operand === undefined ? '' : JSON.stringify(value.operand)} placeholder={['json', 'object', 'array', 'auto'].includes(type) ? t`Value as JSON` : t`Value`} onChange={event => { let operand: unknown = event.target.value; if (['json', 'object', 'array', 'auto'].includes(type)) { try { operand = JSON.parse(event.target.value) } catch { /* Preserve the editable draft. */ } }; set({ operand }) }} />
 return <ConfigProvider componentDisabled={readOnly}><div className="rounded-lg border border-slate-200 bg-white p-4" data-condition-depth={depth}>
  <div className="mb-3 flex flex-wrap items-center gap-3">
   {multiple ? <Typography.Text strong>{t`Condition group`}</Typography.Text> : <Select aria-label={t`Rule type`} value={value.kind} style={{ minWidth: 170 }} options={kinds} onChange={changeKind} />}
   {multiple && <Space.Compact><Button disabled={readOnly || (!!allowedKinds && !allowedKinds.includes('all'))} type={value.kind === 'all' ? 'primary' : 'default'} onClick={() => set({ kind: 'all' })}>{t`AND`}</Button><Button disabled={readOnly || (!!allowedKinds && !allowedKinds.includes('any'))} type={value.kind === 'any' ? 'primary' : 'default'} onClick={() => set({ kind: 'any' })}>{t`OR`}</Button></Space.Compact>}
   {multiple && <Typography.Text type="secondary">{value.kind === 'all' ? t`Every condition below must match` : t`At least one condition below must match`}</Typography.Text>}
  </div>
  {value.kind === 'event' && <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
   <label>{t`Event`}<Select showSearch optionFilterProp="label" className="w-full mt-1" aria-label={t`Event`} value={value.event || undefined} options={events.map(event => ({ value: event.key, label: event.name }))} onChange={event => set({ event })} /></label>
   <label>{t`Minimum occurrences`}<InputNumber aria-label={t`Minimum occurrences`} className="w-full mt-1" min={1} precision={0} value={value.minimum ?? 1} onChange={minimum => set({ minimum: minimum ?? 1 })} /></label>
   <label>{t`Window in seconds`}<InputNumber aria-label={t`Window in seconds`} className="w-full mt-1" min={0} precision={0} value={value.window_seconds ?? 0} onChange={seconds => set({ window_seconds: seconds ?? 0 })} /><small className="text-gray-500">{t`Zero means no lookback limit`}</small></label>
   <label>{t`Revocation event`}<Select allowClear className="w-full mt-1" aria-label={t`Revocation event`} value={value.revoke_event} options={events.map(event => ({ value: event.key, label: event.name }))} onChange={revoke_event => set({ revoke_event })} /></label>
  </div>}
  {value.kind === 'attribute' && <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
   <label>{t`Field`}<AutoComplete className="w-full mt-1" aria-label={t`Attribute path`} value={value.field} options={attributes.map(field => ({ value: field.key, label: `${field.name} (${field.key})` }))} filterOption={(input, option) => String(option?.label).toLowerCase().includes(input.toLowerCase())} placeholder={t`Choose a field or enter its path`} onChange={field => set({ field, operator: value.operator ?? 'eq' })} />{selectedField && <small className="mt-1 block break-words text-gray-500">{selectedField.name}</small>}</label>
   <label>{t`Comparison`}<Select className="w-full mt-1" aria-label={t`Comparison`} value={value.operator ?? 'eq'} options={[{ value: 'eq', label: t`Equals` }, { value: 'neq', label: t`Does not equal` }, { value: 'gt', label: t`Greater than` }, { value: 'gte', label: t`At least` }, { value: 'lt', label: t`Less than` }, { value: 'lte', label: t`At most` }, { value: 'exists', label: t`Exists` }]} onChange={operator => set({ operator })} /></label>
   {value.operator !== 'exists' && <label>{t`Comparison value`}<div className="mt-1">{valueEditor}</div></label>}
  </div>}
  {value.kind === 'state' && <Space wrap><Select aria-label={t`State`} style={{ minWidth: 180 }} value={value.state_key} options={scopedStates.map(state => ({ value: state.key, label: state.name }))} onChange={state_key => set({ state_key, value: value.value ?? 'true' })} /><Select aria-label={t`State value`} value={value.value ?? 'true'} options={[{ value: 'true', label: t`Satisfied` }, { value: 'false', label: t`Not satisfied` }, { value: 'unknown', label: t`Unknown` }]} onChange={(next: StateValue) => set({ value: next })} /></Space>}
  {value.kind === 'object_any' && <Space wrap className="mb-3"><Select aria-label={t`Related object type`} placeholder={t`Related object type`} style={{ minWidth: 180 }} value={value.subject_type} options={objects.filter(object => object.key !== subjectType).map(object => ({ value: object.key, label: object.name }))} onChange={subject_type => set({ subject_type, relation_key: undefined })} />{subjectType !== 'customer' && <Select allowClear aria-label={t`Relation key`} placeholder={t`Any configured relation`} style={{ minWidth: 180 }} value={value.relation_key} options={scopedRelations.map(relation => ({ value: relation.key, label: relation.name }))} onChange={relation_key => set({ relation_key })} />}{subjectType === 'customer' && <Typography.Text type="secondary">{t`Objects are linked to customers by ownership.`}</Typography.Text>}{subjectType === 'customer' && value.relation_key && <Button onClick={() => set({ relation_key: undefined })}>{t`Use customer ownership`}</Button>}</Space>}
  {grouped && <div className="ml-2 space-y-3 border-l-2 border-violet-200 pl-4">
   {(value.children ?? []).map((child, index) => <div key={index} className="relative"><span className="absolute -left-4 top-6 w-4 border-t-2 border-violet-200" />
    <StateRuleEditor {...{ objects, events, states, allowedKinds, readOnly, relations, objectAttributes }} attributes={value.kind === 'object_any' ? objectAttributes[value.subject_type ?? ''] ?? [] : attributes} subjectType={value.kind === 'object_any' ? value.subject_type : subjectType} depth={depth + 1} value={child} onChange={rule => set({ children: value.children?.map((item, position) => position === index ? rule : item) })} />
    {multiple && <Button type="text" danger icon={<DeleteOutlined />} disabled={readOnly || (value.children?.length ?? 0) <= 1} onClick={() => set({ children: value.children?.filter((_, position) => position !== index) })}>{t`Remove condition`}</Button>}
   </div>)}
   {multiple && depth < 12 && <Space wrap><Button icon={<PlusOutlined />} onClick={() => set({ children: [...value.children ?? [], newLeaf()] })}>{t`Add condition`}</Button>{(!allowedKinds || allowedKinds.includes('all')) && <Button icon={<PlusOutlined />} onClick={() => set({ children: [...value.children ?? [], { kind: 'all', children: [newLeaf()] }] })}>{t`Add condition group`}</Button>}</Space>}
  </div>}
 </div></ConfigProvider>
}
