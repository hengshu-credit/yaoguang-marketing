import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { I18nProvider } from '@lingui/react'
import { i18n } from '../../i18n'
import { useWorkspacePermissions } from '../../contexts/AuthContext'
import { ContentCenterTabs, DataAnalyticsTabs } from './WorkspaceSectionTabs'

vi.mock('../../contexts/AuthContext', () => ({
  useWorkspacePermissions: vi.fn()
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a data-to={to}>{children}</a>
  )
}))

const permissionSet = (overrides: Record<string, { read: boolean; write: boolean }> = {}) => ({
  customers: { read: true, write: true },
  contacts: { read: true, write: true },
  lists: { read: true, write: true },
  templates: { read: true, write: true },
  broadcasts: { read: true, write: true },
  transactional: { read: true, write: true },
  workspace: { read: true, write: true },
  message_history: { read: true, write: true },
  blog: { read: true, write: true },
  automations: { read: true, write: true },
  llm: { read: true, write: true },
  web_analytics: { read: true, write: true },
  ...overrides
})

const renderWithI18n = (node: ReactNode) => {
  i18n.activate('en')
  return render(<I18nProvider i18n={i18n}>{node}</I18nProvider>)
}

const routeForTab = (navigation: HTMLElement, name: string) =>
  within(navigation).getByRole('tab', { name }).querySelector('a')

describe('ContentCenterTabs', () => {
  beforeEach(() => {
    vi.mocked(useWorkspacePermissions).mockReturnValue({
      permissions: permissionSet(),
      loading: false
    } as never)
  })

  it('keeps all three retained content pages reachable and selects the current route', () => {
    renderWithI18n(<ContentCenterTabs workspaceId="ws1" activeKey="blog" />)

    const navigation = screen.getByRole('navigation', { name: 'Content Center' })
    expect(routeForTab(navigation, 'Template Management')).toHaveAttribute(
      'data-to',
      '/console/workspace/$workspaceId/templates'
    )
    expect(routeForTab(navigation, 'Blog Content')).toHaveAttribute(
      'data-to',
      '/console/workspace/$workspaceId/blog'
    )
    expect(routeForTab(navigation, 'File Manager')).toHaveAttribute(
      'data-to',
      '/console/workspace/$workspaceId/file-manager'
    )
    expect(within(navigation).getByRole('tab', { name: 'Blog Content' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  })

  it('does not expose template management without template read access', () => {
    vi.mocked(useWorkspacePermissions).mockReturnValue({
      permissions: permissionSet({ templates: { read: false, write: false } }),
      loading: false
    } as never)

    renderWithI18n(<ContentCenterTabs workspaceId="ws1" activeKey="blog" />)

    expect(screen.queryByRole('tab', { name: 'Template Management' })).not.toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Blog Content' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'File Manager' })).toBeInTheDocument()
  })
})

describe('DataAnalyticsTabs', () => {
  it('restores all marketing and website analytics pages with route-backed tabs', () => {
    vi.mocked(useWorkspacePermissions).mockReturnValue({
      permissions: permissionSet(),
      loading: false
    } as never)

    renderWithI18n(<DataAnalyticsTabs workspaceId="ws1" activeKey="explore" />)

    const navigation = screen.getByRole('navigation', { name: 'Data Analytics' })
    const expected = [
      ['Marketing Overview', '/console/workspace/$workspaceId/analytics'],
      ['Website Overview', '/console/workspace/$workspaceId/web-analytics/$tab'],
      ['Live Visitors', '/console/workspace/$workspaceId/web-analytics/live'],
      ['Multidimensional Analysis', '/console/workspace/$workspaceId/web-analytics/$tab'],
      ['Conversion Goals', '/console/workspace/$workspaceId/web-analytics/$tab'],
      ['Attribution Rules', '/console/workspace/$workspaceId/web-analytics/$tab'],
      ['Analytics Annotations', '/console/workspace/$workspaceId/web-analytics/$tab']
    ]

    for (const [label, route] of expected) {
      expect(routeForTab(navigation, label)).toHaveAttribute('data-to', route)
    }
    expect(within(navigation).getByRole('tab', { name: 'Multidimensional Analysis' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  })

  it('shows only marketing overview when the member lacks web analytics access', () => {
    vi.mocked(useWorkspacePermissions).mockReturnValue({
      permissions: permissionSet({ web_analytics: { read: false, write: false } }),
      loading: false
    } as never)

    renderWithI18n(<DataAnalyticsTabs workspaceId="ws1" activeKey="marketing" />)

    expect(screen.getByRole('tab', { name: 'Marketing Overview' })).toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: 'Website Overview' })).not.toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: 'Live Visitors' })).not.toBeInTheDocument()
  })

  it('shows website analytics without marketing overview when message history is denied', () => {
    vi.mocked(useWorkspacePermissions).mockReturnValue({
      permissions: permissionSet({ message_history: { read: false, write: false } }),
      loading: false
    } as never)

    renderWithI18n(<DataAnalyticsTabs workspaceId="ws1" activeKey="dashboard" />)

    expect(screen.queryByRole('tab', { name: 'Marketing Overview' })).not.toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Website Overview' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Analytics Annotations' })).toBeInTheDocument()
  })
})
