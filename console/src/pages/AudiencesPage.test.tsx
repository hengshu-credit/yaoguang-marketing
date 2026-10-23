import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'
import { AudiencesPage } from './AudiencesPage'
import { audienceApi } from '../services/api/marketing'
import { listsApi } from '../services/api/list'

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ workspaceId: 'workspace-1' }),
  Link: ({ children, to, params }: { children: ReactNode; to: string; params?: Record<string, string> }) => {
    const href = to
      .replace('$sourceType', params?.sourceType ?? '$sourceType')
      .replace('$sourceId', params?.sourceId ?? '$sourceId')
    return <a href={href}>{children}</a>
  }
}))
vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [{ id: 'workspace-1', settings: {} }] }),
  useWorkspacePermissions: () => ({
    permissions: {
      segments: { read: true, write: true },
      lists: { read: true, write: true }
    }
  })
}))
vi.mock('../services/api/marketing', () => ({ audienceApi: { list: vi.fn(), delete: vi.fn() } }))
vi.mock('../services/api/list', () => ({ listsApi: { list: vi.fn(), delete: vi.fn() } }))
vi.mock('../components/lists/ListStats', () => ({
  ListStats: ({ listId }: { listId: string }) => (
    <div data-testid={`list-stats-${listId}`}>Active 12 · Pending 3 · Unsubscribed 4 · Bounced 2 · Complained 1</div>
  )
}))
vi.mock('../components/audiences/AudienceDrawer', () => ({
  AudienceDrawer: ({ audienceId }: { audienceId?: string }) => (
    <div>{audienceId ? `Editing audience ${audienceId}` : 'Audience drawer'}</div>
  )
}))
vi.mock('../components/lists/ListDrawer', () => ({
  CreateListDrawer: ({ list, buttonProps }: { list?: { name: string }; buttonProps?: { buttonContent?: ReactNode; disabled?: boolean } }) => (
    <button disabled={buttonProps?.disabled}>{buttonProps?.buttonContent ?? (list ? 'Edit list' : 'Create list')}</button>
  )
}))

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(audienceApi.list).mockResolvedValue({
    items: [
      {
        id: 'audience-1',
        name: 'Recently active customers',
        description: 'Customers with recent activity',
        kind: 'dynamic',
        active_version: 2,
        created_at: '2026-08-02T08:30:00Z',
        updated_at: '2026-08-03T09:45:00Z'
      }
    ],
    total: 1
  })
  vi.mocked(listsApi.list).mockResolvedValue({
    lists: [
      {
        id: 'newsletter',
        name: 'Newsletter',
        description: 'Product news subscribers',
        is_double_optin: false,
        is_public: true,
        created_at: '2026-08-01T00:00:00Z',
        updated_at: '2026-08-01T00:00:00Z'
      }
    ]
  } as never)
  vi.mocked(listsApi.delete).mockResolvedValue({ status: 'success' })
  vi.mocked(audienceApi.delete).mockResolvedValue({ deleted: true })
})

it('shows lists and dynamic audiences as linked rows in one audience table', async () => {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <AudiencesPage />
    </QueryClientProvider>
  )

  expect(await screen.findByRole('link', { name: 'Newsletter' })).toHaveAttribute(
    'href',
    '/console/workspace/$workspaceId/lists/list/newsletter'
  )
  expect(screen.getByRole('link', { name: 'Recently active customers' })).toHaveAttribute(
    'href',
    '/console/workspace/$workspaceId/lists/dynamic/audience-1'
  )
  expect(screen.getByText('List')).toBeInTheDocument()
  expect(screen.getByText('Dynamic')).toBeInTheDocument()
  expect(screen.queryByRole('columnheader', { name: 'Version' })).not.toBeInTheDocument()
  expect(screen.getByRole('columnheader', { name: 'Created At' })).toBeInTheDocument()
  expect(screen.getByRole('columnheader', { name: 'Updated At' })).toBeInTheDocument()
  expect(screen.getAllByText('2026-08-01 00:00')).toHaveLength(2)
  expect(screen.getByText('2026-08-03 09:45')).toBeInTheDocument()
  expect(screen.getByTestId('list-stats-newsletter')).toHaveTextContent('Active 12')
  expect(screen.getByTestId('list-stats-newsletter')).toHaveTextContent('Complained 1')
  expect(screen.getByRole('button', { name: 'Create list' })).toBeEnabled()
  expect(screen.getByRole('button', { name: 'Create dynamic audience' })).toBeEnabled()
  fireEvent.click(screen.getByRole('button', { name: 'Edit Recently active customers' }))
  expect(screen.getByText('Editing audience audience-1')).toBeInTheDocument()
  expect(screen.queryByRole('link', { name: 'Manage lists' })).not.toBeInTheDocument()
})

it('lets the workspace layout own the page-edge padding', async () => {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <AudiencesPage />
    </QueryClientProvider>
  )

  const heading = await screen.findByRole('heading', {
    level: 1,
    name: 'Audience segmentation'
  })
  expect(heading.closest('.p-6')).toBeNull()
  expect(heading.parentElement?.parentElement).toHaveClass('flex-col', 'sm:flex-row')
})

it('keeps list editing and guarded deletion on the unified audience page', async () => {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <AudiencesPage />
    </QueryClientProvider>
  )

  expect(await screen.findByRole('button', { name: 'Edit list' })).toBeEnabled()
  fireEvent.click(screen.getByRole('button', { name: 'Delete Newsletter' }))
  expect(screen.getByRole('button', { name: 'Delete' })).toBeDisabled()
  fireEvent.change(screen.getByPlaceholderText('Enter ID to confirm'), {
    target: { value: 'newsletter' }
  })
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }))

  await waitFor(() => expect(listsApi.delete).toHaveBeenCalledWith({
    workspace_id: 'workspace-1',
    id: 'newsletter'
  }))
})

it('lets users delete a dynamic audience after confirming its ID', async () => {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <AudiencesPage />
    </QueryClientProvider>
  )

  fireEvent.click(await screen.findByRole('button', { name: 'Delete Recently active customers' }))
  expect(screen.getByText('Delete audience')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Delete' })).toBeDisabled()
  fireEvent.change(screen.getByPlaceholderText('Enter ID to confirm'), {
    target: { value: 'audience-1' }
  })
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }))

  await waitFor(() => expect(audienceApi.delete).toHaveBeenCalledWith('workspace-1', 'audience-1'))
  expect(listsApi.delete).not.toHaveBeenCalled()
})
