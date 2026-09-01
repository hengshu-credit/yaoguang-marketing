import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from '@tanstack/react-router'
import { Alert, Button, Card, DatePicker, Empty, Input, Select, Space, Table, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import dayjs, { type Dayjs } from 'dayjs'
import utc from 'dayjs/plugin/utc'
import { useLingui } from '@lingui/react/macro'
import { audienceApi, type AudienceMember, type AudienceMemberRequest } from '../services/api/marketing'
import { listsApi } from '../services/api/list'
import { CustomerDrawer } from '../components/customers/CustomerDrawer'
import { useWorkspacePermissions } from '../contexts/AuthContext'
import { WorkspacePageTitle } from '../components/navigation/WorkspacePageTitle'

dayjs.extend(utc)

interface MemberFilters {
  status: string
  eventName: string
  joinedRange: [Dayjs, Dayjs] | null
  attributeKey: string
  attributeValue: string
}

const emptyFilters = (): MemberFilters => ({
  status: '',
  eventName: '',
  joinedRange: null,
  attributeKey: '',
  attributeValue: ''
})

export function AudienceMembersPage() {
  const { t } = useLingui()
  const { workspaceId, sourceType, sourceId } = useParams({ strict: false }) as {
    workspaceId: string
    sourceType: 'list' | 'dynamic'
    sourceId: string
  }
  const { permissions } = useWorkspacePermissions(workspaceId)
  const [draft, setDraft] = useState<MemberFilters>(emptyFilters)
  const [filters, setFilters] = useState<MemberFilters>(emptyFilters)
  const [cursor, setCursor] = useState('')
  const [cursorHistory, setCursorHistory] = useState<string[]>([])
  const [selectedCustomerID, setSelectedCustomerID] = useState<string | null>(null)

  const lists = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId }),
    enabled: Boolean(permissions?.lists?.read)
  })
  const audience = useQuery({
    queryKey: ['audience', workspaceId, sourceId],
    queryFn: () => audienceApi.get(workspaceId, sourceId),
    enabled: sourceType === 'dynamic' && Boolean(permissions?.segments?.read)
  })
  const request = useMemo<AudienceMemberRequest>(() => ({
    workspace_id: workspaceId,
    ...(sourceType === 'list' ? { list_id: sourceId } : { audience_id: sourceId }),
    status: filters.status || undefined,
    event_name: filters.eventName.trim() || undefined,
    joined_after: filters.joinedRange?.[0].startOf('day').toISOString(),
    joined_before: filters.joinedRange?.[1].add(1, 'day').startOf('day').toISOString(),
    attribute_key: filters.attributeKey.trim() || undefined,
    attribute_value: filters.attributeValue.trim() || undefined,
    after: cursor || undefined,
    limit: 50
  }), [workspaceId, sourceType, sourceId, filters, cursor])
  const members = useQuery({
    queryKey: ['audience-members', request],
    queryFn: () => audienceApi.memberDetails(request)
  })

  const listNames = useMemo(
    () => new Map((lists.data?.lists ?? []).map((list) => [list.id, list.name])),
    [lists.data?.lists]
  )
  const sourceName = sourceType === 'list'
    ? listNames.get(sourceId) ?? sourceId
    : audience.data?.name ?? sourceId

  const statusLabel = (status: string) => {
    switch (status) {
      case 'active': return t`Active`
      case 'pending': return t`Pending`
      case 'unsubscribed': return t`Unsubscribed`
      case 'bounced': return t`Bounced`
      case 'complained': return t`Complained`
      default: return status
    }
  }
  const statusColor = (status: string) => {
    switch (status) {
      case 'active': return 'green'
      case 'pending': return 'blue'
      case 'bounced': return 'orange'
      case 'complained': return 'red'
      default: return 'default'
    }
  }
  const columns: ColumnsType<AudienceMember> = [
    {
      title: t`Customer number`,
      dataIndex: ['customer', 'customer_no'],
      width: 220
    },
    {
      title: t`Identity aliases`,
      dataIndex: ['customer', 'identities'],
      render: (identities: AudienceMember['customer']['identities']) => (
        <Space wrap>{(identities ?? []).map((identity) => <Tag key={identity.id}>{identity.display_hint}</Tag>)}</Space>
      )
    },
    {
      title: t`Current subscription status`,
      dataIndex: 'subscriptions',
      render: (subscriptions: AudienceMember['subscriptions']) => {
        const visibleSubscriptions = sourceType === 'list'
          ? (subscriptions ?? []).filter((subscription) => subscription.list_id === sourceId)
          : (subscriptions ?? [])
        return (
          <Space wrap>
            {visibleSubscriptions.map((subscription) => (
              <Tag key={subscription.list_id} color={statusColor(subscription.status)}>
                {sourceType === 'dynamic' && `${listNames.get(subscription.list_id) ?? subscription.list_id} · `}
                {statusLabel(subscription.status)}
              </Tag>
            ))}
          </Space>
        )
      }
    },
    ...(sourceType === 'list' ? [{
      title: t`Joined list at`,
      dataIndex: 'joined_at',
      width: 180,
      render: (value?: string) => value ? dayjs.utc(value).format('YYYY-MM-DD HH:mm') : '—'
    } satisfies ColumnsType<AudienceMember>[number]] : []),
    {
      title: t`Customer attributes`,
      dataIndex: ['customer', 'profile', 'attributes'],
      render: (attributes?: Record<string, unknown>) => attributes && Object.keys(attributes).length > 0
        ? <Typography.Text code>{JSON.stringify(attributes)}</Typography.Text>
        : <Typography.Text type="secondary">—</Typography.Text>
    }
  ]

  const applyFilters = () => {
    setCursor('')
    setCursorHistory([])
    setFilters({ ...draft })
  }
  const resetFilters = () => {
    const cleared = emptyFilters()
    setDraft(cleared)
    setFilters(cleared)
    setCursor('')
    setCursorHistory([])
  }
  const nextPage = () => {
    if (!members.data?.next) return
    setCursorHistory((history) => [...history, cursor])
    setCursor(members.data.next)
  }
  const previousPage = () => {
    setCursorHistory((history) => {
      if (history.length === 0) return history
      setCursor(history[history.length - 1])
      return history.slice(0, -1)
    })
  }

  return (
    <Space orientation="vertical" size="large" style={{ width: '100%', minWidth: 0 }}>
      <div>
        <Link to="/console/workspace/$workspaceId/audiences" params={{ workspaceId }}>
          {t`Back to audiences`}
        </Link>
        <WorkspacePageTitle style={{ marginTop: 8, marginBottom: 4 }}>
          {t`Customers in ${sourceName}`}
        </WorkspacePageTitle>
        <Typography.Text type="secondary">
          {sourceType === 'list'
            ? t`Subscription status and customer facts are read in real time for this list.`
            : t`Audience membership and customer facts are evaluated from the current audience definition.`}
        </Typography.Text>
      </div>

      <Card size="small" title={t`Filters`}>
        <Space wrap align="end">
          <div>
            <Typography.Text className="block mb-1">{t`Subscription status`}</Typography.Text>
            <Select
              aria-label={t`Subscription status`}
              allowClear
              value={draft.status || undefined}
              onChange={(status) => setDraft((current) => ({ ...current, status: status ?? '' }))}
              placeholder={t`Any status`}
              style={{ width: 180 }}
              options={[
                { value: 'active', label: t`Active` },
                { value: 'pending', label: t`Pending` },
                { value: 'unsubscribed', label: t`Unsubscribed` },
                { value: 'bounced', label: t`Bounced` },
                { value: 'complained', label: t`Complained` }
              ]}
            />
          </div>
          {sourceType === 'list' && (
            <div role="group" aria-label={t`Joined list between`}>
              <Typography.Text className="block mb-1">{t`Joined list between`}</Typography.Text>
              <DatePicker.RangePicker
                value={draft.joinedRange}
                onChange={(range) => setDraft((current) => ({
                  ...current,
                  joinedRange: range?.[0] && range[1] ? [range[0], range[1]] : null
                }))}
              />
            </div>
          )}
          <div>
            <Typography.Text className="block mb-1">{t`Event name`}</Typography.Text>
            <Input
              aria-label={t`Event name`}
              value={draft.eventName}
              onChange={(event) => setDraft((current) => ({ ...current, eventName: event.target.value }))}
              placeholder={t`Customers who performed this event`}
              style={{ width: 220 }}
            />
          </div>
          <div>
            <Typography.Text className="block mb-1">{t`Attribute key`}</Typography.Text>
            <Input
              aria-label={t`Attribute key`}
              value={draft.attributeKey}
              onChange={(event) => setDraft((current) => ({ ...current, attributeKey: event.target.value }))}
              placeholder={t`For example, tier`}
              style={{ width: 180 }}
            />
          </div>
          <div>
            <Typography.Text className="block mb-1">{t`Attribute value`}</Typography.Text>
            <Input
              aria-label={t`Attribute value`}
              value={draft.attributeValue}
              onChange={(event) => setDraft((current) => ({ ...current, attributeValue: event.target.value }))}
              placeholder={t`Contains this value`}
              style={{ width: 180 }}
            />
          </div>
          <Button
            type="primary"
            onClick={applyFilters}
            disabled={Boolean(draft.attributeKey.trim()) !== Boolean(draft.attributeValue.trim())}
          >
            {t`Apply filters`}
          </Button>
          <Button onClick={resetFilters}>{t`Reset`}</Button>
        </Space>
      </Card>

      {members.isError && (
        <Alert
          type="error"
          showIcon
          title={t`Unable to load audience customers`}
          description={members.error instanceof Error ? members.error.message : undefined}
          action={<Button onClick={() => members.refetch()}>{t`Retry`}</Button>}
        />
      )}
      {!members.isError && (
        <Table<AudienceMember>
          rowKey={(item) => item.customer.customer_id}
          columns={columns}
          dataSource={members.data?.items ?? []}
          loading={members.isLoading || members.isFetching}
          pagination={false}
          scroll={{ x: 960 }}
          locale={{ emptyText: <Empty description={t`No customers match these filters`} /> }}
          onRow={(item) => ({
            onClick: () => setSelectedCustomerID(item.customer.customer_id),
            style: { cursor: 'pointer' }
          })}
        />
      )}
      <Space style={{ justifyContent: 'flex-end', width: '100%' }}>
        <Button disabled={cursorHistory.length === 0} onClick={previousPage}>{t`Previous`}</Button>
        <Button disabled={!members.data?.next} onClick={nextPage}>{t`Next`}</Button>
      </Space>

      <CustomerDrawer
        workspaceId={workspaceId}
        customerId={selectedCustomerID}
        open={Boolean(selectedCustomerID)}
        onClose={() => setSelectedCustomerID(null)}
        canWrite={Boolean(permissions?.customers?.write)}
      />
    </Space>
  )
}
