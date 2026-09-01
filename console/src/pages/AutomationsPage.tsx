import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Row, Col, Space, App, Empty, Pagination, Drawer, Button } from 'antd'
import { useParams, useSearch } from '@tanstack/react-router'
import { useEffect, useMemo, useRef, useState } from 'react'
import { PlusOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { automationApi, Automation } from '../services/api/automation'
import { listsApi } from '../services/api/list'
import { listSegments } from '../services/api/segment'
import { templatesApi } from '../services/api/template'
import { useWorkspacePermissions, useAuth } from '../contexts/AuthContext'
import { AutomationCard } from '../components/automations/AutomationCard'
import { UpsertAutomationDrawer } from '../components/automations/UpsertAutomationDrawer'
import { JourneyPreflightPanel } from '../components/automations/JourneyPreflightPanel'
import { ActionableError } from '../components/errors/ActionableError'
import { audienceApi } from '../services/api/marketing'
import { AutomationAudienceRunModal } from '../components/automations/AutomationAudienceRunModal'
import { WorkspacePageTitle } from '../components/navigation/WorkspacePageTitle'

export function AutomationsPage() {
  const { t } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId' })
  const search = useSearch({ from: '/console/workspace/$workspaceId/automations' })
  const { permissions } = useWorkspacePermissions(workspaceId)
  const { workspaces } = useAuth()
  const queryClient = useQueryClient()
  const { message } = App.useApp()

  // Get current workspace
  const currentWorkspace = workspaces.find((w) => w.id === workspaceId)

  // State for editing automation
  const [editingAutomation, setEditingAutomation] = useState<Automation | undefined>(undefined)
  const [editingNodeId, setEditingNodeId] = useState<string | undefined>()
  const [preflightAutomation, setPreflightAutomation] = useState<Automation | undefined>()
  const [operationError, setOperationError] = useState<unknown>()
  const [audienceRunOpen, setAudienceRunOpen] = useState(false)
  const handledTraceFix = useRef('')

  const [currentPage, setCurrentPage] = useState(1)
  const pageSize = 10

  // Fetch automations
  const {
    data: automationsData,
    isLoading: isLoadingAutomations,
    isFetching: isFetchingAutomations,
    error: automationsError,
    refetch: refetchAutomations
  } = useQuery({
    queryKey: ['automations', workspaceId, currentPage, pageSize],
    queryFn: () =>
      automationApi.list({
        workspace_id: workspaceId,
        limit: pageSize,
        offset: (currentPage - 1) * pageSize
      }),
    enabled: !!workspaceId
  })

  // Fetch lists for reference
  const { data: listsData } = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId }),
    enabled: !!workspaceId
  })

  // Fetch segments for reference
  const { data: segmentsData } = useQuery({
    queryKey: ['segments', workspaceId],
    queryFn: () => listSegments({ workspace_id: workspaceId }),
    enabled: !!workspaceId
  })

  // Fetch templates for reference (name display on canvas email nodes).
  // Must include every email template the node picker allows selecting — the
  // picker (EmailConfigForm → TemplateSelectorInput) is category-agnostic, so a
  // category filter here would leave non-marketing templates (e.g. welcome)
  // unresolved and the node would fall back to the generic "Template set" label.
  const { data: templatesData } = useQuery({
    queryKey: ['templates', workspaceId, 'email'],
    queryFn: () => templatesApi.list({ workspace_id: workspaceId, channel: 'email' }),
    enabled: !!workspaceId
  })

  const { data: audiencesData } = useQuery({
    queryKey: ['audiences', workspaceId],
    queryFn: () => audienceApi.list(workspaceId),
    enabled: audienceRunOpen && !!workspaceId
  })

  const automations = useMemo(() => automationsData?.automations || [], [automationsData?.automations])
  const totalAutomations = automationsData?.total || 0
  const lists = listsData?.lists || []
  const segments = segmentsData?.segments || []
  const templates = templatesData?.templates || []

  useEffect(() => {
    if (!search.automation_id) return
    const requestKey = `${search.automation_id}:${search.node_id ?? ''}`
    if (handledTraceFix.current === requestKey) return
    const automation = automations.find((item) => item.id === search.automation_id)
    if (!automation) return
    handledTraceFix.current = requestKey
    setEditingNodeId(search.node_id)
    setEditingAutomation(automation)
  }, [automations, search.automation_id, search.node_id])

  // Handle activate automation
  const handleActivate = (automation: Automation) => {
    setPreflightAutomation(automation)
  }

  // Handle pause automation
  const handlePause = async (automation: Automation) => {
    setOperationError(undefined)
    try {
      await automationApi.pause({
        workspace_id: workspaceId,
        automation_id: automation.id
      })
      message.success(t`Automation paused successfully`)
      queryClient.invalidateQueries({ queryKey: ['automations', workspaceId] })
    } catch (error) {
      setOperationError(error)
    }
  }

  // Handle delete automation
  const handleDelete = async (automation: Automation) => {
    setOperationError(undefined)
    try {
      await automationApi.delete({
        workspace_id: workspaceId,
        automation_id: automation.id
      })
      message.success(t`Automation deleted successfully`)
      queryClient.invalidateQueries({ queryKey: ['automations', workspaceId] })
    } catch (error) {
      setOperationError(error)
    }
  }

  // Handle edit automation
  const handleEdit = (automation: Automation) => {
    setEditingNodeId(undefined)
    setEditingAutomation(automation)
  }

  // Handle edit drawer close
  const handleEditClose = () => {
    setEditingNodeId(undefined)
    setEditingAutomation(undefined)
  }

  // Handle page change
  const handlePageChange = (page: number) => {
    setCurrentPage(page)
    // Scroll to top smoothly
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  if (automationsError) {
    return (
      <div className="p-6">
        <ActionableError
          error={automationsError}
          onRetry={() => void refetchAutomations()}
          retrying={isFetchingAutomations}
        />
      </div>
    )
  }

  return (
    <div>
      <Row justify="space-between" align="middle" className="mb-6">
        <Col>
          <WorkspacePageTitle>{t`Automations`}</WorkspacePageTitle>
        </Col>
        <Col>
          <Space>
            <Button
              disabled={!permissions?.automations?.write || !automations.some((item) => item.status === 'live')}
              onClick={() => setAudienceRunOpen(true)}
            >
              {t`Start from audience`}
            </Button>
            {currentWorkspace && (
              <UpsertAutomationDrawer
                workspace={currentWorkspace}
                lists={lists}
                segments={segments}
                templates={templates}
                buttonProps={{
                  type: 'primary',
                  icon: <PlusOutlined />,
                  disabled: !permissions?.automations?.write
                }}
                buttonContent={t`Create Automation`}
              />
            )}
          </Space>
        </Col>
      </Row>

      {Boolean(operationError) && <ActionableError error={operationError} />}

      {isLoadingAutomations ? (
        <div className="text-center py-12 text-gray-500">{t`Loading automations...`}</div>
      ) : automations.length === 0 ? (
        <Empty
          description={t`No automations yet`}
          className="py-12"
        >
          {currentWorkspace && (
            <UpsertAutomationDrawer
              workspace={currentWorkspace}
              lists={lists}
              segments={segments}
              templates={templates}
              buttonProps={{
                type: 'primary',
                icon: <PlusOutlined />,
                disabled: !permissions?.automations?.write
              }}
              buttonContent={t`Create your first automation`}
            />
          )}
        </Empty>
      ) : (
        <>
          {automations.map((automation) => (
            <AutomationCard
              key={automation.id}
              automation={automation}
              lists={lists}
              segments={segments}
              permissions={permissions}
              workspaceId={workspaceId}
              onActivate={handleActivate}
              onPause={handlePause}
              onDelete={handleDelete}
              onEdit={handleEdit}
            />
          ))}

          {totalAutomations > pageSize && (
            <div className="flex justify-center mt-6">
              <Pagination
                current={currentPage}
                total={totalAutomations}
                pageSize={pageSize}
                onChange={handlePageChange}
                showSizeChanger={false}
                showTotal={(total, range) => t`${range[0]}-${range[1]} of ${total} automations`}
              />
            </div>
          )}
        </>
      )}

      {/* Edit Automation Drawer (controlled) */}
      {currentWorkspace && editingAutomation && (
        <UpsertAutomationDrawer
          workspace={currentWorkspace}
          automation={editingAutomation}
          lists={lists}
          segments={segments}
          templates={templates}
          initialNodeId={editingNodeId}
          open={!!editingAutomation}
          onOpenChange={(open) => {
            if (!open) handleEditClose()
          }}
          onClose={handleEditClose}
        />
      )}

      <Drawer
        title={t`Activation preflight`}
        size={640}
        open={Boolean(preflightAutomation)}
        onClose={() => setPreflightAutomation(undefined)}
        destroyOnHidden
      >
        {preflightAutomation && (
          <JourneyPreflightPanel
            workspaceId={workspaceId}
            automationId={preflightAutomation.id}
            onFixIssue={(issue) => {
              setEditingNodeId(issue.node_id)
              setEditingAutomation(preflightAutomation)
              setPreflightAutomation(undefined)
            }}
            onActivated={() => {
              message.success(t`Automation activated successfully`)
              setPreflightAutomation(undefined)
              void queryClient.invalidateQueries({ queryKey: ['automations', workspaceId] })
            }}
          />
        )}
      </Drawer>

      <AutomationAudienceRunModal
        open={audienceRunOpen}
        workspaceId={workspaceId}
        automations={automations}
        audiences={(audiencesData?.items ?? []).map((item) => ({ id: item.id, name: item.name }))}
        onClose={() => setAudienceRunOpen(false)}
      />
    </div>
  )
}
