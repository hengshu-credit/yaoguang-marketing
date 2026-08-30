import { useState, useEffect, useRef } from 'react'
import { useParams, useNavigate } from '@tanstack/react-router'
import { Layout } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { locales, type Locale } from '../i18n'
import { workspaceService } from '../services/api/workspace'
import { Workspace, WorkspaceMember } from '../services/api/types'
import { WorkspaceMembers } from '../components/settings/WorkspaceMembers'
import { GeneralSettings } from '../components/settings/GeneralSettings'
import { SMTPBridgeSettings } from '../components/settings/SMTPBridgeSettings'
import { Integrations } from '../components/settings/Integrations'
import { CustomFieldsConfiguration } from '../components/settings/CustomFieldsConfiguration'
import { BlogSettings } from '../components/settings/BlogSettings'
import { WebAnalyticsSettings } from '../components/settings/WebAnalyticsSettings'
import { WebhooksSettings } from '../components/settings/WebhooksSettings'
import { UITranslationsSettings } from '../components/settings/UITranslationsSettings'
import { FrequencyPoliciesSettings } from '../components/settings/FrequencyPoliciesSettings'
import { useAuth } from '../contexts/AuthContext'
import { DeleteWorkspaceSection } from '../components/settings/DeleteWorkspace'
import {
  SettingsSidebar,
  SETTINGS_SECTIONS,
  SettingsSection
} from '../components/settings/SettingsSidebar'

const { Sider, Content } = Layout

