import { Layout, Menu, Select, Space, Button, Dropdown, message, Avatar } from 'antd'
import type { MenuProps } from 'antd'
import { Outlet, Link, useParams, useMatches, useNavigate } from '@tanstack/react-router'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { useLingui } from '@lingui/react/macro'
import md5 from 'blueimp-md5'
import {
  faFileLines,
  faQuestionCircle
} from '@fortawesome/free-regular-svg-icons'
import {
  faPlus,
  faPowerOff,
  faAngleLeft,
  faAngleRight
} from '@fortawesome/free-solid-svg-icons'
import { useAuth } from '../contexts/AuthContext'
import { LanguageSwitcher } from '../components/LanguageSwitcher'
import { BrandLockup } from '../components/BrandLockup'
import { Workspace, UserPermissions } from '../services/api/types'
import { ContactsCsvUploadProvider } from '../components/contacts/ContactsCsvUploadProvider'
import { useState, useEffect } from 'react'
import { FileManagerProvider } from '../components/file_manager/context'
import { FileManagerSettings } from '../components/file_manager/interfaces'
import { workspaceService } from '../services/api/workspace'
import { clearWorkspaceCatalog, setWorkspaceCatalog } from '../i18n/workspaceCatalog'
import { createEmptyPermissions, createFullPermissions } from '../services/api/permissions'
import { isRootUser } from '../services/api/auth'
import {
  AppstoreOutlined,
  BarChartOutlined,
  FileTextOutlined,
  RocketOutlined,
  SendOutlined,
  SettingOutlined,
  ThunderboltOutlined,
  WarningOutlined,
  DownOutlined,
  MenuOutlined,
  TeamOutlined,
  UsergroupAddOutlined
} from '@ant-design/icons'

const { Content, Sider, Header } = Layout

// Helper function to generate Gravatar URL from email
const getGravatarUrl = (email: string | undefined, size: number = 32): string => {
  if (!email) return ''
  const hash = md5(email.trim().toLowerCase())
  return `https://www.gravatar.com/avatar/${hash}?s=${size}&d=identicon`
}

