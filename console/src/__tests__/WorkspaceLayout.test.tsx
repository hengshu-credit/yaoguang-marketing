import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { I18nProvider } from '@lingui/react'
import { i18n, loadLocale } from '../i18n'
import { WorkspaceLayout } from '../layouts/WorkspaceLayout'
import { useAuth } from '../contexts/AuthContext'
import { useNavigate } from '@tanstack/react-router'
import { isRootUser } from '../services/api/auth'
import { workspaceService } from '../services/api/workspace'

vi.mock('../contexts/AuthContext', () => ({
  useAuth: vi.fn()
}))

vi.mock('../i18n/locales/zh-CN.po', () => ({
  messages: { 'Data Analytics': ['数据分析'] }
}))

vi.mock('../i18n/locales/en.po', () => ({
  messages: {}
}))

// Read at render time, so a test can put the layout on any route before rendering.
let currentPathname = '/console/workspace/ws1'

vi.mock('@tanstack/react-router', () => ({
  Outlet: () => <div data-testid="outlet" />,
  Link: ({ children, to }: { children: ReactNode; to: string }) => <a data-to={to}>{children}</a>,
  useParams: () => ({ workspaceId: 'ws1' }),
  useMatches: () => [{ pathname: currentPathname }],
  useNavigate: vi.fn()
}))

vi.mock('../services/api/auth', () => ({
  isRootUser: vi.fn()
}))

vi.mock('../services/api/workspace', () => ({
  workspaceService: {
    getMembers: vi.fn(),
    update: vi.fn()
  }
}))

// Providers pulling in react-query and the file manager are noise here.
vi.mock('../components/contacts/ContactsCsvUploadProvider', () => ({
  ContactsCsvUploadProvider: ({ children }: { children: ReactNode }) => <>{children}</>
}))
vi.mock('../components/file_manager/context', () => ({
  FileManagerProvider: ({ children }: { children: ReactNode }) => <>{children}</>
}))
vi.mock('../components/LanguageSwitcher', () => ({
  LanguageSwitcher: () => null
}))

const mockNavigate = vi.fn()

const grantAll = (value: boolean) => ({
  customers: { read: value, write: value },
  contacts: { read: value, write: value },
  lists: { read: value, write: value },
  templates: { read: value, write: value },
  broadcasts: { read: value, write: value },
  transactional: { read: value, write: value },
  workspace: { read: value, write: value },
  message_history: { read: value, write: value },
  blog: { read: value, write: value },
  automations: { read: value, write: value },
  llm: { read: value, write: value },
  web_analytics: { read: value, write: value }
})

const signInAsRoot = (settings: Record<string, unknown> = {}) => {
  vi.clearAllMocks()
  Object.defineProperty(window, 'innerWidth', {
    configurable: true,
    writable: true,
    value: 1024
  })
  currentPathname = '/console/workspace/ws1'
  vi.mocked(useNavigate).mockReturnValue(mockNavigate as never)
  vi.mocked(isRootUser).mockReturnValue(true)
  vi.mocked(useAuth).mockReturnValue({
    signout: vi.fn(),
    refreshWorkspaces: vi.fn(),
    workspaces: [{ id: 'ws1', name: 'Workspace One', settings }],
    user: { id: 'u1', email: 'root@example.com' }
  } as never)
}

describe('WorkspaceLayout workspace catalog scope', () => {
  beforeEach(signInAsRoot)

  it('renders a workspace sidebar override and restores the bundled value after unmount', async () => {
    await loadLocale('zh-CN')
    signInAsRoot({
      ui_translations: {
        'zh-CN': { 'Data Analytics': '自定义数据分析' }
      }
    })

    const view = render(
      <I18nProvider i18n={i18n}>
        <WorkspaceLayout />
      </I18nProvider>
    )

    await waitFor(() => {
      expect(i18n._('Data Analytics')).toBe('自定义数据分析')
    })
    view.rerender(
      <I18nProvider i18n={i18n}>
        <WorkspaceLayout />
      </I18nProvider>
    )
    expect(screen.getByText('自定义数据分析')).toBeInTheDocument()

    view.unmount()
    signInAsRoot()
    const restoredView = render(
      <I18nProvider i18n={i18n}>
        <WorkspaceLayout />
      </I18nProvider>
    )

    await waitFor(() => {
      expect(i18n._('Data Analytics')).toBe('数据分析')
    })
    restoredView.rerender(
      <I18nProvider i18n={i18n}>
        <WorkspaceLayout />
      </I18nProvider>
    )
    expect(screen.getByText('数据分析')).toBeInTheDocument()
  })
})

