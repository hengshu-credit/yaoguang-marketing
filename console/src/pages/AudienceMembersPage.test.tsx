import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { beforeEach, expect, it, vi } from 'vitest'
import { AudienceMembersPage } from './AudienceMembersPage'
import { audienceApi } from '../services/api/marketing'
import { listsApi } from '../services/api/list'

const routeState = vi.hoisted(() => ({ sourceType: 'list', sourceId: 'newsletter' }))

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ workspaceId: 'workspace-1', ...routeState }),
  Link: ({ children, to }: { children: ReactNode; to: string }) => <a href={to}>{children}</a>
}))
vi.mock('../contexts/AuthContext', () => ({
  useWorkspacePermissions: () => ({ permissions: {
    customers: { read: true, write: false },
    lists: { read: true, write: false },
    segments: { read: true, write: false }
  } })
}))
vi.mock('../services/api/marketing', () => ({
  audienceApi: { memberDetails: vi.fn(), get: vi.fn() }
}))
vi.mock('../services/api/list', () => ({ listsApi: { list: vi.fn() } }))
vi.mock('../components/customers/CustomerDrawer', () => ({ CustomerDrawer: () => null }))

function Wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      {children}
    </QueryClientProvider>
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  routeState.sourceType = 'list'
  routeState.sourceId = 'newsletter'
  vi.mocked(listsApi.list).mockResolvedValue({
    lists: [{
      id: 'newsletter', name: 'Newsletter', description: 'Product news',
      is_double_optin: false, is_public: true,
      created_at: '2026-08-01T00:00:00Z', updated_at: '2026-08-01T00:00:00Z'
    }, {
      id: 'transactional', name: 'Transactional', description: 'Service messages',
      is_double_optin: false, is_public: false,
      created_at: '2026-08-01T00:00:00Z', updated_at: '2026-08-01T00:00:00Z'
    }]
  } as never)
  vi.mocked(audienceApi.memberDetails).mockResolvedValue({
    items: [{
      customer: {
        customer_id: '22222222-2222-4222-8222-222222222222',
        customer_no: 'CUS-1001',
        external_user_id: 'external-1',
        version: 2,
        profile: {
          customer_id: '22222222-2222-4222-8222-222222222222',
          status: 'active', language: 'zh-CN', timezone: 'Asia/Shanghai',
          attributes: { tier: 'gold' }, version: 1,
          created_at: '2026-07-01T00:00:00Z', updated_at: '2026-08-20T00:00:00Z'
        },
        identities: [{
          id: 'identity-1', type: 'email', display_hint: 'a@example.com',
          verified: true, primary: true, enabled: true,
          created_at: '2026-07-01T00:00:00Z', updated_at: '2026-07-01T00:00:00Z'
        }],
        tags: ['vip'],
        created_at: '2026-07-01T00:00:00Z', updated_at: '2026-08-20T00:00:00Z'
      },
      subscriptions: [
        {
          list_id: 'newsletter', status: 'active',
          created_at: '2026-08-02T03:04:05Z', updated_at: '2026-08-20T00:00:00Z'
        },
        {
          list_id: 'transactional', status: 'pending',
          created_at: '2026-08-03T03:04:05Z', updated_at: '2026-08-20T00:00:00Z'
        }
      ],
      joined_at: '2026-08-02T03:04:05Z'
    }],
    next: ''
  })
})

it('shows list customers with live status, join time, event, and attribute filters', async () => {
  render(<AudienceMembersPage />, { wrapper: Wrapper })

  expect(await screen.findByText('CUS-1001')).toBeInTheDocument()
  expect(screen.getByText('Customers in Newsletter')).toBeInTheDocument()
  expect(screen.getByText('Active')).toBeInTheDocument()
  expect(screen.queryByText('Pending')).not.toBeInTheDocument()
  expect(screen.getByText('2026-08-02 03:04')).toBeInTheDocument()
  expect(screen.getByLabelText('Subscription status')).toBeInTheDocument()
  expect(screen.getByLabelText('Joined list between')).toBeInTheDocument()
  expect(screen.getByLabelText('Event name')).toBeInTheDocument()
  expect(screen.getByLabelText('Attribute key')).toBeInTheDocument()
  expect(screen.getByLabelText('Attribute value')).toBeInTheDocument()

  fireEvent.change(screen.getByLabelText('Event name'), { target: { value: 'purchase' } })
  fireEvent.change(screen.getByLabelText('Attribute key'), { target: { value: 'tier' } })
  fireEvent.change(screen.getByLabelText('Attribute value'), { target: { value: 'gold' } })
  fireEvent.click(screen.getByRole('button', { name: 'Apply filters' }))

  await waitFor(() => expect(audienceApi.memberDetails).toHaveBeenLastCalledWith(expect.objectContaining({
    workspace_id: 'workspace-1', list_id: 'newsletter', event_name: 'purchase',
    attribute_key: 'tier', attribute_value: 'gold'
  })))
})

it('does not offer a list join-time filter for a dynamic audience', async () => {
  routeState.sourceType = 'dynamic'
  routeState.sourceId = '11111111-1111-4111-8111-111111111111'
  vi.mocked(audienceApi.get).mockResolvedValue({
    id: routeState.sourceId, name: 'Recently active customers', kind: 'dynamic', active_version: 1
  })

  render(<AudienceMembersPage />, { wrapper: Wrapper })

  expect(await screen.findByText('Customers in Recently active customers')).toBeInTheDocument()
  expect(screen.getByText('Newsletter · Active')).toBeInTheDocument()
  expect(screen.getByText('Transactional · Pending')).toBeInTheDocument()
  expect(screen.queryByLabelText('Joined list between')).not.toBeInTheDocument()
  await waitFor(() => expect(audienceApi.memberDetails).toHaveBeenCalledWith(expect.objectContaining({
    workspace_id: 'workspace-1', audience_id: routeState.sourceId
  })))
})
