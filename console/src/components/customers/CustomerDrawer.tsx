import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Alert, Descriptions, Drawer, Empty, Space, Spin, Tabs, Tag, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { customerApi, customerQueryKeys, type Customer } from '../../services/api/customer'
import { CustomerTimelineTab } from './CustomerTimelineTab'
import { CustomerJourneysTab } from './CustomerJourneysTab'
import { CustomerDeliveriesTab } from './CustomerDeliveriesTab'
import { JourneyTraceDrawer } from '../automations/JourneyTraceDrawer'
import { ActionableError } from '../errors/ActionableError'

interface CustomerDrawerProps {
  workspaceId: string
  customerId: string | null
  open: boolean
  onClose: () => void
}

function CustomerOverview({ customer }: { customer: Customer }) {
  const { t } = useLingui()
  return (
    <Space orientation="vertical" size="large" style={{ width: '100%' }}>
      <Descriptions bordered size="small" column={1}>
        <Descriptions.Item label={t`Customer number`}><Typography.Text copyable>{customer.customer_no}</Typography.Text></Descriptions.Item>
        <Descriptions.Item label={t`Internal ID`}><Typography.Text copyable>{customer.customer_id}</Typography.Text></Descriptions.Item>
        <Descriptions.Item label={t`External user ID`}>{customer.external_user_id ? <Typography.Text copyable>{customer.external_user_id}</Typography.Text> : '—'}</Descriptions.Item>
        <Descriptions.Item label={t`Status`}>{customer.profile?.status ?? '—'}</Descriptions.Item>
        <Descriptions.Item label={t`Language`}>{customer.profile?.language ?? '—'}</Descriptions.Item>
        <Descriptions.Item label={t`Timezone`}>{customer.profile?.timezone ?? '—'}</Descriptions.Item>
      </Descriptions>
      <section aria-labelledby="customer-tags-title">
        <Typography.Title id="customer-tags-title" level={5}>{t`Tags`}</Typography.Title>
        {customer.tags.length > 0 ? <Space wrap>{customer.tags.map((tag) => <Tag key={tag}>{tag}</Tag>)}</Space> : <Typography.Text type="secondary">{t`No tags`}</Typography.Text>}
      </section>
      {customer.profile?.attributes && Object.keys(customer.profile.attributes).length > 0 && (
        <Descriptions title={t`Profile attributes`} bordered size="small" column={1}>
          {Object.entries(customer.profile.attributes).map(([key, value]) => (
            <Descriptions.Item key={key} label={key}>{typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean' ? String(value) : JSON.stringify(value)}</Descriptions.Item>
          ))}
        </Descriptions>
      )}
    </Space>
  )
}

function CustomerIdentities({ customer }: { customer: Customer }) {
  const { t } = useLingui()
  return (
    <Space orientation="vertical" size="large" style={{ width: '100%' }}>
      <section aria-labelledby="customer-identities-title">
        <Typography.Title id="customer-identities-title" level={5}>{t`Identity aliases`}</Typography.Title>
        {customer.identities.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t`No identity aliases`} /> : (
          <Space wrap>{customer.identities.map((identity) => <Tag key={identity.id} color={identity.primary ? 'blue' : undefined}>{identity.type}: {identity.display_hint}{identity.primary ? ` · ${t`Primary`}` : ''}</Tag>)}</Space>
        )}
      </section>
      <section aria-labelledby="customer-consents-title">
        <Typography.Title id="customer-consents-title" level={5}>{t`Consent`}</Typography.Title>
        {(customer.consents?.length ?? 0) > 0 ? <Space orientation="vertical">{customer.consents!.map((consent) => <Space key={consent.id} wrap><Typography.Text>{consent.purpose} / {consent.channel}</Typography.Text><Tag color={consent.status === 'granted' ? 'green' : 'default'}>{consent.status}</Tag></Space>)}</Space> : <Typography.Text type="secondary">{t`No consent records`}</Typography.Text>}
      </section>
    </Space>
  )
}