describe('WorkspaceLayout product navigation', () => {
  beforeEach(() => {
    i18n.activate('en')
    signInAsRoot()
  })

  it('shows the approved Yaoguang brand lockup', () => {
    render(<WorkspaceLayout />)

    expect(screen.getByRole('img', { name: '衡枢真信' })).toBeInTheDocument()
    expect(screen.getByText('瑶光营销平台')).toBeInTheDocument()
    expect(screen.getByText('观心知意，循光达客')).toBeInTheDocument()
  })

  it('renders the eight merged flat first-level entries without a duplicate dashboard', async () => {
    render(<WorkspaceLayout />)

    const expectedEntries = [
      'Data Analytics',
      'Customers',
      'Audiences',
      'Marketing Campaigns',
      'Automation Journeys',
      'Content Center',
      'Delivery Center',
      'Settings'
    ]

    for (const entry of expectedEntries) {
      expect(await screen.findByText(entry)).toBeInTheDocument()
    }
    expect(screen.queryByText('Dashboard')).not.toBeInTheDocument()
    expect(document.querySelectorAll('.workspace-sider-nav .ant-menu-item')).toHaveLength(8)
    expect(document.querySelector('.workspace-sider-nav .ant-menu-submenu')).not.toBeInTheDocument()
    expect(document.querySelector('.workspace-sider-nav .ant-menu-item')?.textContent).toBe('Data Analytics')
  })

  it('links the audiences navigation item to the condition audience workspace', async () => {
    render(<WorkspaceLayout />)

    const audienceEntry = await screen.findByText('Audiences')
    expect(audienceEntry.closest('a')).toHaveAttribute(
      'data-to',
      '/console/workspace/$workspaceId/audiences'
    )
  })

  const selectedLabel = () => document.querySelector('.ant-menu-item-selected')?.textContent ?? null

  it.each([
    ['', 'Data Analytics'],
    ['/customers', 'Customers'],
    ['/contacts', 'Customers'],
    ['/audiences', 'Audiences'],
    ['/lists', 'Audiences'],
    ['/broadcasts', 'Marketing Campaigns'],
    ['/automations', 'Automation Journeys'],
    ['/templates', 'Content Center'],
    ['/blog', 'Content Center'],
    ['/file-manager', 'Content Center'],
    ['/analytics', 'Data Analytics'],
    ['/web-analytics/annotations', 'Data Analytics'],
    ['/deliveries', 'Delivery Center'],
    ['/logs', 'Delivery Center'],
    ['/transactional-notifications', 'Delivery Center'],
    ['/settings/team', 'Settings']
  ])('maps the legacy %s route to %s', async (path, label) => {
    currentPathname = `/console/workspace/ws1${path}`
    render(<WorkspaceLayout />)

    await screen.findByText(label)
    expect(selectedLabel()).toBe(label)
  })

  it('collapses navigation without shifting content on a narrow viewport', async () => {
    Object.defineProperty(window, 'innerWidth', {
      configurable: true,
      writable: true,
      value: 375
    })

    render(<WorkspaceLayout />)

    const toggle = screen.getByTestId('workspace-mobile-nav-toggle')
    const pageLayout = document.querySelector('.workspace-main-layout')
    expect(toggle).toBeVisible()
    expect(document.querySelector('.workspace-sider')).toHaveClass('ant-layout-sider-collapsed')
    expect((pageLayout as HTMLElement).style.marginLeft).toBe('0px')

    await userEvent.click(toggle)
    expect(document.querySelector('.workspace-sider')).not.toHaveClass('ant-layout-sider-collapsed')
    expect((pageLayout as HTMLElement).style.marginLeft).toBe('0px')
    expect(screen.getByTestId('workspace-mobile-nav-mask')).toBeVisible()

    await userEvent.click(screen.getByTestId('workspace-mobile-nav-mask'))
    expect(document.querySelector('.workspace-sider')).toHaveClass('ant-layout-sider-collapsed')
  })

  it('uses the content fallback route when the member cannot read templates', async () => {
    vi.mocked(isRootUser).mockReturnValue(false)
    vi.mocked(workspaceService.getMembers).mockResolvedValue({
      members: [
        {
          user_id: 'u1',
          permissions: { ...grantAll(true), templates: { read: false, write: false } }
        }
      ]
    } as never)

    render(<WorkspaceLayout />)
    const contentEntry = await screen.findByText('Content Center')

    expect(contentEntry.closest('a')).toHaveAttribute(
      'data-to',
      '/console/workspace/$workspaceId/blog'
    )
  })

  it('opens marketing overview when message history is the available analytics capability', async () => {
    vi.mocked(isRootUser).mockReturnValue(false)
    vi.mocked(workspaceService.getMembers).mockResolvedValue({
      members: [
        {
          user_id: 'u1',
          permissions: {
            ...grantAll(false),
            message_history: { read: true, write: false }
          }
        }
      ]
    } as never)

    render(<WorkspaceLayout />)
    const dataEntry = await screen.findByText('Data Analytics')

    expect(dataEntry.closest('a')).toHaveAttribute(
      'data-to',
      '/console/workspace/$workspaceId/analytics'
    )
  })

  it('falls back to website overview when message history is unavailable', async () => {
    vi.mocked(isRootUser).mockReturnValue(false)
    vi.mocked(workspaceService.getMembers).mockResolvedValue({
      members: [
        {
          user_id: 'u1',
          permissions: {
            ...grantAll(false),
            web_analytics: { read: true, write: false }
          }
        }
      ]
    } as never)

    render(<WorkspaceLayout />)
    const dataEntry = await screen.findByText('Data Analytics')

    expect(dataEntry.closest('a')).toHaveAttribute(
      'data-to',
      '/console/workspace/$workspaceId/web-analytics/$tab'
    )
  })
})
