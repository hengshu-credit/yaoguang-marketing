import { useEffect, useRef, useState } from 'react'
import { Alert, Button, DatePicker, Select, Space, Statistic, Table, Typography } from 'antd'
import dayjs from 'dayjs'
import { useLingui } from '@lingui/react/macro'
import { useBusinessCatalogue } from '../business/useBusinessCatalogue'
import { subjectStatesApi, type SubjectStateCoverage } from '../../services/api/subject_states'

export function StateCoveragePanel({ workspaceId }: { workspaceId: string }) {
 const { t } = useLingui()
 const catalogue = useBusinessCatalogue(workspaceId)
 const [subjectType, setSubjectType] = useState('customer'), [keys, setKeys] = useState<string[]>([])
 const [asOf, setAsOf] = useState(dayjs()), [watermark, setWatermark] = useState(dayjs())
 const [result, setResult] = useState<SubjectStateCoverage>(), [busy, setBusy] = useState(false), [error, setError] = useState('')
 const request = useRef<AbortController | null>(null)
 useEffect(() => { const requests = request; return () => requests.current?.abort() }, [workspaceId])
 const states = catalogue.definitions.filter(d => d.kind === 'state' && d.settings.subject_type === subjectType)
 const invalidate = () => { request.current?.abort(); setResult(undefined); setBusy(false) }
 const query = async () => { request.current?.abort(); const controller = new AbortController(); request.current = controller; setBusy(true); setError(''); try { const data = await subjectStatesApi.coverage(workspaceId, subjectType, keys, asOf.toISOString(), watermark.toISOString(), controller.signal); if (!controller.signal.aborted) setResult(data) } catch { if (!controller.signal.aborted) setError(t`Unable to load state coverage.`) } finally { if (!controller.signal.aborted) setBusy(false) } }
 return <Space orientation="vertical" style={{ width: '100%' }}><Typography.Title level={3}>{t`State coverage and intersection`}</Typography.Title><Typography.Paragraph>{t`Independent states can overlap. Unknown includes existing subjects without state evidence; these counts are not funnel stages.`}</Typography.Paragraph>
  {(error || catalogue.error) && <Alert type="error" title={error || t`Unable to load the business catalogue.`} />}
  <Space wrap><Select aria-label={t`Coverage object`} value={subjectType} style={{ minWidth: 180 }} options={[{ value: 'customer', label: t`Customer` }, ...catalogue.definitions.filter(d => d.kind === 'object_type' && d.status === 'active').map(d => ({ value: d.key, label: d.name }))]} onChange={value => { invalidate(); setSubjectType(value); setKeys([]) }} /><Select aria-label={t`Selected states`} placeholder={t`Selected states`} mode="multiple" style={{ minWidth: 280 }} value={keys} options={states.map(d => ({ value: d.key, label: d.name }))} onChange={value => { invalidate(); setKeys(value) }} />
   <Space><Typography.Text>{t`State time`}</Typography.Text><DatePicker aria-label={t`State time`} showTime allowClear={false} value={asOf} onChange={value => { if (value) { invalidate(); setAsOf(value) } }} /></Space><Space><Typography.Text>{t`Evidence watermark`}</Typography.Text><DatePicker aria-label={t`Evidence watermark`} showTime allowClear={false} value={watermark} onChange={value => { if (value) { invalidate(); setWatermark(value) } }} /></Space><Button type="primary" disabled={!keys.length || asOf.isAfter(watermark)} loading={busy} onClick={query}>{t`Analyze state coverage`}</Button></Space>
  {result && <><Space wrap size="large"><Statistic title={t`Total subjects`} value={result.total} /><Statistic title={t`All selected states satisfied`} value={result.intersection} /><Statistic title={t`Any selected state satisfied`} value={result.union} /></Space><Table rowKey="state_key" pagination={false} dataSource={result.states} columns={[{ title: t`State`, dataIndex: 'state_key', render: key => states.find(s => s.key === key)?.name ?? key }, { title: t`Satisfied`, dataIndex: 'satisfied' }, { title: t`Not satisfied`, dataIndex: 'false' }, { title: t`Unknown`, dataIndex: 'unknown' }]} /></>}
 </Space>
}
