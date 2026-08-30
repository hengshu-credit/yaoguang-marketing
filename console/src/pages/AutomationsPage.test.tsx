import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'

// jsdom lacks these browser APIs that antd touches.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)
vi.stubGlobal('matchMedia', () => ({
  matches: false,
  addListener() {},
  removeListener() {},
  addEventListener() {},
  removeEventListener() {}
}))

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ workspaceId: 'ws1' })
}))

vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [{ id: 'ws1', name: 'Workspace' }] }),
  useWorkspacePermissions: () => ({ permissions: { automations: { write: true } } })
}))

// Keep the page render shallow — these children are exercised by their own tests.
vi.mock('../components/automations/AutomationCard', () => ({
  AutomationCard: ({ automation, onActivate }: { automation: { id: string }; onActivate: (automation: { id: string }) => void }) => (
    <button type="button" onClick={() => onActivate(automation)}>
      Check and activate {automation.id}
    </button>
  )
}))
vi.mock('../components/automations/UpsertAutomationDrawer', () => ({
  UpsertAutomationDrawer: ({ open, initialNodeId }: { open?: boolean; initialNodeId?: string }) =>
    open ? <section aria-label="Automation editor">{initialNodeId}</section> : null
}))
vi.mock('../components/automations/JourneyPreflightPanel', () => ({
  JourneyPreflightPanel: ({
    automationId,
    onFixIssue
  }: {
    automationId: string
    onFixIssue: (issue: { node_id: string }) => void
  }) => (
    <section aria-label="Journey activation preflight">
      {automationId}
      <button type="button" onClick={() => onFixIssue({ node_id: 'email-1' })}>
        Fix message node
      </button>
    </section>
  )
}))

const templatesList = vi.fn().mockResolvedValue({ templates: [] })
const automationsList = vi.fn().mockResolvedValue({ automations: [], total: 0 })
vi.mock('../services/api/template', () => ({
  templatesApi: { list: (...args: unknown[]) => templatesList(...args) }
}))
vi.mock('../services/api/automation', () => ({
  automationApi: { list: (...args: unknown[]) => automationsList(...args) },
  Automation: class {}
}))
vi.mock('../services/api/list', () => ({
  listsApi: { list: vi.fn().mockResolvedValue({ lists: [] }) }
}))
vi.mock('../services/api/segment', () => ({
  listSegments: vi.fn().mockResolvedValue({ segments: [] })
}))

import { AutomationsPage } from './AutomationsPage'

const renderPage = () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider i18n={i18n}>
        <App>
          <AutomationsPage />
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )
}

describe('AutomationsPage template reference query', () => {
  beforeEach(() => {
    templatesList.mockClear()
    automationsList.mockReset().mockResolvedValue({ automations: [], total: 0 })
  })

  it('fetches all email templates (no category filter) so email nodes resolve any selected template', async () => {
    // The automation email-node picker (EmailConfigForm -> TemplateSelectorInput)
    // is category-agnostic, so the canvas reference list must be too. Restricting
    // it to category:'marketing' made validly-selected non-marketing templates
    // (e.g. a welcome email) render as "Template set" instead of their name. This
    // asserts the reference query is not category-restricted — it fails on the
    // pre-fix code that passed category:'marketing'.
    renderPage()

    await waitFor(() => expect(templatesList).toHaveBeenCalled())
    const params = templatesList.mock.calls[0][0] as {
      workspace_id: string
      channel?: string
      category?: string
    }
    expect(params.workspace_id).toBe('ws1')
    expect(params.channel).toBe('email')
    expect(params.category).toBeUndefined()
  })

  it('opens activation preflight instead of activating a draft directly', async () => {
    const user = userEvent.setup()
    automationsList.mockResolvedValue({
      automations: [
        {
          id: 'automation-1',
          workspace_id: 'ws1',
          name: 'Welcome journey',
          status: 'draft',
          list_id: '',
          root_node_id: 'trigger-1',
          nodes: [],
          created_at: '2026-08-30T00:00:00Z',
          updated_at: '2026-08-30T00:00:00Z'
        }
      ],
      total: 1
    })

    renderPage()

    await user.click(await screen.findByRole('button', { name: 'Check and activate automation-1' }))

    expect(await screen.findByRole('region', { name: 'Journey activation preflight' })).toHaveTextContent(
      'automation-1'
    )

    await user.click(screen.getByRole('button', { name: 'Fix message node' }))
    expect(await screen.findByRole('region', { name: 'Automation editor' })).toHaveTextContent(
      'email-1'
    )
  })
})
