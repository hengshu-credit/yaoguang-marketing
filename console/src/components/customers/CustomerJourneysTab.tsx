import { useQuery } from '@tanstack/react-query'
import { Button, Empty, Skeleton, Space, Tag, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { automationApi } from '../../services/api/automation'
import { ActionableError } from '../errors/ActionableError'

interface CustomerJourneysTabProps {
  workspaceId: string
  customerId: string
  active: boolean
  onViewTrace: (instanceId: string) => void
}

export function CustomerJourneysTab({ workspaceId, customerId, active, onViewTrace }: CustomerJourneysTabProps) {
  const { t } = useLingui()
  const query = useQuery({
    queryKey: ['customer-360', workspaceId, customerId, 'journeys'],
    queryFn: () => automationApi.listJourneyInstances({ workspace_id: workspaceId, customer_id: customerId, limit: 50 }),
    enabled: active
  })
  if (query.isError) return <ActionableError error={query.error} onRetry={() => void query.refetch()} retrying={query.isFetching} />
  const instances = query.data?.instances ?? []
  if (query.isLoading) return <Skeleton active paragraph={{ rows: 3 }} />
  if (instances.length === 0) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t`This customer has not entered a journey`} />
  return <ul style={{ listStyle: 'none', margin: 0, padding: 0 }}>{instances.map((instance) => (
    <li key={instance.id} style={{ padding: '14px 0', borderBottom: '1px solid #f0f0f0' }}>
      <Space align="start" style={{ width: '100%', justifyContent: 'space-between' }}>
        <Space orientation="vertical" size={2}>
          <Space wrap><Typography.Text strong>{instance.automation_name}</Typography.Text><Tag>{instance.status}</Tag></Space>
          <Typography.Text type="secondary">{instance.waiting_reason || instance.entry_reason || t`Entered the journey`}</Typography.Text>
        </Space>
        <Button onClick={() => onViewTrace(instance.id)}>{t`View trace`}</Button>
      </Space>
    </li>
  ))}</ul>
}
