import { useQuery } from '@tanstack/react-query'
import { Empty, Skeleton, Space, Tag, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { deliveryApi } from '../../services/api/delivery'
import { deliveryExplanation, deliveryStatusLabels } from '../delivery/deliveryPresentation'
import { ActionableError } from '../errors/ActionableError'

interface CustomerDeliveriesTabProps {
  workspaceId: string
  customerId: string
  active: boolean
}

export function CustomerDeliveriesTab({ workspaceId, customerId, active }: CustomerDeliveriesTabProps) {
  const { t } = useLingui()
  const query = useQuery({
    queryKey: ['customer-360', workspaceId, customerId, 'deliveries'],
    queryFn: () => deliveryApi.list({ workspace_id: workspaceId, customer_id: customerId, limit: 50 }),
    enabled: active
  })
  if (query.isError) return <ActionableError error={query.error} onRetry={() => void query.refetch()} retrying={query.isFetching} />
  const deliveries = query.data?.deliveries ?? []
  if (query.isLoading) return <Skeleton active paragraph={{ rows: 3 }} />
  if (deliveries.length === 0) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t`No deliveries for this customer`} />
  return <ul style={{ listStyle: 'none', margin: 0, padding: 0 }}>{deliveries.map((intent) => (
    <li key={intent.id} style={{ padding: '14px 0', borderBottom: '1px solid #f0f0f0' }}>
      <Space orientation="vertical" size={2} style={{ width: '100%' }}>
        <Space wrap><Tag>{intent.channel.toUpperCase()}</Tag><Tag>{deliveryStatusLabels[intent.status]}</Tag></Space>
        <Typography.Text>{deliveryExplanation(intent)}</Typography.Text>
        <Typography.Text type="secondary">{new Date(intent.created_at).toLocaleString()}</Typography.Text>
      </Space>
    </li>
  ))}</ul>
}
