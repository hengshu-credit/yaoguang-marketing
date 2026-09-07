import { Alert, Button, Empty, Input, Modal, Space, Table, Tag, Tooltip, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { PlusOutlined } from '@ant-design/icons'
import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import dayjs from '../lib/dayjs'
import { audienceApi } from '../services/api/marketing'
import { listsApi } from '../services/api/list'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import { AudienceDrawer } from '../components/audiences/AudienceDrawer'
import { ActionableError } from '../components/errors/ActionableError'
import { WorkspacePageTitle } from '../components/navigation/WorkspacePageTitle'
import { CreateListDrawer } from '../components/lists/ListDrawer'
import { ListStats } from '../components/lists/ListStats'
import type { List } from '../services/api/list'

const { Paragraph, Text } = Typography

interface AudienceRow {
  id: string
  sourceType: 'list' | 'dynamic'
  name: string
  description?: string
  createdAt?: string
  updatedAt?: string
  list?: List
}

export function AudiencesPage() {
  const { t } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId' })
  const { workspaces } = useAuth()
  const { permissions } = useWorkspacePermissions(workspaceId)
  const queryClient = useQueryClient()
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [editingAudienceId, setEditingAudienceId] = useState<string>()
  const [itemToDelete, setItemToDelete] = useState<AudienceRow | null>(null)
  const [confirmationInput, setConfirmationInput] = useState('')
  const [deleteError, setDeleteError] = useState<string>()
  const [deleting, setDeleting] = useState(false)
  const workspace = workspaces.find((item) => item.id === workspaceId)
  const canReadAudiences = Boolean(permissions?.segments?.read)
  const canReadLists = Boolean(permissions?.lists?.read)
  const audiences = useQuery({
    queryKey: ['audiences', workspaceId],
    queryFn: () => audienceApi.list(workspaceId),
    enabled: Boolean(workspaceId) && canReadAudiences
  })
  const lists = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId }),
    enabled: Boolean(workspaceId) && canReadLists
  })

  if (audiences.error || lists.error) {
    const failed = audiences.error ? audiences : lists
    return <div className="p-6"><ActionableError error={failed.error} onRetry={() => void failed.refetch()} /></div>
  }

  const items: AudienceRow[] = [
    ...(lists.data?.lists ?? []).map((list: List) => ({
      id: list.id,
      sourceType: 'list' as const,
      name: list.name,
      description: list.description,
      createdAt: list.created_at,
      updatedAt: list.updated_at,
      list
    })),
    ...(audiences.data?.items ?? []).map((audience) => ({
      id: audience.id,
      sourceType: 'dynamic' as const,
      name: audience.name,
      description: audience.description,
      createdAt: audience.created_at,
      updatedAt: audience.updated_at
    }))
  ]
  const canWrite = Boolean(permissions?.segments?.write)
  const canWriteLists = Boolean(permissions?.lists?.write)
  const closeDelete = () => {
    setItemToDelete(null)
    setConfirmationInput('')
    setDeleteError(undefined)
  }
  const deleteItem = async () => {
    if (!itemToDelete || confirmationInput !== itemToDelete.id) return
    setDeleting(true)
    setDeleteError(undefined)
    try {
      if (itemToDelete.list) {
        await listsApi.delete({ workspace_id: workspaceId, id: itemToDelete.id })
        await queryClient.invalidateQueries({ queryKey: ['lists', workspaceId] })
      } else {
        await audienceApi.delete(workspaceId, itemToDelete.id)
        await queryClient.invalidateQueries({ queryKey: ['audiences', workspaceId] })
      }
      closeDelete()
    } catch (error) {
      setDeleteError(error instanceof Error ? error.message : t`Failed to delete audience`)
    } finally {
      setDeleting(false)
    }
  }
  const formatDate = (value?: string) => {
    if (!value) return <Text type="secondary">—</Text>
    const date = dayjs(value).tz(workspace?.settings?.timezone || 'UTC')
    return (
      <Tooltip title={date.format('LLLL')}>
        <span>{date.format('YYYY-MM-DD HH:mm')}</span>
      </Tooltip>
    )
  }
  const columns: ColumnsType<AudienceRow> = [
    {
      title: t`Name`,
      dataIndex: 'name',
      render: (name: string, row) => (
        <Link
          to="/console/workspace/$workspaceId/audiences/$sourceType/$sourceId"
          params={{ workspaceId, sourceType: row.sourceType, sourceId: row.id }}
        >
          {name}
        </Link>
      )
    },
    {
      title: t`Type`,
      dataIndex: 'sourceType',
      width: 140,
      render: (sourceType: AudienceRow['sourceType']) => (
        <Tag color={sourceType === 'dynamic' ? 'blue' : 'green'}>
          {sourceType === 'dynamic' ? t`Dynamic` : t`List`}
        </Tag>
      )
    },
    {
      title: t`Description`,
      dataIndex: 'description',
      render: (description?: string) => description || <Text type="secondary">—</Text>
    },
    { title: t`Created At`, dataIndex: 'createdAt', width: 170, render: formatDate },
    { title: t`Updated At`, dataIndex: 'updatedAt', width: 170, render: formatDate },
    {
      title: t`Actions`,
      key: 'actions',
      width: 190,
      render: (_: unknown, row) => row.list ? (
        <Space>
          <CreateListDrawer
            workspaceId={workspaceId}
            list={row.list}
            buttonProps={{ type: 'link', buttonContent: t`Edit list`, disabled: !canWriteLists }}
          />
          <Button
            type="link"
            danger
            aria-label={t`Delete ${row.name}`}
            disabled={!canWriteLists}
            onClick={() => setItemToDelete(row)}
          >
            {t`Delete`}
          </Button>
        </Space>
      ) : (
        <Space>
          <Button
            type="link"
            aria-label={`${t`Edit`} ${row.name}`}
            disabled={!canWrite}
            onClick={() => setEditingAudienceId(row.id)}
          >
            {t`Edit`}
          </Button>
          <Button
            type="link"
            danger
            aria-label={t`Delete ${row.name}`}
            disabled={!canWrite}
            onClick={() => setItemToDelete(row)}
          >
            {t`Delete`}
          </Button>
        </Space>
      )
    }
  ]

  return (
    <div>
      <div className="flex flex-col sm:flex-row justify-between items-start gap-4 mb-6">
        <div>
          <WorkspacePageTitle style={{ marginBottom: 4 }}>{t`Audience segmentation`}</WorkspacePageTitle>
          <Paragraph type="secondary" className="!mb-0">
            {t`Define reusable audiences from customer attributes, status, lists, activity, and goals.`}
          </Paragraph>
        </div>
        <Space>
          <CreateListDrawer
            workspaceId={workspaceId}
            buttonProps={{ buttonContent: t`Create list`, disabled: !canWriteLists }}
          />
          <Button
            type="primary"
            icon={<PlusOutlined />}
            aria-label={t`Create dynamic audience`}
            disabled={!canWrite}
            onClick={() => {
              setEditingAudienceId(undefined)
              setDrawerOpen(true)
            }}
          >
            {t`Create dynamic audience`}
          </Button>
        </Space>
      </div>

      {(canReadAudiences && audiences.isLoading) || (canReadLists && lists.isLoading) ? (
        <Table<AudienceRow> rowKey={(row) => `${row.sourceType}:${row.id}`} columns={columns} loading pagination={false} />
      ) : items.length === 0 ? (
        <Empty description={t`No audiences yet`} />
      ) : (
        <Table<AudienceRow>
          rowKey={(row) => `${row.sourceType}:${row.id}`}
          columns={columns}
          dataSource={items}
          pagination={false}
          scroll={{ x: 960 }}
          expandable={{
            expandedRowKeys: items.filter((row) => row.list).map((row) => `list:${row.id}`),
            expandedRowRender: (row) => row.list ? (
              <div aria-label={t`Customer status counts for ${row.name}`} className="py-2 px-4">
                <ListStats workspaceId={workspaceId} listId={row.id} />
              </div>
            ) : null,
            rowExpandable: (row) => Boolean(row.list),
            showExpandColumn: false
          }}
        />
      )}

      <AudienceDrawer
        open={drawerOpen || Boolean(editingAudienceId)}
        workspaceId={workspaceId}
        audienceId={editingAudienceId}
        lists={(lists.data?.lists ?? []).map((list) => ({ id: list.id, name: list.name }))}
        customFieldLabels={workspace?.settings?.custom_field_labels}
        onClose={() => {
          setDrawerOpen(false)
          setEditingAudienceId(undefined)
        }}
      />
      <Modal
        title={itemToDelete?.list ? t`Delete List` : t`Delete audience`}
        open={Boolean(itemToDelete)}
        onCancel={closeDelete}
        footer={[
          <Button key="cancel" onClick={closeDelete}>{t`Cancel`}</Button>,
          <Button
            key="delete"
            type="primary"
            danger
            loading={deleting}
            disabled={confirmationInput !== (itemToDelete?.id ?? '')}
            onClick={() => void deleteItem()}
          >
            {t`Delete`}
          </Button>
        ]}
      >
        {itemToDelete && (
          <Space orientation="vertical" style={{ width: '100%' }}>
            <Text>{t`Enter the ID to confirm deletion of "${itemToDelete.name}".`}</Text>
            <Text code>{itemToDelete.id}</Text>
            <Input
              placeholder={t`Enter ID to confirm`}
              value={confirmationInput}
              onChange={(event) => setConfirmationInput(event.target.value)}
            />
            {deleteError && <Alert type="error" showIcon message={deleteError} />}
          </Space>
        )}
      </Modal>
    </div>
  )
}
