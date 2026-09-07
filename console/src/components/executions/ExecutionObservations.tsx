import { useQuery } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { Alert, Descriptions, Empty, Skeleton, Space, Table, Typography } from 'antd'
import { executionsApi, type ObservationMetric } from '../../services/api/executions'
import { ActionableError } from '../errors/ActionableError'
export function ExecutionObservations({ workspaceId, executionId }: { workspaceId: string; executionId: string }) {
 const { t } = useLingui()
 const query = useQuery({ queryKey: ['execution-observations', workspaceId, executionId], queryFn: () => executionsApi.observations(workspaceId, executionId) })
 if (query.isPending) return <Skeleton active />
 if (query.isError) return <ActionableError error={query.error} onRetry={() => void query.refetch()} />
 const result = query.data
 if (!result.plan) return <Empty description={t`This execution has no saved tracking plan`} />
 const columns = [
  { title: t`Tracked item`, dataIndex: 'key', render: (key: string, item: ObservationMetric) => <>{key}{item.goal && <Typography.Text type="secondary"> · {t`Goal`}</Typography.Text>}</> },
  { title: t`Observed occurrences`, render: (_: unknown, item: ObservationMetric) => item.observed.occurrences },
  { title: t`Distinct customers`, render: (_: unknown, item: ObservationMetric) => item.observed.customers },
  { title: t`Distinct configured objects`, render: (_: unknown, item: ObservationMetric) => item.observed.objects },
  { title: t`Attributed occurrences`, render: (_: unknown, item: ObservationMetric) => item.attributed.occurrences }
 ]
 return <Space orientation="vertical" style={{ width: '100%' }}>
  <Alert type="info" showIcon title={t`Observed behavior and attributed conversions are separate`} description={t`Observed counts use this execution's selected customers and bound business objects. Attribution requires a verified touchpoint or eligible click; event timing alone is not attribution.`} />
  <Descriptions column={1} size="small" items={[{ key: 'cohort', label: t`Execution cohort customers`, children: result.cohort_customers }, { key: 'object', label: t`Configured object type`, children: result.plan.subject_type }, { key: 'window', label: t`Observation window in seconds`, children: result.plan.window_seconds }, { key: 'watermark', label: t`Data received through`, children: new Date(result.watermark).toLocaleString() }]} />
  <Typography.Title level={5}>{t`Tracked events and goals`}</Typography.Title>
  <Table size="small" rowKey="key" dataSource={result.events} columns={columns} pagination={false} expandable={{ expandedRowRender: (item) => <Typography.Paragraph copyable>{item.event_ids.join('\n')}</Typography.Paragraph> }} />
  <Typography.Title level={5}>{t`Tracked state observations`}</Typography.Title>
  <Typography.Paragraph type="secondary">{t`State counts describe true state intervals overlapping the execution window. They do not establish conversion attribution.`}</Typography.Paragraph>
  <Table size="small" rowKey="key" dataSource={result.states} columns={columns.slice(0, 4)} pagination={false} />
 </Space>
}
