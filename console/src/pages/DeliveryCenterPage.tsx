import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useParams } from '@tanstack/react-router'
import { Alert, Button, Card, Descriptions, Drawer, Empty, Form, Input, Modal, Pagination, Select, Space, Table, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useLingui } from '@lingui/react/macro'
import { deliveryApi, type DeliveryDetail, type DeliveryIntent, type DeliveryListRequest, type DeliveryStatus } from '../services/api/delivery'
import { deliveryExplanation, deliveryStatusLabels } from '../components/delivery/deliveryPresentation'
import { CustomerDrawer } from '../components/customers/CustomerDrawer'
import { ActionableError } from '../components/errors/ActionableError'
import { useWorkspacePermissions } from '../contexts/AuthContext'

const PAGE_SIZE = 50
const statusOptions: DeliveryStatus[] = [
  'unknown', 'terminal_failed', 'transient_failed', 'suppressed', 'deferred', 'submitting',
  'provider_accepted', 'confirmed', 'queued', 'planned', 'reserved', 'cancelled'
]

interface DeliveryFilters {
  status?: DeliveryStatus
  channel?: string
  source_type?: string
  source_id?: string
  provider?: string
  customer_id?: string
  from?: string
  to?: string
}

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
  const [draft, setDraft] = useState<DeliveryFilters>({ status: 'unknown' })
  const [filters, setFilters] = useState<DeliveryFilters>({ status: 'unknown' })
  const [page, setPage] = useState(1)
  const [selectedIntentId, setSelectedIntentId] = useState<string>()
  const [selectedCustomerId, setSelectedCustomerId] = useState<string | null>(null)
  const [resolutionOpen, setResolutionOpen] = useState(false)
  const [resolutionForm] = Form.useForm<ResolutionForm>()

  const request = useMemo<DeliveryListRequest>(() => ({
    workspace_id: workspaceId,
    ...filters,
    limit: PAGE_SIZE,
    offset: (page - 1) * PAGE_SIZE
  }), [workspaceId, filters, page])
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
    { title: t`Status`, dataIndex: 'status', width: 150, render: (status: DeliveryStatus) => <Tag color={status === 'unknown' ? 'orange' : status === 'terminal_failed' ? 'red' : undefined}>{deliveryStatusLabels[status]}</Tag> },
    { title: t`Channel`, dataIndex: 'channel', width: 100, render: (channel: string) => channel.toUpperCase() },
    { title: t`Source`, width: 190, render: (_, intent) => <Typography.Text>{intent.source_type} / {intent.source_id}</Typography.Text> },
    { title: t`Why`, render: (_, intent) => deliveryExplanation(intent) },
    { title: t`Created`, dataIndex: 'created_at', width: 180, render: (value: string) => new Date(value).toLocaleString() },
    { title: t`Action`, key: 'action', width: 110, render: (_, intent) => <Button onClick={() => setSelectedIntentId(intent.id)}>{t`Review`}</Button> }
  ]

  const detail: DeliveryDetail | undefined = detailQuery.data
  const applyFilters = () => {
    setFilters({
      ...draft,
      provider: draft.provider?.trim() || undefined,
      customer_id: draft.customer_id?.trim() || undefined,
      source_id: draft.source_id?.trim() || undefined,
      from: draft.from ? new Date(draft.from).toISOString() : undefined,
      to: draft.to ? new Date(draft.to).toISOString() : undefined
    })
    setPage(1)
  }

  return (
    <Space orientation="vertical" size="large" style={{ width: '100%', minWidth: 0 }}>
      <div>
        <Typography.Title level={2} style={{ marginBottom: 4 }}>{t`Delivery Center`}</Typography.Title>
        <Typography.Text type="secondary">{t`Track every send decision, provider attempt and uncertain outcome in one place.`}</Typography.Text>
      </div>
      <Alert type="warning" showIcon title={t`Uncertain and permanently failed deliveries need attention`} description={t`An uncertain provider result is never retried automatically. Verify it first to avoid duplicate customer contact.`} />
      <Card size="small" title={t`Filters`}>
        <Space wrap align="end">
          <label><Typography.Text>{t`Status`}</Typography.Text><Select allowClear value={draft.status} onChange={(status) => setDraft((current) => ({ ...current, status }))} options={statusOptions.map((status) => ({ value: status, label: deliveryStatusLabels[status] }))} style={{ display: 'block', width: 190 }} /></label>
          <label><Typography.Text>{t`Channel`}</Typography.Text><Select allowClear value={draft.channel} onChange={(channel) => setDraft((current) => ({ ...current, channel }))} options={['email', 'sms', 'push', 'whatsapp', 'telegram', 'in_app', 'webhook'].map((channel) => ({ value: channel, label: channel.toUpperCase() }))} style={{ display: 'block', width: 150 }} /></label>
          <label><Typography.Text>{t`Source type`}</Typography.Text><Select allowClear value={draft.source_type} onChange={(source_type) => setDraft((current) => ({ ...current, source_type }))} options={['automation', 'campaign', 'broadcast', 'api', 'legacy'].map((source) => ({ value: source, label: source }))} style={{ display: 'block', width: 160 }} /></label>
          <label><Typography.Text>{t`Source ID`}</Typography.Text><Input value={draft.source_id} onChange={(event) => setDraft((current) => ({ ...current, source_id: event.target.value }))} style={{ width: 190 }} /></label>
          <label><Typography.Text>{t`Customer`}</Typography.Text><Input value={draft.customer_id} onChange={(event) => setDraft((current) => ({ ...current, customer_id: event.target.value }))} placeholder={t`Customer ID or number`} style={{ width: 210 }} /></label>
          <label><Typography.Text>{t`Provider`}</Typography.Text><Input value={draft.provider} onChange={(event) => setDraft((current) => ({ ...current, provider: event.target.value }))} placeholder={t`Provider`} style={{ width: 160 }} /></label>
          <label><Typography.Text>{t`From`}</Typography.Text><Input type="datetime-local" value={draft.from} onChange={(event) => setDraft((current) => ({ ...current, from: event.target.value || undefined }))} /></label>
          <label><Typography.Text>{t`To`}</Typography.Text><Input type="datetime-local" value={draft.to} onChange={(event) => setDraft((current) => ({ ...current, to: event.target.value || undefined }))} /></label>
          <Button type="primary" onClick={applyFilters}>{t`Apply filters`}</Button>
        </Space>
      </Card>

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
            <Descriptions.Item label={t`Status`}><Tag>{deliveryStatusLabels[detail.intent.status]}</Tag></Descriptions.Item>
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
