import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Empty, Pagination, Skeleton, Space, Tag, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { deliveryApi, type DeliveryIntent } from '../../services/api/delivery'
import { broadcastApi } from '../../services/api/broadcast'
import { deliveryExplanation } from '../delivery/deliveryPresentation'
import { ActionableError } from '../errors/ActionableError'

function CampaignName({ workspaceId, intent, active }: { workspaceId: string; intent: DeliveryIntent; active: boolean }) {
  const { t } = useLingui()
  const query = useQuery({
    queryKey: ['contact-campaign', workspaceId, intent.source_id],
    queryFn: () => broadcastApi.get({ workspace_id: workspaceId, id: intent.source_id }),
    enabled: active,
    staleTime: 60000,
    retry: false
  })
  return <Typography.Text strong>{query.data?.broadcast?.name || `${t`Marketing Campaigns`} · ${intent.source_id}`}</Typography.Text>
}

export function ContactDeliveriesTab({ workspaceId, customerId, sourceType, active, canReadCampaigns }: {
  workspaceId: string
  customerId: string
  sourceType: 'automation' | 'broadcast'
  active: boolean
  canReadCampaigns: boolean
}) {
  const { t } = useLingui()
  const [page, setPage] = useState(1)
  const statusLabels = {
    planned: t`Planned`, reserved: t`Reserved`, queued: t`Queued`,
    submitting: t`Submitting to provider`, provider_accepted: t`Accepted by provider`,
    confirmed: t`Confirmed`, suppressed: t`Suppressed`, deferred: t`Deferred`,
    transient_failed: t`Will retry`, terminal_failed: t`Failed permanently`,
    unknown: t`Needs confirmation`, cancelled: t`Cancelled`
  }
  const pageSize = 20
  const query = useQuery({
    queryKey: ['contact-deliveries', workspaceId, customerId, sourceType, page],
    queryFn: () => deliveryApi.list({
      workspace_id: workspaceId,
      customer_id: customerId,
      source_type: sourceType,
      limit: pageSize,
      offset: (page - 1) * pageSize
    }),
    enabled: active
  })
  if (query.isError) return <ActionableError error={query.error} onRetry={() => void query.refetch()} retrying={query.isFetching} />
  if (query.isLoading) return <Skeleton active paragraph={{ rows: 3 }} />
  const deliveries = query.data?.deliveries ?? []
  return <>
    {deliveries.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t`No deliveries for this customer`} /> :
      <ul className="m-0 list-none p-0">
        {deliveries.map((intent) => <li key={intent.id} className="border-b border-gray-200 py-3">
          <Space orientation="vertical" size={4} style={{ width: '100%' }}>
            {sourceType === 'broadcast' && <CampaignName workspaceId={workspaceId} intent={intent} active={active && canReadCampaigns} />}
            <Space wrap><Tag>{intent.channel.toUpperCase()}</Tag><Tag>{statusLabels[intent.status] || intent.status}</Tag></Space>
            {intent.suppression_reason && <Typography.Text>{deliveryExplanation(intent)}</Typography.Text>}
            <Typography.Text type="secondary">{new Date(intent.created_at).toLocaleString()}</Typography.Text>
          </Space>
        </li>)}
      </ul>}
    <Pagination className="mt-4" current={page} pageSize={pageSize} total={query.data?.total ?? 0} onChange={setPage} showSizeChanger={false} hideOnSinglePage />
  </>
}
