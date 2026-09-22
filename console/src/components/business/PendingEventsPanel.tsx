import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, App, Button, Empty, Input, Modal, Select, Space, Table, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useLingui } from '@lingui/react/macro'
import { businessActivityApi, type CustomerTrackedEvent } from '../../services/api/businessActivity'
import { customerApi, type CustomerSummary } from '../../services/api/customer'

export function PendingEventsPanel({ workspaceId, canRead, canResolve }: { workspaceId: string; canRead: boolean; canResolve: boolean }) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [selected, setSelected] = useState<CustomerTrackedEvent>()
  const [customerId, setCustomerId] = useState<string>()
  const [customerSearch, setCustomerSearch] = useState('')
  const [reason, setReason] = useState('')
  const pending = useQuery({
    queryKey: ['pending-business-events', workspaceId],
    queryFn: () => businessActivityApi.pending(workspaceId),
    enabled: canRead
  })
  const customers = useQuery({
    queryKey: ['customers', workspaceId, 'pending-event-targets', customerSearch],
    queryFn: () => customerApi.list({ workspace_id: workspaceId, search: customerSearch, limit: 50 }),
    enabled: Boolean(selected && canResolve)
  })
  const resolve = useMutation({
    mutationFn: () => businessActivityApi.resolvePending({
      workspace_id: workspaceId,
      source: selected?.source ?? '',
      report_id: selected?.report_id ?? '',
      customer_id: customerId ?? '',
      reason: reason.trim()
    }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['pending-business-events', workspaceId] })
      message.success(t`Pending event resolved and queued for processing`)
      setSelected(undefined); setCustomerId(undefined); setCustomerSearch(''); setReason('')
    },
    onError: (error) => message.error(error instanceof Error ? error.message : t`Unable to resolve pending event`)
  })
  const columns: ColumnsType<CustomerTrackedEvent> = [
    { title: t`Source`, dataIndex: 'source', width: 180 },
    { title: t`Report ID`, dataIndex: 'report_id' },
    { title: t`Event`, render: (_: unknown, item) => item.event?.event_name || item.event?.event_code || '—' },
    { title: t`Reported`, dataIndex: 'reported_at', render: (value: string) => new Date(value).toLocaleString() },
    { title: t`Status`, render: () => <Tag color="orange">{t`Waiting for customer identity`}</Tag> },
    { title: t`Action`, render: (_: unknown, item) => <Button size="small" disabled={!canResolve} onClick={() => setSelected(item)}>{t`Resolve identity`}</Button> }
  ]
  if (!canRead) return null
  return <section className="rounded-lg border border-gray-200 p-4">
    <div className="mb-3 flex items-center justify-between gap-2">
      <div><Typography.Title level={5} className="m-0">{t`Pending event identity`}</Typography.Title><Typography.Text type="secondary">{t`Accepted events without a Customer stay here until an operator resolves their identity.`}</Typography.Text></div>
      <Button onClick={() => void pending.refetch()} loading={pending.isFetching}>{t`Refresh`}</Button>
    </div>
    {!canResolve ? <Alert className="mb-3" type="info" showIcon title={t`Contacts write and Customers write permissions are required to resolve pending events`} /> : null}
    {pending.isError ? <Alert type="error" showIcon title={t`Unable to load pending events`} description={pending.error instanceof Error ? pending.error.message : undefined} /> : <Table<CustomerTrackedEvent> rowKey={(item) => `${item.source}:${item.report_id}`} loading={pending.isLoading} dataSource={pending.data?.events ?? []} columns={columns} pagination={false} scroll={{ x: 820 }} locale={{ emptyText: <Empty description={t`No pending events`} /> }} />}
    <Modal title={t`Resolve pending event identity`} open={Boolean(selected)} onCancel={() => { if (!resolve.isPending) { setSelected(undefined); setCustomerId(undefined); setCustomerSearch(''); setReason('') } }} onOk={() => resolve.mutate()} okText={t`Resolve and release`} confirmLoading={resolve.isPending} okButtonProps={{ disabled: !customerId || reason.trim().length < 1 || !canResolve }}>
      <Space orientation="vertical" className="w-full">
        <Typography.Text>{t`Report ID`}: <Typography.Text code>{selected?.report_id}</Typography.Text></Typography.Text>
        <Select aria-label={t`Target customer`} className="w-full" showSearch filterOption={false} onSearch={setCustomerSearch} placeholder={t`Search and select the Customer to associate`} value={customerId} onChange={setCustomerId} options={(customers.data?.customers ?? []).map((customer: CustomerSummary) => ({ value: customer.customer_id, label: `${customer.customer_no} · ${customer.external_user_id || t`Anonymous`}` }))} loading={customers.isLoading || customers.isFetching} />
        {customers.isError ? <Alert type="error" showIcon title={t`Unable to search target customers`} /> : null}
        <Input.TextArea aria-label={t`Resolution reason`} rows={3} value={reason} onChange={(event) => setReason(event.target.value)} placeholder={t`Explain why this event belongs to the selected Customer`} />
      </Space>
    </Modal>
  </section>
}
