import { Button, Card, Col, Empty, Row, Space, Tag, Typography } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { audienceApi } from '../services/api/marketing'
import { listsApi } from '../services/api/list'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import { AudienceDrawer } from '../components/audiences/AudienceDrawer'
import { ActionableError } from '../components/errors/ActionableError'
import { WorkspacePageTitle } from '../components/navigation/WorkspacePageTitle'

const { Paragraph, Text } = Typography

export function AudiencesPage() {
  const { t } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId' })
  const { workspaces } = useAuth()
  const { permissions } = useWorkspacePermissions(workspaceId)
  const [drawerOpen, setDrawerOpen] = useState(false)
  const workspace = workspaces.find((item) => item.id === workspaceId)
  const audiences = useQuery({
    queryKey: ['audiences', workspaceId],
    queryFn: () => audienceApi.list(workspaceId),
    enabled: Boolean(workspaceId)
  })
  const lists = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId }),
    enabled: Boolean(workspaceId)
  })

  if (audiences.error) {
    return <div className="p-6"><ActionableError error={audiences.error} onRetry={() => void audiences.refetch()} /></div>
  }

  const items = audiences.data?.items ?? []
  const canWrite = Boolean(permissions?.segments?.write)

  return (
    <div className="p-6">
      <div className="flex justify-between items-start gap-4 mb-6">
        <div>
          <WorkspacePageTitle style={{ marginBottom: 4 }}>{t`Audience segmentation`}</WorkspacePageTitle>
          <Paragraph type="secondary" className="!mb-0">
            {t`Define reusable audiences from customer attributes, status, lists, activity, and goals.`}
          </Paragraph>
        </div>
        <Space>
          <Link to="/console/workspace/$workspaceId/lists" params={{ workspaceId }}>
            {t`Manage lists`}
          </Link>
          <Button type="primary" icon={<PlusOutlined />} disabled={!canWrite} onClick={() => setDrawerOpen(true)}>
            {t`Create dynamic audience`}
          </Button>
        </Space>
      </div>

      {audiences.isLoading ? (
        <Row gutter={[16, 16]}>{[1, 2, 3].map((key) => <Col xs={24} md={8} key={key}><Card loading /></Col>)}</Row>
      ) : items.length === 0 ? (
        <Empty description={t`No dynamic audiences yet`}>
          <Button type="primary" disabled={!canWrite} onClick={() => setDrawerOpen(true)}>{t`Create dynamic audience`}</Button>
        </Empty>
      ) : (
        <Row gutter={[16, 16]}>
          {items.map((audience) => (
            <Col xs={24} md={12} xl={8} key={audience.id}>
              <Card>
                <Space orientation="vertical" size="small" style={{ width: '100%' }}>
                  <Space wrap>
                    <Text strong>{audience.name}</Text>
                    <Tag color="blue">{t`Dynamic`}</Tag>
                    <Tag>{t`Version ${audience.active_version}`}</Tag>
                  </Space>
                  {audience.description && <Paragraph type="secondary" className="!mb-0">{audience.description}</Paragraph>}
                  <Text type="secondary">{t`Members are evaluated only for previews and when a marketing run starts.`}</Text>
                </Space>
              </Card>
            </Col>
          ))}
        </Row>
      )}

      <AudienceDrawer
        open={drawerOpen}
        workspaceId={workspaceId}
        lists={(lists.data?.lists ?? []).map((list) => ({ id: list.id, name: list.name }))}
        customFieldLabels={workspace?.settings?.custom_field_labels}
        onClose={() => setDrawerOpen(false)}
      />
    </div>
  )
}