export function WorkspaceLayout() {
  const { t } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId' })
  const { signout, workspaces, user, refreshWorkspaces } = useAuth()
  const workspaceTranslations = workspaces.find((workspace) => workspace.id === workspaceId)?.settings.ui_translations
  const navigate = useNavigate()
  const [viewportWidth, setViewportWidth] = useState(() => window.innerWidth)
  const narrow = viewportWidth < 768
  const compactHeader = viewportWidth < 1024
  const [collapsed, setCollapsed] = useState(() => window.innerWidth < 768)
  const [userPermissions, setUserPermissions] = useState<UserPermissions | null>(null)
  const [loadingPermissions, setLoadingPermissions] = useState(true)

  // Use useMatches to determine the current route path
  const matches = useMatches()
  const currentPath = matches[matches.length - 1]?.pathname || ''
  const isSettingsPage = currentPath.includes('/settings') || currentPath.includes('/blog')

  useEffect(() => {
    const updateViewport = () => {
      const nextWidth = window.innerWidth
      const nextNarrow = nextWidth < 768
      setViewportWidth(nextWidth)
      setCollapsed(nextNarrow)
    }
    window.addEventListener('resize', updateViewport)
    return () => window.removeEventListener('resize', updateViewport)
  }, [])

  useEffect(() => {
    if (narrow) setCollapsed(true)
  }, [currentPath, narrow])

  useEffect(() => {
    void setWorkspaceCatalog(workspaceId, workspaceTranslations ?? {}).catch((error) => {
      console.error('Failed to apply workspace translations', error)
    })

    return () => {
      void clearWorkspaceCatalog(workspaceId).catch((error) => {
        console.error('Failed to clear workspace translations', error)
      })
    }
  }, [workspaceId, workspaceTranslations])

  // Fetch user permissions for the current workspace
  useEffect(() => {
    const fetchUserPermissions = async () => {
      if (!user || !workspaceId) {
        setLoadingPermissions(false)
        return
      }

      // If user is root, they have full permissions
      if (isRootUser(user.email)) {
        setUserPermissions(createFullPermissions())
        setLoadingPermissions(false)
        return
      }

      try {
        const response = await workspaceService.getMembers(workspaceId)
        const currentUserMember = response.members.find((member) => member.user_id === user.id)

        if (currentUserMember) {
          // The stored map may be partial or null; a resource it does not mention is denied,
          // which is what the empty base spells out.
          setUserPermissions({ ...createEmptyPermissions(), ...currentUserMember.permissions })
        } else {
          // User is not a member of this workspace, set empty permissions
          setUserPermissions(createEmptyPermissions())
        }
      } catch (error) {
        console.error('Failed to fetch user permissions', error)
        // On error, assume no permissions
        setUserPermissions(createEmptyPermissions())
      } finally {
        setLoadingPermissions(false)
      }
    }

    fetchUserPermissions()
  }, [workspaceId, user])

  // Helper function to check if user has access to a resource
  const hasAccess = (resource: keyof UserPermissions): boolean => {
    if (!userPermissions) return false
    // User needs at least read or write permission to access the resource
    const permissions = userPermissions[resource]
    return permissions?.read || permissions?.write || false
  }

  // Determine which key should be selected based on the current path
  let selectedKey = 'dashboard'
  if (currentPath.includes('/settings')) {
    selectedKey = 'settings'
  } else if (currentPath.includes('/customers') || currentPath.includes('/contacts')) {
    selectedKey = 'customers'
  } else if (currentPath.includes('/lists') || currentPath.includes('/audiences')) {
    selectedKey = 'audiences'
  } else if (currentPath.includes('/broadcasts')) {
    selectedKey = 'campaigns'
  } else if (currentPath.includes('/automations')) {
    selectedKey = 'journeys'
  } else if (
    currentPath.includes('/templates') ||
    currentPath.includes('/blog') ||
    currentPath.includes('/file-manager')
  ) {
    selectedKey = 'content'
  } else if (currentPath.includes('/web-analytics') || currentPath.includes('/analytics')) {
    selectedKey = 'data'
  } else if (
    currentPath.includes('/logs') ||
    currentPath.includes('/transactional-notifications')
  ) {
    selectedKey = 'delivery'
  }

  const handleWorkspaceChange = (workspaceId: string) => {
    if (workspaceId === 'new-workspace') {
      // Navigate to workspace creation page or open a modal
      navigate({ to: '/console/workspace/create' })
      return
    }

    navigate({
      to: '/console/workspace/$workspaceId',
      params: { workspaceId }
    })
  }

  // Function to handle workspace settings update
  const handleUpdateWorkspaceSettings = async (settings: FileManagerSettings): Promise<void> => {
    const workspace = workspaces.find((w) => w.id === workspaceId)
    if (!workspace) {
      message.error(t`Workspace not found`)
      return
    }

    try {
      // Update workspace using workspace service
      await workspaceService.update({
        id: workspace.id,
        name: workspace.name,
        settings: {
          ...workspace.settings,
          file_manager: settings
        }
      })

      // Refresh workspaces from context
      await refreshWorkspaces()

      message.success(t`Workspace settings updated successfully`)
    } catch (error: unknown) {
      console.error('Error updating workspace settings:', error)
      const errorMessage = error instanceof Error ? error.message : t`Unknown error`
      message.error(t`Failed to update workspace settings: ${errorMessage}`)
    }
  }

  const menuItems = [
    hasAccess('message_history') && {
      key: 'dashboard',
      icon: <AppstoreOutlined />,
      label: (
        <Link to="/console/workspace/$workspaceId" params={{ workspaceId }}>
          {t`Dashboard`}
        </Link>
      )
    },
    (hasAccess('customers') || hasAccess('contacts')) && {
      key: 'customers',
      icon: <TeamOutlined />,
      label: (
        <Link
          to={
            hasAccess('customers')
              ? '/console/workspace/$workspaceId/customers'
              : '/console/workspace/$workspaceId/contacts'
          }
          params={{ workspaceId }}
        >
          {t`Customers`}
        </Link>
      )
    },
    hasAccess('lists') && {
      key: 'audiences',
      icon: <UsergroupAddOutlined />,
      label: (
        <Link to="/console/workspace/$workspaceId/audiences" params={{ workspaceId }}>
          {t`Audiences`}
        </Link>
      )
    },
    hasAccess('broadcasts') && {
      key: 'campaigns',
      icon: <RocketOutlined />,
      label: (
        <Link to="/console/workspace/$workspaceId/broadcasts" params={{ workspaceId }}>
          {t`Marketing Campaigns`}
        </Link>
      )
    },
    hasAccess('automations') && {
      key: 'journeys',
      icon: <ThunderboltOutlined />,
      label: (
        <Link to="/console/workspace/$workspaceId/automations" params={{ workspaceId }}>
          {t`Automation Journeys`}
        </Link>
      )
    },
    (hasAccess('templates') || hasAccess('workspace')) && {
      key: 'content',
      icon: <FileTextOutlined />,
      label: (
        <Link
          to={
            hasAccess('templates')
              ? '/console/workspace/$workspaceId/templates'
              : '/console/workspace/$workspaceId/blog'
          }
          params={{ workspaceId }}
        >
          {t`Content Center`}
        </Link>
      )
    },
    hasAccess('web_analytics') && {
      key: 'data',
      icon: <BarChartOutlined />,
      label: (
        <Link
          to="/console/workspace/$workspaceId/web-analytics/$tab"
          params={{ workspaceId, tab: 'dashboard' }}
        >
          {t`Data Analytics`}
        </Link>
      )
    },
    (hasAccess('message_history') || hasAccess('transactional')) && {
      key: 'delivery',
      icon: <SendOutlined />,
      label: (
        <Link
          to={
            hasAccess('message_history')
              ? '/console/workspace/$workspaceId/logs'
              : '/console/workspace/$workspaceId/transactional-notifications'
          }
          params={{ workspaceId }}
        >
          {t`Delivery Center`}
        </Link>
      )
    },
    hasAccess('workspace') && {
      key: 'settings',
      icon: <SettingOutlined />,
      label: (
        <Link to="/console/workspace/$workspaceId/settings" params={{ workspaceId }}>
          {t`Settings`}
        </Link>
      )
    }
  ].filter((item) => Boolean(item)) as MenuProps['items']

  return (
    <ContactsCsvUploadProvider>
      <Layout style={{ minHeight: '100vh', backgroundColor: '#F9F9F9' }}>
        <Layout>
          {narrow && !collapsed && (
            <button
              type="button"
              className="workspace-mobile-nav-mask"
              data-testid="workspace-mobile-nav-mask"
              aria-label={t`Collapse`}
              onClick={() => setCollapsed(true)}
            />
          )}
          <Sider
            width={250}
            collapsedWidth={narrow ? 0 : 80}
            theme="light"
            style={{
              position: 'fixed',
              height: '100vh',
              left: 0,
              top: 0,
              // The nav inside owns the scrolling; the panel must not also
              // scroll, or the logo and the collapse button travel with it.
              overflow: 'hidden',
              zIndex: 12,
              backgroundColor: '#F9F9F9'
            }}
            collapsible
            collapsed={collapsed}
            trigger={null}
            className="workspace-sider border-r border-gray-200"
          >
            <div
              style={{
                flex: '0 0 auto',
                padding: collapsed ? '14px 0' : '12px 16px',
                borderBottom: '1px solid #f0f0f0'
              }}
            >
              <BrandLockup compact={collapsed} style={{ justifyContent: 'center' }} />
            </div>
            <div className="workspace-sider-nav">
              <Menu
                mode="inline"
                selectedKeys={[selectedKey]}
                style={{
                  borderRight: 0,
                  backgroundColor: '#F9F9F9',
                  fontSize: '13px',
                  // Item labels are <Link> anchors, which index.css pins to 500.
                  // Submenu titles are plain text and inherit this instead, so it
                  // has to match or the group rows read heavier than the rest.
                  fontWeight: 500
                }}
                items={loadingPermissions ? [] : menuItems}
                theme="light"
              />
            </div>
            <div
              style={{
                flex: '0 0 auto',
                padding: '16px',
                borderTop: '1px solid #f0f0f0',
                backgroundColor: '#F9F9F9'
              }}
            >
              <div
                style={{
                  textAlign: 'center',
                  fontSize: '9px',
                  color: '#000',
                  opacity: 0.7,
                  marginBottom: '8px'
                }}
              >
                v{window.VERSION || '1.0'}
              </div>
              <Button
                type="text"
                block
                icon={<FontAwesomeIcon icon={collapsed ? faAngleRight : faAngleLeft} />}
                onClick={() => setCollapsed(!collapsed)}
              >
                {!collapsed && t`Collapse`}
              </Button>
            </div>
          </Sider>
          <Header
            className="workspace-header"
            style={{
              position: 'fixed',
              top: 0,
              right: 0,
              width: narrow ? '100%' : `calc(100% - ${collapsed ? '80px' : '250px'})`,
              height: '64px',
              backgroundColor: '#F9F9F9',
              borderBottom: '1px solid #f0f0f0',
              padding: narrow ? '0 12px' : '0 24px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              zIndex: 10,
              transition: 'width 0.2s'
            }}
          >
            <Space size="small" style={{ minWidth: 0 }}>
              {narrow && (
                <Button
                  data-testid="workspace-mobile-nav-toggle"
                  type="text"
                  icon={<MenuOutlined />}
                  aria-label={t`Expand`}
                  onClick={() => setCollapsed(false)}
                />
              )}
              <Select
                value={workspaceId}
                variant="filled"
                onChange={handleWorkspaceChange}
                style={{
                  width: narrow ? 'min(180px, calc(100vw - 116px))' : '200px'
                }}
                placeholder={t`Select workspace`}
                options={[
                  ...workspaces.map((workspace: Workspace) => ({
                    label: (
                      <Space size="small">
                        {workspace.settings.logo_url && (
                          <img
                            src={workspace.settings.logo_url}
                            alt=""
                            style={{
                              height: '14px',
                              width: '14px',
                              objectFit: 'contain',
                              verticalAlign: 'middle',
                              display: 'inline-block'
                            }}
                          />
                        )}
                        {workspace.name}
                      </Space>
                    ),
                    value: workspace.id
                  })),
                  ...(isRootUser(user?.email)
                    ? [
                        {
                          label: (
                            <Space className="text-indigo-500">
                              <FontAwesomeIcon icon={faPlus} /> {t`New workspace`}
                            </Space>
                          ),
                          value: 'new-workspace'
                        }
                      ]
                    : [])
                ]}
              />
            </Space>
            <Space size="middle" className="workspace-header-actions">
              {!compactHeader && (
                <Dropdown
                  trigger={['click']}
                  menu={{
                    items: [
                      {
                        key: 'docs',
                        label: (
                          <a
                            href="https://github.com/hengshu-credit/yaoguang-marketing#readme"
                            target="_blank"
                            rel="noopener noreferrer"
                          >
                            <FontAwesomeIcon icon={faFileLines} className="mr-2" />{' '}
                            {t`Documentation`}
                          </a>
                        )
                      },
                      {
                        key: 'report-issue',
                        label: (
                          <a
                            href="https://github.com/hengshu-credit/yaoguang-marketing/issues"
                            target="_blank"
                            rel="noopener noreferrer"
                          >
                            <WarningOutlined className="mr-2" />
                            {t`Report An Issue`}
                          </a>
                        )
                      }
                    ]
                  }}
                  placement="bottomRight"
                >
                  <Button
                    color="default"
                    variant="filled"
                    icon={<FontAwesomeIcon icon={faQuestionCircle} />}
                  >
                    {t`Help`}
                  </Button>
                </Dropdown>
              )}
              {!compactHeader && <LanguageSwitcher />}
              <Dropdown
                menu={{
                  items: [
                    {
                      key: 'logout',
                      label: (
                        <Space>
                          <FontAwesomeIcon icon={faPowerOff} size="sm" style={{ opacity: 0.7 }} />
                          {t`Logout`}
                        </Space>
                      ),
                      onClick: () => signout()
                    }
                  ]
                }}
                trigger={['click']}
                placement="bottomRight"
              >
                <Button type="text">
                  <Space size="small">
                    <Avatar src={getGravatarUrl(user?.email)} size={24} />
                    {!compactHeader && user?.email}
                    {!compactHeader && <DownOutlined style={{ fontSize: '10px' }} />}
                  </Space>
                </Button>
              </Dropdown>
            </Space>
          </Header>
          <Layout
            className="workspace-main-layout"
            style={{
              marginLeft: narrow ? '0px' : collapsed ? '80px' : '250px',
              marginTop: '64px',
              padding: isSettingsPage ? '0' : narrow ? '16px' : '24px',
              transition: 'margin-left 0.2s',
              backgroundColor: '#F9F9F9',
              minWidth: 0
            }}
          >
            <Content style={{ backgroundColor: '#F9F9F9', minWidth: 0 }}>
              <FileManagerProvider
                key={`fm-${workspaceId}-${!userPermissions?.templates?.write}`}
                settings={workspaces.find((w) => w.id === workspaceId)?.settings.file_manager}
                onUpdateSettings={handleUpdateWorkspaceSettings}
                readOnly={!userPermissions?.templates?.write}
              >
                <Outlet />
              </FileManagerProvider>
            </Content>
          </Layout>
        </Layout>
      </Layout>
    </ContactsCsvUploadProvider>
  )
}
