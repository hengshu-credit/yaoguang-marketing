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
  messages: { Dashboard: ['工作台'] }
}))

// Read at render time, so a test can put the layout on any route before rendering.
let currentPathname = '/console/workspace/ws1'

vi.mock('@tanstack/react-router', () => ({
  Outlet: () => <div data-testid="outlet" />,
  Link: ({ children }: { children: ReactNode }) => <a>{children}</a>,
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
        'zh-CN': { Dashboard: '自定义工作台' }
      }
    })

    const view = render(
      <I18nProvider i18n={i18n}>
        <WorkspaceLayout />
      </I18nProvider>
    )

    await waitFor(() => {
      expect(i18n._('Dashboard')).toBe('自定义工作台')
    })
    view.rerender(
      <I18nProvider i18n={i18n}>
        <WorkspaceLayout />
      </I18nProvider>
    )
    expect(screen.getByText('自定义工作台')).toBeInTheDocument()

    view.unmount()
    signInAsRoot()
    const restoredView = render(
      <I18nProvider i18n={i18n}>
        <WorkspaceLayout />
      </I18nProvider>
    )

    await waitFor(() => {
      expect(i18n._('Dashboard')).toBe('工作台')
    })
    restoredView.rerender(
      <I18nProvider i18n={i18n}>
        <WorkspaceLayout />
      </I18nProvider>
    )
    expect(screen.getByText('工作台')).toBeInTheDocument()
  })
})

describe('WorkspaceLayout sidebar groups', () => {
  beforeEach(signInAsRoot)

  it('shows the approved Yaoguang brand lockup', () => {
    render(<WorkspaceLayout />)

    expect(screen.getByRole('img', { name: '恒数科技' })).toBeInTheDocument()
    expect(screen.getByText('瑶光营销平台')).toBeInTheDocument()
    expect(screen.getByText('观心知意，循光达客')).toBeInTheDocument()
  })

  it('shows and highlights the Customer authority entry', async () => {
    currentPathname = '/console/workspace/ws1/customers'
    render(<WorkspaceLayout />)

    await screen.findByText('Customers')
    expect(document.querySelector('.ant-menu-item-selected')?.textContent).toBe('Customers')
  })

  const openGroup = async (label: string) => {
    render(<WorkspaceLayout />)
    const title = await screen.findByText(label)
    await userEvent.click(title)
    return title
  }

  it('opens the web analytics dashboard when the collapsed group is clicked', async () => {
    await openGroup('Web Analytics')

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/console/workspace/$workspaceId/web-analytics/$tab',
      params: { workspaceId: 'ws1', tab: 'dashboard' }
    })
  })

  it('does not navigate when the open web analytics group is clicked shut', async () => {
    const title = await openGroup('Web Analytics')
    mockNavigate.mockClear()

    await userEvent.click(title)

    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('opens templates when the collapsed content group is clicked', async () => {
    await openGroup('Content')

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/console/workspace/$workspaceId/templates',
      params: { workspaceId: 'ws1' }
    })
  })

  it('falls back to the blog when the member cannot read templates', async () => {
    vi.mocked(isRootUser).mockReturnValue(false)
    vi.mocked(workspaceService.getMembers).mockResolvedValue({
      members: [
        {
          user_id: 'u1',
          permissions: { ...grantAll(true), templates: { read: false, write: false } }
        }
      ]
    } as never)

    await openGroup('Content')

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/console/workspace/$workspaceId/blog',
      params: { workspaceId: 'ws1' }
    })
  })
})

describe('WorkspaceLayout web analytics entries', () => {
  beforeEach(signInAsRoot)

  const selectedLabel = () => document.querySelector('.ant-menu-item-selected')?.textContent ?? null

  it('lists Annotations under the Web Analytics group', async () => {
    render(<WorkspaceLayout />)
    await userEvent.click(await screen.findByText('Web Analytics'))

    expect(screen.getByText('Annotations')).toBeInTheDocument()
  })

  it('highlights the annotations entry on the annotations route', async () => {
    // WEB_ANALYTICS_SECTIONS is the one registration TypeScript cannot catch:
    // a section missing from it falls through to the dashboard entry, so the
    // sidebar says you are somewhere you are not.
    currentPathname = '/console/workspace/ws1/web-analytics/annotations'

    render(<WorkspaceLayout />)
    await screen.findByText('Annotations')

    expect(selectedLabel()).toBe('Annotations')
  })

  it('still highlights the filters entry on the filters route', async () => {
    currentPathname = '/console/workspace/ws1/web-analytics/filters'

    render(<WorkspaceLayout />)
    await screen.findByText('Filters')

    expect(selectedLabel()).toBe('Filters')
  })
})
