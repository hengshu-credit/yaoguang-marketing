import { Alert, Button, Empty, Input, Modal, Space, Table, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { PlusOutlined } from '@ant-design/icons'
import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { audienceApi } from '../services/api/marketing'
import { listsApi } from '../services/api/list'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import { AudienceDrawer } from '../components/audiences/AudienceDrawer'
import { ActionableError } from '../components/errors/ActionableError'
import { WorkspacePageTitle } from '../components/navigation/WorkspacePageTitle'
import { CreateListDrawer } from '../components/lists/ListDrawer'
import type { List } from '../services/api/list'

const { Paragraph, Text } = Typography

interface AudienceRow {
  id: string
  sourceType: 'list' | 'dynamic'
  name: string
  description?: string
  version?: number
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
  const [listToDelete, setListToDelete] = useState<List | null>(null)
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
      list
    })),
    ...(audiences.data?.items ?? []).map((audience) => ({
      id: audience.id,
      sourceType: 'dynamic' as const,
      name: audience.name,
      description: audience.description,
      version: audience.active_version
    }))
  ]
  const canWrite = Boolean(permissions?.segments?.write)
  const canWriteLists = Boolean(permissions?.lists?.write)
  const closeDelete = () => {
    setListToDelete(null)
    setConfirmationInput('')
    setDeleteError(undefined)
  }
  const deleteList = async () => {
    if (!listToDelete || confirmationInput !== listToDelete.id) return
    setDeleting(true)
    setDeleteError(undefined)
    try {
      await listsApi.delete({ workspace_id: workspaceId, id: listToDelete.id })
      await queryClient.invalidateQueries({ queryKey: ['lists', workspaceId] })
      closeDelete()
    } catch (error) {
      setDeleteError(error instanceof Error ? error.message : t`Failed to delete list`)
    } finally {
      setDeleting(false)
    }
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
    {
      title: t`Version`,
      dataIndex: 'version',
      width: 120,
      render: (version?: number) => version ? t`Version ${version}` : <Text type="secondary">—</Text>
    },
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
            onClick={() => setListToDelete(row.list ?? null)}
          >
            {t`Delete`}
          </Button>
        </Space>
      ) : (
        <Button
          type="link"
          aria-label={`${t`Edit`} ${row.name}`}
          disabled={!canWrite}
          onClick={() => setEditingAudienceId(row.id)}
        >
          {t`Edit`}
        </Button>
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
          scroll={{ x: 720 }}
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
        title={t`Delete List`}
        open={Boolean(listToDelete)}
        onCancel={closeDelete}
        footer={[
          <Button key="cancel" onClick={closeDelete}>{t`Cancel`}</Button>,
          <Button
            key="delete"
            type="primary"
            danger
            loading={deleting}
            disabled={confirmationInput !== (listToDelete?.id ?? '')}
            onClick={() => void deleteList()}
          >
            {t`Delete`}
          </Button>
        ]}
      >
        {listToDelete && (
          <Space orientation="vertical" style={{ width: '100%' }}>
            <Text>{t`Enter the list ID to confirm deletion of "${listToDelete.name}".`}</Text>
            <Text code>{listToDelete.id}</Text>
            <Input
              placeholder={t`Enter list ID to confirm`}
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
