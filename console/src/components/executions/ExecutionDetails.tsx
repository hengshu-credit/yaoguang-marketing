import { useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { Alert, Button, Collapse, Descriptions, Drawer, Empty, Select, Skeleton, Space, Tabs, Timeline, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { executionsApi, type ExecutionEvidence } from '../../services/api/executions'
import { ActionableError } from '../errors/ActionableError'
import { ArchivedMediaScope } from './ArchivedMediaScope'
import { useArchivedMedia } from './executionMediaContext'
import { ExecutionObservations } from './ExecutionObservations'
import { executionSections, materialTemplates, record, safeContentPreview } from './executionPresentation'

function JSONValue({ value }: { value: unknown }) {
 return <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', background: '#fafafa', padding: 12, maxHeight: 440, overflow: 'auto' }}>{JSON.stringify(value, null, 2)}</pre>
}
function RecordedContent({ content }: { content: unknown }) {
 const { t } = useLingui()
 const archivedMedia = useArchivedMedia()
const fields = record(record(content).payload ?? content)
 const html = fields.html_content ?? fields.compiled_html ?? fields.compiled_preview ?? fields.html
 return <Space orientation="vertical" style={{ width: '100%' }}>
  {typeof fields.subject === 'string' && <Typography.Text strong>{fields.subject}</Typography.Text>}
  {typeof html === 'string' && <iframe title={t`Recorded content preview`} sandbox="" srcDoc={safeContentPreview(Object.entries(archivedMedia).reduce((content, [url, data]) => content.split(url).join(data).split(url.replace(/&/g, '&amp;')).join(data), html))} style={{ width: '100%', minHeight: 280, border: '1px solid #eee' }} />}
  <JSONValue value={content} />
 </Space>
}
function EvidenceItem({ evidence, workspaceId, executionId, media }: { evidence: ExecutionEvidence; workspaceId: string; executionId: string; media?: unknown }) {
 const { t } = useLingui()
 const statuses: Record<string, string> = { pending: t`Pending`, planned: t`Planned`, reserved: t`Reserved`, queued: t`Queued`, submitting: t`Submitting`, submitted: t`Submitted`, provider_accepted: t`Provider accepted`, confirmed: t`Confirmed`, processing: t`Processing`, completed: t`Completed`, failed: t`Failed`, unknown: t`Outcome unknown`, suppressed: t`Suppressed`, deferred: t`Deferred`, selected: t`Selected`, rendered: t`Rendered`, skipped: t`Skipped`, waiting: t`Waiting`, exited: t`Exited`, archived: t`Archived` }
 const kinds: Record<string, string> = { node: t`Node`, audience: t`Audience`, eligibility: t`Eligibility`, delivery_content: t`Delivery content`, delivery_status: t`Delivery status`, delivery_media: t`Archived media` }
 return <ArchivedMediaScope workspaceId={workspaceId} executionId={executionId} materials={media}><Collapse items={[{ key: evidence.id, label: <Space wrap><Typography.Text>{evidence.node_id || evidence.delivery_id || kinds[evidence.kind] || evidence.kind}</Typography.Text><Typography.Text type="secondary">{statuses[evidence.status] || evidence.status}</Typography.Text></Space>, children: <>
  <Descriptions size="small" column={1} items={[{ key: 'at', label: t`Recorded at`, children: new Date(evidence.occurred_at).toLocaleString() }, { key: 'customer', label: t`Customer`, children: evidence.customer_id || t`Not recorded` }]} />
  <RecordedContent content={evidence.payload} />
  {Array.isArray(record(evidence.payload).assets) && (record(evidence.payload).assets as unknown[]).map((asset, index) => <ArchivedAsset key={index} workspaceId={workspaceId} executionId={executionId} asset={record(asset)} />)}
 </> }]} /></ArchivedMediaScope>
}
export function ExecutionDetails({ workspaceId, executionId }: { workspaceId: string; executionId: string }) {
 const { t } = useLingui()
 const query = useInfiniteQuery({ queryKey: ['execution-details', workspaceId, executionId], initialPageParam: '', queryFn: ({ pageParam }) => executionsApi.get(workspaceId, executionId, pageParam), getNextPageParam: (page) => page.next || undefined })
 if (query.isPending) return <Skeleton active />
 if (query.isError) {
  const error = query.error as { status?: number; code?: string }
  if (error.status === 404 || error.code === 'historical_snapshot_missing') return <Alert type="warning" showIcon title={t`This historical execution has no configuration snapshot`} description={t`Current configuration cannot reconstruct missing historical content.`} />
  return <ActionableError error={query.error} onRetry={() => void query.refetch()} retrying={query.isFetching} />
 }
 const snapshot = query.data.pages[0].snapshot
 const evidence = query.data.pages.flatMap((page) => page.evidence)
 const templates = materialTemplates(snapshot.materials)
 const sections = executionSections(snapshot.configuration)
 const mediaByDelivery = new Map(evidence.filter((item) => item.kind === 'delivery_media').map((item) => [item.delivery_id, item.payload]))
 return <ArchivedMediaScope workspaceId={workspaceId} executionId={snapshot.execution_id} materials={snapshot.materials}><Space orientation="vertical" size="middle" style={{ width: '100%' }}>
  <Descriptions size="small" column={1} items={[{ key: 'run', label: t`Execution`, children: snapshot.execution_id }, { key: 'time', label: t`Started at`, children: new Date(snapshot.created_at).toLocaleString() }]} />
  {snapshot.completeness !== 'complete' && <Alert type="warning" showIcon title={t`Historical content is incomplete`} description={<><Typography.Paragraph>{t`Unavailable fields are listed below. Current templates are never substituted.`}</Typography.Paragraph><JSONValue value={snapshot.missing} /></>} />}
  <Tabs items={[
   { key: 'configuration', label: t`Configuration`, children: <><JSONValue value={snapshot.configuration} /><Collapse items={[{ key: 'dependencies', label: t`Dependency definitions`, children: <JSONValue value={snapshot.dependencies} /> }]} /></> },
   { key: 'audience', label: t`Filters and audience`, children: <><JSONValue value={sections.filters} />{evidence.filter((item) => item.kind === 'eligibility' || item.kind === 'audience').map((item) => <EvidenceItem key={item.id} evidence={item} workspaceId={workspaceId} executionId={snapshot.execution_id} media={mediaByDelivery.get(item.delivery_id)} />)}</> },
   { key: 'materials', label: t`Materials`, children: <><Alert type="info" title={t`Configured materials are not proof of sending`} style={{ marginBottom: 12 }} /><Collapse items={templates.map((template, index) => ({ key: String(index), label: String(template.name || template.id), children: <><RecordedContent content={template.email ?? template.content ?? template} /><Collapse items={[{ key: 'all', label: t`Full template and translations`, children: <JSONValue value={template} /> }]} /></> }))} />{Array.isArray(record(snapshot.materials).assets) && (record(snapshot.materials).assets as unknown[]).map((asset, index) => <ArchivedAsset key={index} workspaceId={workspaceId} executionId={snapshot.execution_id} asset={record(asset)} />)}</> },
   { key: 'timeline', label: t`Execution timeline`, children: evidence.length === 0 ? <Empty description={t`No execution evidence has been recorded yet`} /> : <><Timeline items={evidence.map((item) => ({ key: item.id, content: <EvidenceItem evidence={item} workspaceId={workspaceId} executionId={snapshot.execution_id} media={mediaByDelivery.get(item.delivery_id)} /> }))} />{query.hasNextPage && <Button loading={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>{t`Load more evidence`}</Button>}</> },
   { key: 'conversions', label: t`Conversion results`, children: <ExecutionObservations workspaceId={workspaceId} executionId={snapshot.execution_id} /> }
  ]} />
 </Space></ArchivedMediaScope>
}
export function ExecutionHistory({ workspaceId, sourceId }: { workspaceId: string; sourceId: string }) {
 const { t } = useLingui()
 const [selected, setSelected] = useState<string>()
 const query = useInfiniteQuery({ queryKey: ['execution-history', workspaceId, sourceId], initialPageParam: '', queryFn: ({ pageParam }) => executionsApi.list(workspaceId, sourceId, pageParam), getNextPageParam: (page) => page.next || undefined })
 if (query.isPending) return <Skeleton active />
 if (query.isError) return <ActionableError error={query.error} onRetry={() => void query.refetch()} retrying={query.isFetching} />
 const items = query.data.pages.flatMap((page) => page.items)
 if (items.length === 0) return <Empty description={t`No execution snapshots are available`} />
 const executionId = items.some((item) => item.execution_id === selected) ? selected! : items[0].execution_id
 return <Space orientation="vertical" style={{ width: '100%' }}><Select aria-label={t`Select execution`} value={executionId} onChange={setSelected} style={{ width: '100%' }} options={items.map((item) => ({ value: item.execution_id, label: `${new Date(item.created_at).toLocaleString()} · ${item.execution_id}` }))} />{query.hasNextPage && <Button loading={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>{t`Load older executions`}</Button>}<ExecutionDetails key={executionId} workspaceId={workspaceId} executionId={executionId} /></Space>
}
export function ExecutionHistoryButton({ workspaceId, sourceId }: { workspaceId: string; sourceId: string }) {
 const { t } = useLingui()
 const [open, setOpen] = useState(false)
 return <><Button onClick={() => setOpen(true)}>{t`Execution history`}</Button><Drawer title={t`Execution history`} size={880} open={open} onClose={() => setOpen(false)} destroyOnHidden>{open && <ExecutionHistory workspaceId={workspaceId} sourceId={sourceId} />}</Drawer></>
}


export function ArchivedAsset({ workspaceId, executionId, asset }: { workspaceId: string; executionId: string; asset: Record<string, unknown> }) {
 const { t } = useLingui()
 const [error, setError] = useState<unknown>()
 const [loading, setLoading] = useState(false)
 const download = async () => {
  setLoading(true)
  try {
   const blob = await executionsApi.asset(workspaceId, executionId, String(asset.hash))
   const objectUrl = URL.createObjectURL(blob)
   const link = document.createElement('a')
   link.href = objectUrl
   link.download = String(asset.source_url || asset.hash).split('/').pop()?.split('?')[0] || String(asset.hash)
   link.click()
   setTimeout(() => URL.revokeObjectURL(objectUrl), 1000)
  } catch (cause) { setError(cause) } finally { setLoading(false) }
 }
 return <Space orientation="vertical"><Typography.Text>{String(asset.source_url || asset.hash)}</Typography.Text><Button loading={loading} onClick={() => void download()}>{t`Download archived material`}</Button>{error !== undefined && <ActionableError error={error} onRetry={() => void download()} />}</Space>
}
