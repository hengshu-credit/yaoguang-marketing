import { createContext, useContext, useState, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { App, Empty, Skeleton, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useWorkspacePermissions } from '../../contexts/AuthContext'
import { customerApi, customerQueryKeys, type CustomerUpdatePatch } from '../../services/api/customer'
import { ApiError } from '../../services/api/errors'
import { CustomerProfilePanel } from '../customers/CustomerProfilePanel'
import { CustomerJourneysTab } from '../customers/CustomerJourneysTab'
import { JourneyTraceDrawer } from '../automations/JourneyTraceDrawer'
import { ActionableError } from '../errors/ActionableError'
import { ContactDeliveriesTab } from './ContactDeliveriesTab'

function useCustomerDetails(workspaceId: string, email: string, onUpdated?: () => void) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const client = useQueryClient()
  const { permissions, loading } = useWorkspacePermissions(workspaceId)
  const canRead = Boolean(permissions?.customers?.read)
  const query = useQuery({
    queryKey: customerQueryKeys.byEmail(workspaceId, email),
    queryFn: () => customerApi.getByEmail(workspaceId, email),
    enabled: canRead,
    retry: false
  })
  const update = useMutation({
    mutationFn: (patch: CustomerUpdatePatch) => {
      if (!permissions?.customers?.write || !query.data) throw new Error(t`Unable to update customer profile`)
      return customerApi.update(workspaceId, query.data.customer_id, patch, crypto.randomUUID())
    },
    onSuccess: async () => {
      onUpdated?.()
      await Promise.all([
        client.invalidateQueries({ queryKey: customerQueryKeys.all(workspaceId) }),
        client.invalidateQueries({ queryKey: ['contacts', workspaceId] }),
        client.invalidateQueries({ queryKey: ['contact_details', workspaceId, email] }),
        client.invalidateQueries({ queryKey: ['contact_timeline', workspaceId, email] })
      ])
      message.success(t`Customer profile updated`)
    },
    onError: (error) => message.error(error.message)
  })
  return { workspaceId, permissions, loading, canRead, query, update }
}

const CustomerDetailsContext = createContext<ReturnType<typeof useCustomerDetails> | null>(null)

export function ContactCustomerProvider({ workspaceId, email, children, onUpdated }: {
  workspaceId: string
  email: string
  children: ReactNode
  onUpdated?: () => void
}) {
  const details = useCustomerDetails(workspaceId, email, onUpdated)
  return <CustomerDetailsContext.Provider value={details}>{children}</CustomerDetailsContext.Provider>
}

function CustomerData({ children }: { children: ReactNode }) {
  const { t } = useLingui()
  const details = useContext(CustomerDetailsContext)!
  if (details.loading || details.query.isLoading) return <Skeleton active paragraph={{ rows: 2 }} />
  if (!details.canRead) return <Typography.Text type="secondary">{t`Customer read permission is required`}</Typography.Text>
  if (details.query.error instanceof ApiError && details.query.error.status === 404) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t`No customer is linked to this contact`} />
  }
  if (details.query.isError) return <ActionableError error={details.query.error} onRetry={() => void details.query.refetch()} retrying={details.query.isFetching} />
  if (!details.query.data) return null
  return children
}

export function ContactCustomerAttributes() {
  const details = useContext(CustomerDetailsContext)!
  const { query, update, permissions } = details
  return <div className="border-t border-gray-200 mt-3">
    <CustomerData>
      {query.data && <CustomerProfilePanel
        key={query.data.customer_id}
        customer={query.data}
        embedded
        canWrite={Boolean(permissions?.customers?.write)}
        saving={update.isPending}
        onUpdate={async (patch) => { await update.mutateAsync(patch) }}
      />}
    </CustomerData>
  </div>
}

export function ContactCustomerActivity({ sourceType, active }: {
  sourceType: 'automation' | 'broadcast'
  active: boolean
}) {
  const { t } = useLingui()
  const { workspaceId, query, permissions } = useContext(CustomerDetailsContext)!
  const [traceId, setTraceId] = useState<string>()
  const customerId = query.data?.customer_id
  const canReadJourneys = Boolean(permissions?.automations?.read)
  const canReadDeliveries = Boolean(permissions?.message_history?.read)
  return <CustomerData>
    {customerId && <div className="pt-4">
      {sourceType === 'automation' && canReadJourneys && <>
        <Typography.Title level={5}>{t`Automation Journeys`}</Typography.Title>
        <CustomerJourneysTab key={customerId} workspaceId={workspaceId} customerId={customerId} active={active} onViewTrace={setTraceId} />
        <JourneyTraceDrawer workspaceId={workspaceId} journeyInstanceId={traceId} open={active && Boolean(traceId)} onClose={() => setTraceId(undefined)} />
      </>}
      {canReadDeliveries ? <>
        {sourceType === 'automation' && <Typography.Title level={5} className="mt-5">{t`Deliveries`}</Typography.Title>}
        <ContactDeliveriesTab key={customerId} workspaceId={workspaceId} customerId={customerId} sourceType={sourceType} active={active} canReadCampaigns={Boolean(permissions?.broadcasts?.read)} />
      </> : <Typography.Text type="secondary">{t`Message history read permission is required to view deliveries`}</Typography.Text>}
    </div>}
  </CustomerData>
}