function CustomerAudiences({ customer }: { customer: Customer }) {
  const { t } = useLingui()
  const listMemberships = customer.list_memberships ?? []
  const audienceMemberships = customer.audience_memberships ?? []
  return (
    <Space orientation="vertical" size="large" style={{ width: '100%' }}>
      <section aria-labelledby="customer-lists-title">
        <Typography.Title id="customer-lists-title" level={5}>{t`List memberships`}</Typography.Title>
        {listMemberships.length > 0 ? <Space wrap>{listMemberships.map((membership) => <Tag key={membership.list_id} color="geekblue">{membership.list_id} · {membership.status}</Tag>)}</Space> : <Typography.Text type="secondary">{t`No list memberships`}</Typography.Text>}
      </section>
      <section aria-labelledby="customer-audiences-title">
        <Typography.Title id="customer-audiences-title" level={5}>{t`Dynamic audience memberships`}</Typography.Title>
        {audienceMemberships.length > 0 ? <Space wrap>{audienceMemberships.map((membership) => <Tag key={membership.audience_id} color="purple">{membership.name} · v{membership.audience_version}</Tag>)}</Space> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t`No active audience memberships`} />}
      </section>
    </Space>
  )
}

export function CustomerDrawer({ workspaceId, customerId, open, onClose }: CustomerDrawerProps) {
  const { t } = useLingui()
  const [activeTab, setActiveTab] = useState('overview')
  const [traceInstanceId, setTraceInstanceId] = useState<string>()
  const query = useQuery({
    queryKey: customerQueryKeys.detail(workspaceId, customerId ?? ''),
    queryFn: () => customerApi.get(workspaceId, customerId!),
    enabled: open && Boolean(customerId)
  })
  const customer = query.data
  const resolvedFrom = customer?.resolved_from_customer_id

  return (
    <>
      <Drawer title={t`Customer 360`} open={open} onClose={onClose} size="large" styles={{ wrapper: { maxWidth: '100vw' } }} destroyOnHidden>
        {query.isLoading && <div style={{ display: 'grid', placeItems: 'center', minHeight: 240 }}><Spin /></div>}
        {query.isError && <ActionableError error={query.error} onRetry={() => void query.refetch()} retrying={query.isFetching} />}
        {customer && (
          <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
            {(resolvedFrom || customer.merged_into_id) && <Alert type="info" showIcon title={t`This customer was merged`} description={<Space orientation="vertical" size={0}><Typography.Text>{t`Showing the surviving customer profile.`}</Typography.Text><Typography.Text copyable>{resolvedFrom || customer.merged_into_id}</Typography.Text></Space>} />}
            <Tabs
              activeKey={activeTab}
              onChange={setActiveTab}
              items={[
                { key: 'overview', label: t`Overview`, children: <CustomerOverview customer={customer} /> },
                { key: 'identities', label: t`Identities & Consent`, children: <CustomerIdentities customer={customer} /> },
                { key: 'audiences', label: t`Audiences`, children: <CustomerAudiences customer={customer} /> },
                { key: 'timeline', label: t`Activity timeline`, children: <CustomerTimelineTab workspaceId={workspaceId} customerId={customer.customer_id} active={activeTab === 'timeline'} /> },
                { key: 'journeys', label: t`Journeys`, children: <CustomerJourneysTab workspaceId={workspaceId} customerId={customer.customer_id} active={activeTab === 'journeys'} onViewTrace={setTraceInstanceId} /> },
                { key: 'deliveries', label: t`Deliveries`, children: <CustomerDeliveriesTab workspaceId={workspaceId} customerId={customer.customer_id} active={activeTab === 'deliveries'} /> }
              ]}
            />
          </Space>
        )}
      </Drawer>
      <JourneyTraceDrawer workspaceId={workspaceId} journeyInstanceId={traceInstanceId} open={Boolean(traceInstanceId)} onClose={() => setTraceInstanceId(undefined)} />
    </>
  )
}
