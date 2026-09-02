import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, App, Button, Drawer, Space, Spin, Tabs, Tag, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { customerApi, customerQueryKeys, type CustomerUpdatePatch } from '../../services/api/customer'
import { listsApi } from '../../services/api/list'
import { CustomerTimelineTab } from './CustomerTimelineTab'
import { CustomerJourneysTab } from './CustomerJourneysTab'
import { CustomerDeliveriesTab } from './CustomerDeliveriesTab'
import { CustomerProfilePanel } from './CustomerProfilePanel'
import { JourneyTraceDrawer } from '../automations/JourneyTraceDrawer'
import { ActionableError } from '../errors/ActionableError'
import { CustomerListMembershipModal } from './CustomerListMembershipModal'
import { CustomerAudienceMemberships } from './CustomerAudienceMemberships'

interface CustomerDrawerProps {
  workspaceId: string
  customerId: string | null
  open: boolean
  onClose: () => void
  canWrite?: boolean
}

export function CustomerDrawer({ workspaceId, customerId, open, onClose, canWrite = true }: CustomerDrawerProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState('summary')
  const [traceInstanceId, setTraceInstanceId] = useState<string>()
  const [membershipModalOpen, setMembershipModalOpen] = useState(false)
  const query = useQuery({
    queryKey: customerQueryKeys.detail(workspaceId, customerId ?? ''),
    queryFn: () => customerApi.get(workspaceId, customerId!),
    enabled: open && Boolean(customerId)
  })
  const listsQuery = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId }),
    enabled: open
  })
  const listNames = useMemo(
    () => new Map((listsQuery.data?.lists ?? []).map((list) => [list.id, list.name])),
    [listsQuery.data?.lists]
  )
  const updateMutation = useMutation({
    mutationFn: (patch: CustomerUpdatePatch) =>
      customerApi.update(workspaceId, customerId!, patch, crypto.randomUUID()),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: customerQueryKeys.detail(workspaceId, customerId ?? '') }),
        queryClient.invalidateQueries({ queryKey: customerQueryKeys.all(workspaceId) })
      ])
      message.success(t`Customer profile updated`)
    },
    onError: (error) => message.error(error instanceof Error ? error.message : t`Unable to update customer profile`)
  })
  const customer = query.data
  const resolvedFrom = customer?.resolved_from_customer_id

  return (
    <>
      <Drawer
        title={t`Customer 360`}
        open={open}
        onClose={onClose}
        size={1200}
        styles={{ wrapper: { maxWidth: '100vw' }, body: { padding: 0, overflow: 'hidden' } }}
        destroyOnHidden
      >
        {query.isLoading ? <div className="grid min-h-60 place-items-center"><Spin /></div> : null}
        {query.isError ? <div className="p-6"><ActionableError error={query.error} onRetry={() => void query.refetch()} retrying={query.isFetching} /></div> : null}
        {customer ? (
          <div data-testid="customer-360-layout" className="flex h-full min-h-0 flex-col md:flex-row">
            <CustomerProfilePanel
              customer={customer}
              canWrite={canWrite}
              saving={updateMutation.isPending}
              onUpdate={async (patch) => { await updateMutation.mutateAsync(patch) }}
            />

            <section className="min-w-0 flex-1 overflow-y-auto p-5 md:p-6">
              {(resolvedFrom || customer.merged_into_id) ? (
                <Alert className="mb-5" type="info" showIcon title={t`This customer was merged`} description={<Space orientation="vertical" size={0}><Typography.Text>{t`Showing the surviving customer profile.`}</Typography.Text><Typography.Text copyable>{resolvedFrom || customer.merged_into_id}</Typography.Text></Space>} />
              ) : null}

              <div className="grid grid-cols-1 gap-4">
                <CustomerAudienceMemberships
                  workspaceId={workspaceId}
                  customerId={customer.customer_id}
                  active={open}
                />
                <section className="rounded-lg border border-gray-200 p-4" aria-labelledby="customer-lists-title">
                  <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                    <Typography.Title id="customer-lists-title" level={5} className="m-0">{t`List memberships`}</Typography.Title>
                    {canWrite ? (
                      <Button size="small" onClick={() => setMembershipModalOpen(true)}>
                        {t`Adjust list memberships`}
                      </Button>
                    ) : null}
                  </div>
                  {(customer.list_memberships?.length ?? 0) > 0 ? <Space wrap>{customer.list_memberships!.map((membership) => <Tag key={membership.list_id} color={membership.status === 'active' ? 'blue' : 'default'}>{listNames.get(membership.list_id) || membership.list_id} · {membership.status}</Tag>)}</Space> : <Typography.Text type="secondary">{t`No list memberships`}</Typography.Text>}
                </section>
              </div>

              <Tabs
                className="mt-5"
                activeKey={activeTab}
                onChange={setActiveTab}
                items={[
                  { key: 'summary', label: t`Activity overview`, children: <Typography.Text type="secondary">{t`Open a tab to inspect this customer's timeline, journeys or deliveries.`}</Typography.Text> },
                  { key: 'timeline', label: t`Activity timeline`, children: <CustomerTimelineTab workspaceId={workspaceId} customerId={customer.customer_id} active={activeTab === 'timeline'} /> },
                  { key: 'journeys', label: t`Journeys`, children: <CustomerJourneysTab workspaceId={workspaceId} customerId={customer.customer_id} active={activeTab === 'journeys'} onViewTrace={setTraceInstanceId} /> },
                  { key: 'deliveries', label: t`Deliveries`, children: <CustomerDeliveriesTab workspaceId={workspaceId} customerId={customer.customer_id} active={activeTab === 'deliveries'} /> }
                ]}
              />
            </section>
          </div>
        ) : null}
      </Drawer>
      <CustomerListMembershipModal
        workspaceId={workspaceId}
        customerIds={customerId ? [customerId] : []}
        currentListIds={customer?.list_memberships?.map((membership) => membership.list_id)}
        open={open && membershipModalOpen}
        onClose={() => setMembershipModalOpen(false)}
        onSuccess={() => {
          setMembershipModalOpen(false)
          void Promise.all([
            queryClient.invalidateQueries({ queryKey: customerQueryKeys.detail(workspaceId, customerId ?? '') }),
            queryClient.invalidateQueries({ queryKey: customerQueryKeys.all(workspaceId) }),
            queryClient.invalidateQueries({ queryKey: ['lists', workspaceId] })
          ])
        }}
      />
      <JourneyTraceDrawer workspaceId={workspaceId} journeyInstanceId={traceInstanceId} open={Boolean(traceInstanceId)} onClose={() => setTraceInstanceId(undefined)} />
    </>
  )
}
