import { useQuery } from '@tanstack/react-query'
import { Alert, Button, Empty, Space, Spin, Timeline, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { contactTimelineApi } from '../../services/api/contact_timeline'

interface CustomerTimelineTabProps {
  workspaceId: string
  customerId: string
  active: boolean
}

export function CustomerTimelineTab({ workspaceId, customerId, active }: CustomerTimelineTabProps) {
  const { t } = useLingui()
  const query = useQuery({
    queryKey: ['customer-360', workspaceId, customerId, 'timeline'],
    queryFn: () => contactTimelineApi.list({ workspace_id: workspaceId, customer_id: customerId, limit: 50 }),
    enabled: active
  })
  if (query.isLoading) return <Spin />
  if (query.isError) {
    return <Alert type="error" showIcon title={t`Unable to load activity timeline`} action={<Button onClick={() => query.refetch()}>{t`Retry`}</Button>} />
  }
  const entries = query.data?.timeline ?? []
  if (entries.length === 0) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t`No customer activity yet`} />
  return (
    <Timeline
      items={entries.map((entry) => ({
        key: entry.id,
        content: (
          <Space orientation="vertical" size={0}>
            <Typography.Text strong>{entry.kind}</Typography.Text>
            <Typography.Text type="secondary">{new Date(entry.created_at).toLocaleString()}</Typography.Text>
          </Space>
        )
      }))}
    />
  )
}
