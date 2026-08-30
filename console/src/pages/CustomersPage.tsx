import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useParams } from '@tanstack/react-router'
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Empty,
  Input,
  Space,
  Table,
  Tag,
  Typography
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useLingui } from '@lingui/react/macro'
import {
  customerApi,
  customerQueryKeys,
  type CustomerListRequest,
  type CustomerSummary
} from '../services/api/customer'
import { CustomerDrawer } from '../components/customers/CustomerDrawer'

function useNarrowCustomerLayout() {
  const [narrow, setNarrow] = useState(() => window.innerWidth < 768)
  useEffect(() => {
    const update = () => setNarrow(window.innerWidth < 768)
    window.addEventListener('resize', update)
    return () => window.removeEventListener('resize', update)
  }, [])
  return narrow
}

export function CustomersPage() {
  const { t } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId/customers' })
  const narrow = useNarrowCustomerLayout()
  const [searchInput, setSearchInput] = useState('')
  const [search, setSearch] = useState('')
  const [includeMerged, setIncludeMerged] = useState(false)
  const [cursor, setCursor] = useState('')
  const [cursorHistory, setCursorHistory] = useState<string[]>([])
  const [selectedCustomerID, setSelectedCustomerID] = useState<string | null>(null)

  const request = useMemo<CustomerListRequest>(
    () => ({
      workspace_id: workspaceId,
      search: search || undefined,
      cursor: cursor || undefined,
      include_merged: includeMerged || undefined,
      limit: 50
    }),
    [workspaceId, search, cursor, includeMerged]
  )
  const query = useQuery({
    queryKey: customerQueryKeys.list(workspaceId, request),
    queryFn: () => customerApi.list(request)
  })

  const applySearch = (value: string) => {
    setSearch(value.trim())
    setCursor('')
    setCursorHistory([])
  }
  const changeMerged = (checked: boolean) => {
    setIncludeMerged(checked)
    setCursor('')
    setCursorHistory([])
  }
  const nextPage = () => {
    if (!query.data?.next_cursor) return
    setCursorHistory((history) => [...history, cursor])
    setCursor(query.data.next_cursor!)
  }
  const previousPage = () => {
    setCursorHistory((history) => {
      if (history.length === 0) return history
      setCursor(history[history.length - 1])
      return history.slice(0, -1)
    })
  }

  const columns: ColumnsType<CustomerSummary> = [
    {
      title: t`Customer number`,
      dataIndex: 'customer_no',
      width: 260,
      render: (value: string) => <Typography.Text copyable>{value}</Typography.Text>
    },
    {
      title: t`External user ID`,
      dataIndex: 'external_user_id',
      width: 180,
      render: (value?: string) => value || '—'
    },
    {
      title: t`Identity aliases`,
      dataIndex: 'identities',
      render: (identities: CustomerSummary['identities']) => (
        <Space wrap>
          {(identities ?? []).map((identity) => (
            <Tag key={identity.id}>{identity.display_hint}</Tag>
          ))}
        </Space>
      )
    },
    {
      title: t`Status`,
      dataIndex: ['profile', 'status'],
      width: 120,
      render: (value?: string) => value || '—'
    },
    {
      title: t`Tags`,
      dataIndex: 'tags',
      render: (tags: string[]) => (
        <Space wrap>
          {(tags ?? []).map((tag) => (
            <Tag key={tag}>{tag}</Tag>
          ))}
        </Space>
      )
    }
  ]

  const customers = query.data?.customers ?? []

  return (
    <Space orientation="vertical" size="large" style={{ width: '100%', minWidth: 0 }}>
      <div>
        <Typography.Title level={2} style={{ marginBottom: 4 }}>
          {t`Customers`}
        </Typography.Title>
        <Typography.Text type="secondary">
          {t`Unified customer profiles and identity aliases for this workspace.`}
        </Typography.Text>
      </div>

      <Space wrap style={{ width: '100%', justifyContent: 'space-between' }}>
        <Input.Search
          value={searchInput}
          onChange={(event) => setSearchInput(event.target.value)}
          onSearch={applySearch}
          allowClear
          enterButton={t`Search`}
          placeholder={t`Search customer number, external ID, email or phone`}
          style={{ width: narrow ? '100%' : 460, maxWidth: '100%' }}
        />
        <Checkbox checked={includeMerged} onChange={(event) => changeMerged(event.target.checked)}>
          {t`Include merged customers`}
        </Checkbox>
      </Space>

      {query.isError && (
        <Alert
          type="error"
          showIcon
          title={t`Unable to load customers`}
          description={query.error instanceof Error ? query.error.message : undefined}
          action={<Button onClick={() => query.refetch()}>{t`Retry`}</Button>}
        />
      )}

      {!query.isError && narrow && (
        <Space orientation="vertical" style={{ width: '100%' }}>
          {customers.length === 0 && !query.isLoading ? (
            <Empty description={t`No customers found`} />
          ) : (
            customers.map((customer) => (
              <Card
                key={customer.customer_id}
                size="small"
                hoverable
                onClick={() => setSelectedCustomerID(customer.customer_id)}
              >
                <Space orientation="vertical" size="small" style={{ width: '100%' }}>
                  <Typography.Text strong style={{ wordBreak: 'break-all' }}>
                    {customer.customer_no}
                  </Typography.Text>
                  <Typography.Text>{customer.external_user_id || t`No external user ID`}</Typography.Text>
                  <Space wrap>
                    {(customer.identities ?? []).map((identity) => (
                      <Tag key={identity.id}>{identity.display_hint}</Tag>
                    ))}
                  </Space>
                </Space>
              </Card>
            ))
          )}
        </Space>
      )}

      {!query.isError && !narrow && (
        <Table<CustomerSummary>
          rowKey="customer_id"
          columns={columns}
          dataSource={customers}
          loading={query.isLoading || query.isFetching}
          pagination={false}
          scroll={{ x: 900 }}
          locale={{ emptyText: <Empty description={t`No customers found`} /> }}
          onRow={(customer) => ({
            onClick: () => setSelectedCustomerID(customer.customer_id),
            style: { cursor: 'pointer' }
          })}
        />
      )}

      <Space style={{ justifyContent: 'flex-end', width: '100%' }}>
        <Button aria-label={t`Previous page`} disabled={cursorHistory.length === 0} onClick={previousPage}>
          {t`Previous`}
        </Button>
        <Button aria-label={t`Next page`} disabled={!query.data?.next_cursor} onClick={nextPage}>
          {t`Next`}
        </Button>
      </Space>

      <CustomerDrawer
        workspaceId={workspaceId}
        customerId={selectedCustomerID}
        open={Boolean(selectedCustomerID)}
        onClose={() => setSelectedCustomerID(null)}
      />
    </Space>
  )
}
