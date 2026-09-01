import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useParams } from '@tanstack/react-router'
import { Alert, Button, Card, Descriptions, Drawer, Empty, Form, Input, Modal, Pagination, Select, Space, Table, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useLingui } from '@lingui/react/macro'
import { deliveryApi, type DeliveryDetail, type DeliveryIntent, type DeliveryListRequest, type DeliveryStatus } from '../services/api/delivery'
import { deliveryExplanation } from '../components/delivery/deliveryPresentation'
import { DeliveryFiltersBar } from '../components/delivery/DeliveryFiltersBar'
import { deliveryStatusOptions, type DeliveryFilters } from '../components/delivery/deliveryFilters'
import { CustomerDrawer } from '../components/customers/CustomerDrawer'
import { ActionableError } from '../components/errors/ActionableError'
import { useWorkspacePermissions } from '../contexts/AuthContext'
import { WorkspacePageTitle } from '../components/navigation/WorkspacePageTitle'

const PAGE_SIZE = 50
interface ResolutionForm {
  action: 'mark_confirmed' | 'mark_terminal_failed' | 'retry_after_verified_not_accepted'
  reason: string
}

export function DeliveryCenterPage() {
  const { t } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId' })
  const { permissions } = useWorkspacePermissions(workspaceId)
  const canWrite = Boolean(permissions?.message_history?.write)
  const queryClient = useQueryClient()
  const [filters, setFilters] = useState<DeliveryFilters>(() => {
    const search = new URLSearchParams(window.location.search)
    const status = search.get('status')
    const initial: DeliveryFilters = status === 'all'
      ? {}
      : { status: deliveryStatusOptions.includes(status as DeliveryStatus) ? status as DeliveryStatus : 'unknown' }

    for (const key of ['channel', 'source_type', 'source_id', 'provider', 'customer_id', 'from', 'to'] as const) {
      const value = search.get(key)?.trim()
      if (value) initial[key] = value
    }
    return initial
  })
  const [page, setPage] = useState(1)
  const [selectedIntentId, setSelectedIntentId] = useState<string>()
  const [selectedCustomerId, setSelectedCustomerId] = useState<string | null>(null)
  const [resolutionOpen, setResolutionOpen] = useState(false)
  const [resolutionForm] = Form.useForm<ResolutionForm>()

  const statusLabels = useMemo<Record<DeliveryStatus, string>>(() => ({
    planned: t`Planned`,
    reserved: t`Reserved`,
    queued: t`Queued`,
    submitting: t`Submitting to provider`,
    provider_accepted: t`Accepted by provider`,
    confirmed: t`Confirmed`,
    suppressed: t`Suppressed`,
    deferred: t`Deferred`,
    transient_failed: t`Will retry`,
    terminal_failed: t`Failed permanently`,
    unknown: t`Needs confirmation`,
    cancelled: t`Cancelled`
  }), [t])

  useEffect(() => {
    const search = new URLSearchParams(window.location.search)
    for (const key of ['status', 'channel', 'source_type', 'source_id', 'provider', 'customer_id', 'from', 'to']) {
      search.delete(key)
    }

    search.set('status', filters.status ?? 'all')
    for (const key of ['channel', 'source_type', 'source_id', 'provider', 'customer_id', 'from', 'to'] as const) {
      if (filters[key]) search.set(key, filters[key]!)
    }

    const query = search.toString()
    const url = `${window.location.pathname}${query ? `?${query}` : ''}${window.location.hash}`
    window.history.replaceState(window.history.state, '', url)
  }, [filters])

  const request = useMemo<DeliveryListRequest>(() => {
    const toISOString = (value?: string) => {
      if (!value) return undefined
      const date = new Date(value)
      return Number.isNaN(date.getTime()) ? undefined : date.toISOString()
    }

    return {
      workspace_id: workspaceId,
      ...filters,
      provider: filters.provider?.trim() || undefined,
      customer_id: filters.customer_id?.trim() || undefined,
      source_id: filters.source_id?.trim() || undefined,
      from: toISOString(filters.from),
      to: toISOString(filters.to),
      limit: PAGE_SIZE,
      offset: (page - 1) * PAGE_SIZE
    }
  }, [workspaceId, filters, page])
  const listQuery = useQuery({
    queryKey: ['delivery-center', request],
    queryFn: () => deliveryApi.list(request)
  })
  const detailQuery = useQuery({
    queryKey: ['delivery-detail', workspaceId, selectedIntentId],
    queryFn: () => deliveryApi.get(workspaceId, selectedIntentId!),
    enabled: Boolean(selectedIntentId)
  })
  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: ['delivery-center'] })
    await queryClient.invalidateQueries({ queryKey: ['delivery-detail', workspaceId, selectedIntentId] })
  }
  const reconcileMutation = useMutation({ mutationFn: (intentId: string) => deliveryApi.reconcile(workspaceId, intentId), onSuccess: refresh })
  const resolveMutation = useMutation({
    mutationFn: (values: ResolutionForm) => deliveryApi.resolveUnknown(workspaceId, selectedIntentId!, values.action, values.reason.trim()),
    onSuccess: async () => {
      setResolutionOpen(false)
      resolutionForm.resetFields()
      await refresh()
    }
  })

  const columns: ColumnsType<DeliveryIntent> = [
    { title: t`Status`, dataIndex: 'status', width: 150, render: (status: DeliveryStatus) => <Tag color={status === 'unknown' ? 'orange' : status === 'terminal_failed' ? 'red' : undefined}>{statusLabels[status]}</Tag> },
    { title: t`Channel`, dataIndex: 'channel', width: 100, render: (channel: string) => channel.toUpperCase() },
    { title: t`Source`, width: 190, render: (_, intent) => <Typography.Text>{intent.source_type} / {intent.source_id}</Typography.Text> },
    { title: t`Why`, render: (_, intent) => deliveryExplanation(intent) },
    { title: t`Created`, dataIndex: 'created_at', width: 180, render: (value: string) => new Date(value).toLocaleString() },
    { title: t`Action`, key: 'action', width: 110, render: (_, intent) => <Button onClick={() => setSelectedIntentId(intent.id)}>{t`Review`}</Button> }
  ]

  const detail: DeliveryDetail | undefined = detailQuery.data

  return (
    <Space orientation="vertical" size="large" style={{ width: '100%', minWidth: 0 }}>
      <div>
        <WorkspacePageTitle style={{ marginBottom: 4 }}>{t`Delivery Center`}</WorkspacePageTitle>
        <Typography.Text type="secondary">{t`Track every send decision, provider attempt and uncertain outcome in one place.`}</Typography.Text>
      </div>
      <Alert type="warning" showIcon title={t`Uncertain and permanently failed deliveries need attention`} description={t`An uncertain provider result is never retried automatically. Verify it first to avoid duplicate customer contact.`} />
      <DeliveryFiltersBar filters={filters} statusLabels={statusLabels} onChange={(nextFilters) => { setFilters(nextFilters); setPage(1) }} />

      {listQuery.isError && <ActionableError error={listQuery.error} onRetry={() => void listQuery.refetch()} retrying={listQuery.isFetching} />}
      <Table<DeliveryIntent>
        rowKey="id"
        columns={columns}
        dataSource={listQuery.data?.deliveries ?? []}
        loading={listQuery.isLoading || listQuery.isFetching}
        pagination={false}
        scroll={{ x: 980 }}
        locale={{ emptyText: <Empty description={t`No deliveries match these filters`} /> }}
      />
      <Pagination current={page} pageSize={PAGE_SIZE} total={listQuery.data?.total ?? 0} showSizeChanger={false} onChange={setPage} style={{ alignSelf: 'flex-end' }} />

      <Drawer title={t`Delivery details`} open={Boolean(selectedIntentId)} onClose={() => setSelectedIntentId(undefined)} size="large" destroyOnHidden>
        {detailQuery.isLoading && <Typography.Text>{t`Loading delivery details...`}</Typography.Text>}
        {detailQuery.isError && <ActionableError error={detailQuery.error} onRetry={() => void detailQuery.refetch()} retrying={detailQuery.isFetching} />}
        {reconcileMutation.isError && <ActionableError error={reconcileMutation.error} />}
        {resolveMutation.isError && <ActionableError error={resolveMutation.error} />}
        {detail && <Space orientation="vertical" size="large" style={{ width: '100%' }}>
          <Descriptions bordered size="small" column={1}>
            <Descriptions.Item label={t`Status`}><Tag>{statusLabels[detail.intent.status]}</Tag></Descriptions.Item>
            <Descriptions.Item label={t`Explanation`}>{deliveryExplanation(detail.intent)}</Descriptions.Item>
            <Descriptions.Item label={t`Delivery ID`}><Typography.Text copyable>{detail.intent.id}</Typography.Text></Descriptions.Item>
            <Descriptions.Item label={t`Customer`}>{detail.intent.customer_id ? <Button type="link" style={{ padding: 0 }} onClick={() => setSelectedCustomerId(detail.intent.customer_id!)}>{detail.intent.customer_id}</Button> : '—'}</Descriptions.Item>
          </Descriptions>
          <section><Typography.Title level={5}>{t`Provider attempts`}</Typography.Title>{detail.attempts.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} /> : detail.attempts.map((attempt, index) => <Card key={String(attempt.id ?? index)} size="small" style={{ marginBottom: 8 }}><Descriptions size="small" column={1}><Descriptions.Item label={t`Provider`}>{String(attempt.provider ?? '—')}</Descriptions.Item><Descriptions.Item label={t`Status`}>{String(attempt.status ?? '—')}</Descriptions.Item>{Boolean(attempt.error_detail) && <Descriptions.Item label={t`Reason`}>{String(attempt.error_detail)}</Descriptions.Item>}</Descriptions></Card>)}</section>
          <section><Typography.Title level={5}>{t`Reconciliation history`}</Typography.Title>{detail.reconciliations.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} /> : <pre>{JSON.stringify(detail.reconciliations, null, 2)}</pre>}</section>
          {detail.intent.status === 'unknown' && (canWrite ? <Space wrap>
            <Button onClick={() => reconcileMutation.mutate(detail.intent.id)} loading={reconcileMutation.isPending}>{t`Check provider again`}</Button>
            <Button type="primary" danger onClick={() => { resolutionForm.setFieldsValue({ action: 'mark_terminal_failed', reason: '' }); setResolutionOpen(true) }}>{t`Resolve status`}</Button>
          </Space> : <Alert type="info" showIcon title={t`Write permission is required to reconcile or resolve this delivery`} />)}
        </Space>}
      </Drawer>

      <Modal title={t`Resolve uncertain delivery`} open={resolutionOpen} onCancel={() => setResolutionOpen(false)} okText={t`Confirm resolution`} confirmLoading={resolveMutation.isPending} onOk={() => resolutionForm.submit()}>
        <Form form={resolutionForm} layout="vertical" onFinish={(values) => resolveMutation.mutate(values)}>
          <Form.Item name="action" label={t`Verified outcome`} rules={[{ required: true }]}>
            <Select options={[
              { value: 'mark_confirmed', label: t`Provider confirmed delivery` },
              { value: 'mark_terminal_failed', label: t`Provider confirmed permanent failure` },
              { value: 'retry_after_verified_not_accepted', label: t`Provider confirmed it was not accepted; retry is safe` }
            ]} />
          </Form.Item>
          <Form.Item name="reason" label={t`Verification evidence`} rules={[{ required: true, min: 8, message: t`Please enter at least 8 characters` }]}>
            <Input.TextArea rows={4} placeholder={t`Describe the verification evidence and reason`} />
          </Form.Item>
        </Form>
      </Modal>
      <CustomerDrawer workspaceId={workspaceId} customerId={selectedCustomerId} open={Boolean(selectedCustomerId)} onClose={() => setSelectedCustomerId(null)} />
    </Space>
  )
}
