import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, App, Button, Empty, Input, Modal, Select, Space, Table, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useLingui } from '@lingui/react/macro'
import { customerApi, customerQueryKeys, type CustomerSummary } from '../../services/api/customer'
import { CustomerDrawer } from './CustomerDrawer'

interface CustomerDirectoryProps {
  workspaceId: string
  canWrite: boolean
}

export function CustomerDirectory({ workspaceId, canWrite }: CustomerDirectoryProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [searchInput, setSearchInput] = useState('')
  const [search, setSearch] = useState('')
  const [cursor, setCursor] = useState<string>()
  const [cursorHistory, setCursorHistory] = useState<string[]>([])
  const [selectedCustomerId, setSelectedCustomerId] = useState<string | null>(null)
  const [mergeSource, setMergeSource] = useState<CustomerSummary | null>(null)
  const [mergeTargetId, setMergeTargetId] = useState<string>()
  const [mergeTargetSearch, setMergeTargetSearch] = useState('')
  const [mergeReason, setMergeReason] = useState('')
  const [merging, setMerging] = useState(false)

  const request = useMemo(() => ({ search, cursor, limit: 50 }), [search, cursor])
  const customers = useQuery({
    queryKey: customerQueryKeys.list(workspaceId, request),
    queryFn: () => customerApi.list({ workspace_id: workspaceId, ...request }),
    enabled: Boolean(workspaceId)
  })
  const rows = customers.data?.customers ?? []
  const mergeTargets = useQuery({
    queryKey: customerQueryKeys.list(workspaceId, { search: mergeTargetSearch, limit: 50 }),
    queryFn: () => customerApi.list({ workspace_id: workspaceId, search: mergeTargetSearch, limit: 50 }),
    enabled: Boolean(mergeSource && canWrite)
  })

  const applySearch = () => {
    setSearch(searchInput.trim())
    setCursor(undefined)
    setCursorHistory([])
  }

  const openMerge = (customer: CustomerSummary) => {
    setMergeSource(customer)
    setMergeTargetId(undefined)
    setMergeTargetSearch('')
    setMergeReason('')
  }

  const closeMerge = () => {
    if (merging) return
    setMergeSource(null)
    setMergeTargetId(undefined)
    setMergeTargetSearch('')
    setMergeReason('')
  }

  const merge = async () => {
    if (!mergeSource || !mergeTargetId || mergeTargetId === mergeSource.customer_id || !canWrite) return
    setMerging(true)
    try {
      await customerApi.merge({
        workspace_id: workspaceId,
        idempotency_key: crypto.randomUUID(),
        source: { customer_id: mergeSource.customer_id },
        target: { customer_id: mergeTargetId },
        reason: mergeReason.trim() || undefined
      })
      await queryClient.invalidateQueries({ queryKey: customerQueryKeys.all(workspaceId) })
      message.success(t`Customers merged successfully`)
      setMergeSource(null)
      setMergeTargetId(undefined)
      setMergeTargetSearch('')
      setMergeReason('')
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Unable to merge customers`)
    } finally {
      setMerging(false)
    }
  }

  const columns: ColumnsType<CustomerSummary> = [
    {
      title: t`Customer number`,
      dataIndex: 'customer_no',
      width: 230,
      render: (value: string, customer) => <Button type="link" className="p-0" onClick={() => setSelectedCustomerId(customer.customer_id)}>{value}</Button>
    },
    {
      title: t`External user ID`,
      dataIndex: 'external_user_id',
      render: (value?: string) => value || <Typography.Text type="secondary">{t`Anonymous`}</Typography.Text>
    },
    {
      title: t`Identity aliases`,
      dataIndex: 'identities',
      render: (identities: CustomerSummary['identities']) => (
        <Space wrap>{(identities ?? []).map((identity) => <Tag key={identity.id}>{identity.display_hint}</Tag>)}</Space>
      )
    },
    {
      title: t`Status`,
      render: (_: unknown, customer) => <Space wrap>
        <Tag color={customer.profile?.status === 'active' ? 'green' : undefined}>{customer.profile?.status || t`No status`}</Tag>
        {(customer.tags ?? []).map((tag) => <Tag key={tag}>{tag}</Tag>)}
      </Space>
    },
    {
      title: t`Actions`,
      key: 'actions',
      width: 150,
      render: (_: unknown, customer) => <Space>
        <Button size="small" onClick={() => setSelectedCustomerId(customer.customer_id)}>{t`Open`}</Button>
        {canWrite ? <Button size="small" onClick={() => openMerge(customer)}>{t`Merge`}</Button> : null}
      </Space>
    }
  ]

  return <Space orientation="vertical" size="middle" className="w-full">
    <Space.Compact className="w-full max-w-2xl">
      <Input
        aria-label={t`Search customers`}
        value={searchInput}
        onChange={(event) => setSearchInput(event.target.value)}
        onPressEnter={applySearch}
        placeholder={t`Search customer number, external ID, email, phone or anonymous identity`}
        allowClear
      />
      <Button type="primary" onClick={applySearch}>{t`Search`}</Button>
    </Space.Compact>
    <Typography.Text type="secondary">{t`Search supports customer_no, external_user_id and masked or exact identity values. Open a customer to maintain profile attributes, tags and memberships.`}</Typography.Text>
    {customers.isError ? <Alert type="error" showIcon title={t`Unable to load customers`} description={customers.error instanceof Error ? customers.error.message : undefined} action={<Button onClick={() => void customers.refetch()}>{t`Retry`}</Button>} /> : null}
    <Table<CustomerSummary>
      rowKey="customer_id"
      columns={columns}
      dataSource={rows}
      loading={customers.isLoading || customers.isFetching}
      pagination={false}
      scroll={{ x: 980 }}
      locale={{ emptyText: <Empty description={search ? t`No customers match this search` : t`No customers yet`} /> }}
      onRow={(customer) => ({ onClick: (event) => {
        const target = event.target as HTMLElement
        if (target.closest('button, a, input')) return
        setSelectedCustomerId(customer.customer_id)
      } })}
    />
    <Space className="w-full justify-end">
      <Button disabled={cursorHistory.length === 0 || customers.isFetching} onClick={() => setCursorHistory((history) => {
        const next = [...history]
        setCursor(next.pop() || undefined)
        return next
      })}>{t`Previous`}</Button>
      <Button disabled={!customers.data?.next_cursor || customers.isFetching} onClick={() => {
        if (!customers.data?.next_cursor) return
        setCursorHistory((history) => [...history, cursor ?? ''])
        setCursor(customers.data.next_cursor)
      }}>{t`Next`}</Button>
    </Space>
    <CustomerDrawer
      workspaceId={workspaceId}
      customerId={selectedCustomerId}
      open={Boolean(selectedCustomerId)}
      onClose={() => setSelectedCustomerId(null)}
      canWrite={canWrite}
    />
    <Modal
      title={t`Merge customers`}
      open={Boolean(mergeSource)}
      onCancel={closeMerge}
      onOk={() => void merge()}
      okText={t`Merge`}
      confirmLoading={merging}
      okButtonProps={{ danger: true, disabled: !mergeTargetId || mergeTargetId === mergeSource?.customer_id }}
    >
      {mergeSource ? <Space orientation="vertical" className="w-full">
        <Typography.Text>{t`Source customer`}: <Typography.Text code>{mergeSource.customer_no}</Typography.Text></Typography.Text>
        <Select
          aria-label={t`Target customer`}
          showSearch
          filterOption={false}
          onSearch={setMergeTargetSearch}
          className="w-full"
          placeholder={t`Select the surviving customer`}
          value={mergeTargetId}
          onChange={setMergeTargetId}
          loading={mergeTargets.isLoading || mergeTargets.isFetching}
          options={(mergeTargets.data?.customers ?? []).filter((customer) => customer.customer_id !== mergeSource.customer_id).map((customer) => ({ value: customer.customer_id, label: `${customer.customer_no} · ${customer.external_user_id || t`Anonymous`}` }))}
        />
        {mergeTargets.isError ? <Alert type="error" showIcon title={t`Unable to search target customers`} action={<Button onClick={() => void mergeTargets.refetch()}>{t`Retry`}</Button>} /> : null}
        <Input.TextArea aria-label={t`Merge reason`} value={mergeReason} onChange={(event) => setMergeReason(event.target.value)} placeholder={t`Optional audit reason`} rows={3} />
        <Typography.Text type="secondary">{t`The target keeps its customer number. The source becomes a redirect and its identities, tags, memberships and activity are retained in the audit trail.`}</Typography.Text>
      </Space> : null}
    </Modal>
  </Space>
}
