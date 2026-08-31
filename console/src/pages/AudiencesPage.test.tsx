import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'
import { AudiencesPage } from './AudiencesPage'
import { audienceApi } from '../services/api/marketing'
import { listsApi } from '../services/api/list'

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ workspaceId: 'workspace-1' }),
  Link: ({ children, to }: { children: ReactNode; to: string }) => <a href={to}>{children}</a>
}))
vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [{ id: 'workspace-1', settings: {} }] }),
  useWorkspacePermissions: () => ({ permissions: { segments: { read: true, write: true } } })
}))
vi.mock('../services/api/marketing', () => ({ audienceApi: { list: vi.fn(), delete: vi.fn() } }))
vi.mock('../services/api/list', () => ({ listsApi: { list: vi.fn() } }))
vi.mock('../components/audiences/AudienceDrawer', () => ({
  AudienceDrawer: () => <div>Audience drawer</div>
}))

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(audienceApi.list).mockResolvedValue({ items: [], total: 0 })
  vi.mocked(listsApi.list).mockResolvedValue({ lists: [] } as never)
})

it('shows an actionable empty state for dynamic audiences instead of list creation', async () => {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <AudiencesPage />
    </QueryClientProvider>
  )
  expect(await screen.findByText('No dynamic audiences yet')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Create dynamic audience' })).toBeEnabled()
  expect(screen.getByRole('link', { name: 'Manage lists' })).toHaveAttribute('href', '/console/workspace/$workspaceId/lists')
  expect(screen.queryByText('Create your first list')).not.toBeInTheDocument()
})
