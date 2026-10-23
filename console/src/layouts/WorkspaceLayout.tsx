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
import { WorkspacePermissionsContext } from '../contexts/WorkspacePermissionsContext'
import { LanguageSwitcher } from '../components/LanguageSwitcher'
import { BrandLockup } from '../components/BrandLockup'
import { Workspace, UserPermissions } from '../services/api/types'
import { ContactsCsvUploadProvider } from '../components/contacts/ContactsCsvUploadProvider'
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent, MouseEvent as ReactMouseEvent } from 'react'
import { FileManagerProvider } from '../components/file_manager/context'
import { FileManagerSettings } from '../components/file_manager/interfaces'
import { workspaceService } from '../services/api/workspace'
import { clearWorkspaceCatalog, setWorkspaceCatalog } from '../i18n/workspaceCatalog'
import { createEmptyPermissions, createFullPermissions } from '../services/api/permissions'
import { isRootUser } from '../services/api/auth'
import { applyConsoleFont } from '../lib/consoleFont'
import { useConsoleTheme } from '../theme/ConsoleThemeProvider'
import { ConsoleThemeDrawer } from '../theme/ConsoleThemeDrawer'
import {
  SIDEBAR_COMPACT_THRESHOLD,
  SIDEBAR_MAX_WIDTH,
  SIDEBAR_MIN_WIDTH,
  readSidebarState,
  resizeSidebarState,
  toggleSidebarState,
  writeSidebarState
} from './sidebarSizing'
import {
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

function browserSessionStorage(): Storage | null {
  try {
    return window.sessionStorage
  } catch {
    return null
  }
}

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
  const currentWorkspace = workspaces.find((workspace) => workspace.id === workspaceId)
  const appearance = useConsoleTheme()
  const [themeWorkspace, setThemeWorkspace] = useState<string>()
  const [themeOwner, setThemeOwner] = useState<{id:string;userId:string;allowed:boolean}>()
  const bindTheme = appearance?.bind
  const savedTheme = currentWorkspace?.settings.console_theme
  const workspaceName = currentWorkspace?.name ?? ''
  useLayoutEffect(() => bindTheme?.(workspaceId,workspaceName,savedTheme),[bindTheme,workspaceId,workspaceName,savedTheme])
  useEffect(() => setThemeWorkspace(undefined),[workspaceId])
  const workspaceTranslations = currentWorkspace?.settings.ui_translations
  const navigate = useNavigate()
  const [viewportWidth, setViewportWidth] = useState(() => window.innerWidth)
  const narrow = viewportWidth < 768
  const compactHeader = viewportWidth < 1024
  const [collapsed, setCollapsed] = useState(() => window.innerWidth < 768)
  const [sidebarState, setSidebarState] = useState(() =>
    readSidebarState(browserSessionStorage())
  )
  const sidebarStateRef = useRef(sidebarState)
  const resizeStartRef = useRef<{ clientX: number; width: number } | null>(null)
  const suppressResizeClickRef = useRef(false)
  const clearResizeClickTimerRef = useRef<number | null>(null)
  const [resizingSidebar, setResizingSidebar] = useState(false)
  const [permissionResult, setPermissionResult] = useState<{ workspaceId: string; userId: string; permissions: UserPermissions }>()
  const userPermissions = permissionResult?.workspaceId === workspaceId && permissionResult.userId === user?.id ? permissionResult.permissions : null
  const loadingPermissions = !!user && !userPermissions
  const permissionSnapshot = useMemo(() => ({ workspaceId, userId: user?.id ?? '', permissions: userPermissions, loading: loadingPermissions }), [workspaceId, user?.id, userPermissions, loadingPermissions])

  const desktopCompact = sidebarState.width < SIDEBAR_COMPACT_THRESHOLD
  const sidebarCompact = narrow ? collapsed : desktopCompact
  const sidebarWidth = narrow
    ? Math.max(SIDEBAR_COMPACT_THRESHOLD, sidebarState.lastExpandedWidth)
    : sidebarState.width

  const applySidebarWidth = useCallback((width: number) => {
    const next = resizeSidebarState(sidebarStateRef.current, width)
    sidebarStateRef.current = next
    setSidebarState(next)
    return next
  }, [])

  const persistSidebarState = useCallback((state = sidebarStateRef.current) => {
    writeSidebarState(browserSessionStorage(), state)
  }, [])

  const handleSidebarResizeStart = (event: ReactMouseEvent<HTMLButtonElement>) => {
    if (narrow || event.button !== 0) return
    event.preventDefault()
    event.stopPropagation()
    suppressResizeClickRef.current = false
    if (clearResizeClickTimerRef.current !== null) {
      window.clearTimeout(clearResizeClickTimerRef.current)
      clearResizeClickTimerRef.current = null
    }
    resizeStartRef.current = { clientX: event.clientX, width: sidebarState.width }
    setResizingSidebar(true)
  }

  const handleSidebarResizeKeyDown = (event: ReactKeyboardEvent<HTMLButtonElement>) => {
    let nextWidth: number | null = null
    if (event.key === 'ArrowLeft') nextWidth = sidebarState.width - 8
    if (event.key === 'ArrowRight') nextWidth = sidebarState.width + 8
    if (event.key === 'Home') nextWidth = SIDEBAR_MIN_WIDTH
    if (event.key === 'End') nextWidth = SIDEBAR_MAX_WIDTH
    if (nextWidth === null) return
    event.preventDefault()
    persistSidebarState(applySidebarWidth(nextWidth))
  }

  const handleSidebarToggle = () => {
    if (narrow) {
      setCollapsed(!collapsed)
      return
    }
    const next = toggleSidebarState(sidebarStateRef.current)
    sidebarStateRef.current = next
    setSidebarState(next)
    persistSidebarState(next)
  }

  // Use useMatches to determine the current route path
  const matches = useMatches()
  const currentPath = matches[matches.length - 1]?.pathname || ''
  const isSettingsPage = currentPath.includes('/settings')

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
    const suppressClickAfterResize = (event: MouseEvent) => {
      if (!suppressResizeClickRef.current) return
      event.preventDefault()
      event.stopImmediatePropagation()
      suppressResizeClickRef.current = false
    }

    window.addEventListener('click', suppressClickAfterResize, true)
    return () => {
      window.removeEventListener('click', suppressClickAfterResize, true)
      if (clearResizeClickTimerRef.current !== null) {
        window.clearTimeout(clearResizeClickTimerRef.current)
      }
    }
  }, [])

  useEffect(() => {
    if (!resizingSidebar) return
    const start = resizeStartRef.current
    if (!start) return

    const previousCursor = document.body.style.cursor
    const previousUserSelect = document.body.style.userSelect
    document.body.style.cursor = 'ew-resize'
    document.body.style.userSelect = 'none'

    const widthAt = (clientX: number) => start.width + clientX - start.clientX
    const handleMouseMove = (event: MouseEvent) => {
      if (Math.abs(event.clientX - start.clientX) >= 3) {
        suppressResizeClickRef.current = true
      }
      applySidebarWidth(widthAt(event.clientX))
    }
    const finishResize = (event: MouseEvent) => {
      persistSidebarState(applySidebarWidth(widthAt(event.clientX)))
      setResizingSidebar(false)
      if (suppressResizeClickRef.current) {
        clearResizeClickTimerRef.current = window.setTimeout(() => {
          suppressResizeClickRef.current = false
          clearResizeClickTimerRef.current = null
        }, 0)
      }
    }

    window.addEventListener('mousemove', handleMouseMove)
    window.addEventListener('mouseup', finishResize)
    return () => {
      window.removeEventListener('mousemove', handleMouseMove)
      window.removeEventListener('mouseup', finishResize)
      document.body.style.cursor = previousCursor
      document.body.style.userSelect = previousUserSelect
      resizeStartRef.current = null
    }
  }, [applySidebarWidth, persistSidebarState, resizingSidebar])

  useEffect(() => {
    if (narrow) setCollapsed(true)
  }, [currentPath, narrow])

  useEffect(
    () =>
      applyConsoleFont(currentWorkspace?.settings.console_font, {
        onError: () => {
          message.warning(t`Uploaded console font could not be loaded. Using the system font instead.`)
        }
      }),
    [currentWorkspace?.settings.console_font, t]
  )

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
    let current = true
    const setUserPermissions = (permissions: UserPermissions) => {
      if (current && user) setPermissionResult({ workspaceId, userId: user.id, permissions })
    }
    const fetchUserPermissions = async () => {
      if (!user || !workspaceId) {
        return
      }

      // If user is root, they have full permissions
      if (isRootUser(user.email)) {
        setThemeOwner({id:workspaceId,userId:user.id,allowed:true})
        setUserPermissions(createFullPermissions())
        return
      }

      try {
        const response = await workspaceService.getMembers(workspaceId)
        if (!current) return
        const currentUserMember = response.members.find((member) => member.user_id === user.id)
        setThemeOwner({id:workspaceId,userId:user.id,allowed:currentUserMember?.role === 'owner'})

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
      }
    }

    fetchUserPermissions()
    return () => { current = false }
  }, [workspaceId, user])

  // Helper function to check if user has access to a resource
  const hasAccess = (resource: keyof UserPermissions): boolean => {
    if (!userPermissions) return false
    // User needs at least read or write permission to access the resource
    const permissions = userPermissions[resource]
    return permissions?.read || permissions?.write || false
  }

  // Determine which key should be selected based on the current path
  let selectedKey = 'data'
  if (currentPath.includes('/settings')) {
    selectedKey = 'settings'
  } else if (currentPath.includes('/customers') || currentPath.includes('/contacts')) {
    selectedKey = 'customers'
  } else if (currentPath.includes('/lists')) {
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
    currentPath.includes('/deliveries') ||
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
    const workspace = currentWorkspace
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
    (hasAccess('message_history') || hasAccess('web_analytics')) && {
      key: 'data',
      icon: <BarChartOutlined />,
      label: (
        <Link
          to={
            hasAccess('message_history')
              ? '/console/workspace/$workspaceId/analytics'
              : '/console/workspace/$workspaceId/web-analytics/$tab'
          }
          params={
            hasAccess('message_history')
              ? { workspaceId }
              : { workspaceId, tab: 'dashboard' }
          }
        >
          {t`Data Analytics`}
        </Link>
      )
    },
    (hasAccess('customers') || hasAccess('contacts')) && {
      key: 'customers',
      icon: <TeamOutlined />,
      label: (
        <Link
          to="/console/workspace/$workspaceId/contacts"
          params={{ workspaceId }}
        >
          {t`Customers`}
        </Link>
      )
    },
    (hasAccess('lists') || hasAccess('segments')) && {
      key: 'audiences',
      icon: <UsergroupAddOutlined />,
      label: (
        <Link to="/console/workspace/$workspaceId/lists" params={{ workspaceId }}>
          {t`Audiences & Lists`}
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
    (hasAccess('message_history') || hasAccess('transactional')) && {
      key: 'delivery',
      icon: <SendOutlined />,
      label: (
        <Link
          to={
            hasAccess('message_history')
              ? '/console/workspace/$workspaceId/deliveries'
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
      <Layout className="console-workspace" style={{ minHeight: '100vh', backgroundColor: 'var(--console-bg, #F9F9F9)' }}>
        <Layout>
          {narrow && !collapsed && (
            <button
              type="button"
              className="workspace-mobile-nav-mask"
              data-testid="workspace-mobile-nav-mask"
              aria-label={t`Collapse`}
              style={{ left: sidebarWidth }}
              onClick={() => setCollapsed(true)}
            />
          )}
          <Sider
            width={sidebarWidth}
            collapsedWidth={narrow ? 0 : sidebarState.width}
            theme={appearance?.value.sidebar_style ?? 'light'}
            style={{
              position: 'fixed',
              height: '100vh',
              left: 0,
              top: 0,
              // The nav inside owns the scrolling; the panel must not also
              // scroll, or the logo and the collapse button travel with it.
              overflow: 'visible',
              zIndex: 12,
              backgroundColor: 'var(--console-sidebar, #F9F9F9)',
              transition: resizingSidebar ? 'none' : undefined
            }}
            collapsible
            collapsed={sidebarCompact}
            trigger={null}
            className={`workspace-sider border-r border-gray-200${
              resizingSidebar ? ' workspace-sider--resizing' : ''
            }`}
          >
            <div
              style={{
                flex: '0 0 auto',
                padding: sidebarCompact ? '14px 0' : '12px 16px',
                borderBottom: '1px solid var(--console-border, #f0f0f0)'
              }}
            >
              <BrandLockup compact={sidebarCompact} style={{ justifyContent: 'center' }} />
            </div>
            <div className="workspace-sider-nav">
              <Menu
                mode="inline"
                inlineCollapsed={sidebarCompact}
                selectedKeys={[selectedKey]}
                style={{
                  borderRight: 0,
                  backgroundColor: 'var(--console-sidebar, #F9F9F9)',
                  fontSize: '13px',
                  // Item labels are <Link> anchors, which index.css pins to 500.
                  // Submenu titles are plain text and inherit this instead, so it
                  // has to match or the group rows read heavier than the rest.
                  fontWeight: 500
                }}
                items={loadingPermissions ? [] : menuItems}
                theme={appearance?.value.sidebar_style ?? 'light'}
              />
            </div>
            <div
              style={{
                flex: '0 0 auto',
                padding: sidebarCompact ? '8px 10px' : '16px',
                borderTop: '1px solid var(--console-border, #f0f0f0)',
                backgroundColor: 'var(--console-sidebar, #F9F9F9)'
              }}
            >
              {!sidebarCompact && (
                <div
                  style={{
                    textAlign: 'center',
                    fontSize: '9px',
                    color: 'var(--console-sidebar-text, #000)',
                    opacity: 0.7,
                    marginBottom: '8px'
                  }}
                >
                  v{window.VERSION || '1.0'}
                </div>
              )}
              <Button
                type="text"
                block
                aria-label={sidebarCompact ? t`Expand` : t`Collapse`}
                icon={<FontAwesomeIcon icon={sidebarCompact ? faAngleRight : faAngleLeft} />}
                onClick={handleSidebarToggle}
              >
                {!sidebarCompact && t`Collapse`}
              </Button>
            </div>
            {!narrow && (
              <button
                type="button"
                className="workspace-sider-resizer"
                role="separator"
                aria-label={t`Resize sidebar`}
                aria-orientation="vertical"
                aria-valuemin={SIDEBAR_MIN_WIDTH}
                aria-valuemax={SIDEBAR_MAX_WIDTH}
                aria-valuenow={sidebarState.width}
                onMouseDown={handleSidebarResizeStart}
                onKeyDown={handleSidebarResizeKeyDown}
              />
            )}
          </Sider>
          {resizingSidebar && (
            <div
              className="workspace-sidebar-resize-shield"
              data-testid="workspace-sidebar-resize-shield"
              aria-hidden="true"
            />
          )}
          <Header
            className="workspace-header"
            style={{
              position: 'fixed',
              top: 0,
              right: 0,
              width: narrow ? '100%' : `calc(100% - ${sidebarState.width}px)`,
              height: '64px',
              backgroundColor: 'var(--console-surface, #F9F9F9)',
              borderBottom: '1px solid var(--console-border, #f0f0f0)',
              padding: narrow ? '0 12px' : '0 24px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              zIndex: 10,
              transition: resizingSidebar ? 'none' : 'width 0.2s'
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
                    { key: 'theme', label: t`Theme settings`, icon: <SettingOutlined />, onClick: () => setThemeWorkspace(workspaceId) },
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
                <Button type="text" aria-label={t`Account menu`}>
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
              marginLeft: narrow ? '0px' : `${sidebarState.width}px`,
              marginTop: '64px',
              padding: isSettingsPage ? '0' : narrow ? '16px' : 'var(--console-page-padding, 24px)',
              transition: resizingSidebar ? 'none' : 'margin-left 0.2s',
              backgroundColor: 'var(--console-bg, #F9F9F9)',
              minWidth: 0
            }}
          >
            <Content className="workspace-page-content" style={{ backgroundColor: 'var(--console-bg, #F9F9F9)', minWidth: 0 }}>
              <FileManagerProvider
                key={`fm-${workspaceId}-${!userPermissions?.templates?.write}`}
                settings={currentWorkspace?.settings.file_manager}
                onUpdateSettings={handleUpdateWorkspaceSettings}
                readOnly={!userPermissions?.templates?.write}
              >
                <WorkspacePermissionsContext.Provider value={permissionSnapshot}>
                  <Outlet />
                </WorkspacePermissionsContext.Provider>
                {themeWorkspace === workspaceId && currentWorkspace && <ConsoleThemeDrawer key={workspaceId} workspace={currentWorkspace} canManage={themeOwner?.id === workspaceId && themeOwner.userId === user?.id && themeOwner.allowed} onClose={() => setThemeWorkspace(undefined)} onSaved={refreshWorkspaces} />}
              </FileManagerProvider>
            </Content>
          </Layout>
        </Layout>
      </Layout>
    </ContactsCsvUploadProvider>
  )
}