export function WorkspaceSettingsPage() {
  const { t, i18n } = useLingui()
  const { workspaceId, section } = useParams({
    from: '/console/workspace/$workspaceId/settings/$section'
  })
  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  const [members, setMembers] = useState<WorkspaceMember[]>([])
  const [loadingMembers, setLoadingMembers] = useState(false)
  const [membersLoaded, setMembersLoaded] = useState(false)
  const [memberAccessResolved, setMemberAccessResolved] = useState(false)
  const [isOwner, setIsOwner] = useState(false)
  const [canManageCustomFields, setCanManageCustomFields] = useState(false)
  const [canManageBlog, setCanManageBlog] = useState(false)
  const [canManageWebAnalytics, setCanManageWebAnalytics] = useState(false)
  const { refreshWorkspaces, user, workspaces } = useAuth()
  const navigate = useNavigate()
  const membersRequestGeneration = useRef(0)
  const currentLocale = locales.includes(i18n.locale as Locale)
    ? (i18n.locale as Locale)
    : 'en'

  // Get active section from URL or default to 'team'
  const activeSection: SettingsSection = SETTINGS_SECTIONS.includes(section as SettingsSection)
    ? (section as SettingsSection)
    : 'team'

  useEffect(() => {
    // Redirect to team section if invalid section is provided
    if (!SETTINGS_SECTIONS.includes(section as SettingsSection)) {
      navigate({
        to: '/console/workspace/$workspaceId/settings/$section',
        params: { workspaceId, section: 'team' },
        replace: true
      })
    }
  }, [section, workspaceId, navigate])

  useEffect(() => {
    if (section !== 'languages' || !membersLoaded || !memberAccessResolved || isOwner) return
    navigate({
      to: '/console/workspace/$workspaceId/settings/$section',
      params: { workspaceId, section: 'team' },
      replace: true
    })
  }, [isOwner, memberAccessResolved, membersLoaded, navigate, section, workspaceId])

  useEffect(() => {
    // Find the workspace from the auth context
    const currentWorkspace = workspaces.find((w) => w.id === workspaceId) || null
    setWorkspace(currentWorkspace)
  }, [workspaceId, workspaces])

  useEffect(() => {
    setMembersLoaded(false)
    setMemberAccessResolved(false)
    setMembers([])
    setIsOwner(false)
    setCanManageCustomFields(false)
    setCanManageBlog(false)
    setCanManageWebAnalytics(false)

    void fetchMembers()
    return () => {
      membersRequestGeneration.current += 1
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- fetchMembers is stable
  }, [workspaceId, user?.id])

  const fetchMembers = async () => {
    const generation = ++membersRequestGeneration.current
    const requestedWorkspaceId = workspaceId
    setLoadingMembers(true)
    try {
      const response = await workspaceService.getMembers(requestedWorkspaceId)
      if (generation !== membersRequestGeneration.current) return
      setMembers(response.members)

      // Check if current user is an owner
      if (user) {
        const currentUserMember = response.members.find((member) => member.user_id === user.id)
        setIsOwner(currentUserMember?.role === 'owner')
        // Custom fields can be managed by owners or members with workspace:write permission
        // (mirrors the backend HasPermission(workspace, write) check).
        setCanManageCustomFields(
          currentUserMember?.role === 'owner' ||
            currentUserMember?.permissions?.workspace?.write === true
        )
        // Blog settings can be managed by owners or members with blog:write permission
        // (mirrors the backend HasPermission(blog, write) check).
        setCanManageBlog(
          currentUserMember?.role === 'owner' ||
            currentUserMember?.permissions?.blog?.write === true
        )
        // Web analytics settings can be managed by owners or members with
        // web_analytics:write permission (mirrors the backend
        // HasPermission(web_analytics, write) check).
        setCanManageWebAnalytics(
          currentUserMember?.role === 'owner' ||
            currentUserMember?.permissions?.web_analytics?.write === true
        )
      }
      setMemberAccessResolved(true)
    } catch (error) {
      if (generation !== membersRequestGeneration.current) return
      console.error(t`Failed to fetch workspace members`, error)
    } finally {
      if (generation === membersRequestGeneration.current) {
        setLoadingMembers(false)
        setMembersLoaded(true)
      }
    }
  }

  const handleWorkspaceUpdate = async (updatedWorkspace: Workspace) => {
    setWorkspace(updatedWorkspace)
    // Refresh the workspaces in auth context to stay in sync
    await refreshWorkspaces()
  }

  const handleWorkspaceDelete = async () => {
    navigate({ to: '/console' })
    await refreshWorkspaces()
  }

  const handleSectionChange = (newSection: SettingsSection) => {
    navigate({
      to: '/console/workspace/$workspaceId/settings/$section',
      params: { workspaceId, section: newSection }
    })
  }

  const renderSection = () => {
    switch (activeSection) {
      case 'team':
        return (
          <WorkspaceMembers
            workspaceId={workspaceId}
            members={members}
            loading={loadingMembers}
            onMembersChange={fetchMembers}
            isOwner={isOwner}
          />
        )
      case 'integrations':
        return (
          <Integrations
            workspace={workspace}
            loading={false}
            onSave={handleWorkspaceUpdate}
            isOwner={isOwner}
          />
        )
      case 'frequency':
        return <FrequencyPoliciesSettings workspaceId={workspaceId} />
      case 'webhooks':
        return workspace ? <WebhooksSettings workspaceId={workspace.id} /> : null
      case 'custom-fields':
        return (
          <CustomFieldsConfiguration
            workspace={workspace}
            onWorkspaceUpdate={handleWorkspaceUpdate}
            canManage={canManageCustomFields}
          />
        )
      case 'smtp-bridge':
        return <SMTPBridgeSettings />
      case 'general':
        return (
          <GeneralSettings
            workspace={workspace}
            onWorkspaceUpdate={handleWorkspaceUpdate}
            isOwner={isOwner}
          />
        )
      case 'blog':
        return (
          <BlogSettings
            workspace={workspace}
            onWorkspaceUpdate={handleWorkspaceUpdate}
            canManage={canManageBlog}
          />
        )
      case 'web-analytics':
        return (
          <WebAnalyticsSettings
            workspace={workspace}
            onWorkspaceUpdate={handleWorkspaceUpdate}
            canManage={canManageWebAnalytics}
          />
        )
      case 'languages':
        return workspace && isOwner ? (
          <UITranslationsSettings
            workspace={workspace}
            isOwner={isOwner}
            refreshWorkspaces={refreshWorkspaces}
            currentLocale={currentLocale}
          />
        ) : null
      case 'danger-zone':
        return workspace && isOwner ? (
          <DeleteWorkspaceSection workspace={workspace} onDeleteSuccess={handleWorkspaceDelete} />
        ) : null
      default:
        return null
    }
  }

  return (
    <Layout style={{ minHeight: 'calc(100vh - 48px)' }}>
      <Sider
        width={250}
        style={{
          borderRight: '1px solid #f0f0f0',
          overflow: 'auto'
        }}
      >
        <SettingsSidebar
          activeSection={activeSection}
          onSectionChange={handleSectionChange}
          isOwner={isOwner}
        />
      </Sider>
      <Layout>
        <Content>
          <div
            style={{ maxWidth: activeSection === 'languages' ? '100%' : '700px', padding: '24px' }}
          >
            {renderSection()}
          </div>
        </Content>
      </Layout>
    </Layout>
  )
}
