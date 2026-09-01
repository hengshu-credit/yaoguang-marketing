import '../__tests__/resizeObserverMock'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nProvider } from '@lingui/react'
import { App } from 'antd'
import { i18n } from '../i18n'
import { customerApi } from '../services/api/customer'
import { CustomersPage } from './CustomersPage'

vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual('@tanstack/react-router')
  return { ...actual, useParams: () => ({ workspaceId: 'ws1' }) }
})

vi.mock('../services/api/customer', () => ({
  customerQueryKeys: {
    all: (workspaceId: string) => ['customers', workspaceId],
    list: (workspaceId: string, request: unknown) => ['customers', workspaceId, 'list', request]
  },
  customerApi: {
    list: vi.fn(async () => ({
      customers: [
        {
          customer_id: 'customer-1',
          customer_no: 'U0001202608301000000811111111111141118111111111111111',
          external_user_id: 'crm-42',
          version: 1,
          identities: [{ id: 'identity-1', type: 'email', display_hint: 'a***@example.com' }],
          tags: ['vip'],
          created_at: '2026-08-30T00:00:00Z',
          updated_at: '2026-08-30T00:00:00Z'
        }
      ],
      next_cursor: 'next-page'
    })),
    updateListMemberships: vi.fn(async () => ({
      request_id: 'request-1', customers: 1, lists: 1, changed: 1, unchanged: 0
    }))
  }
}))

vi.mock('../services/api/list', () => ({
  listsApi: {
    list: vi.fn(async () => ({
      lists: [{ id: 'newsletter', name: 'Newsletter', is_double_optin: false, is_public: false, created_at: '', updated_at: '' }]
    }))
  }
}))

vi.mock('../components/customers/CustomerDrawer', () => ({
  CustomerDrawer: ({ open, customerId }: { open: boolean; customerId: string | null }) =>
    open ? <div>drawer:{customerId}</div> : null
}))

vi.mock('../contexts/AuthContext', () => ({
  useWorkspacePermissions: () => ({ permissions: { customers: { read: true, write: true } } })
}))

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
  return (
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}><App>{children}</App></I18nProvider>
    </QueryClientProvider>
  )
}

describe('CustomersPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1024 })
    window.dispatchEvent(new Event('resize'))
  })

  it('renders the primary page title at the shared 24px size', async () => {
    render(<CustomersPage />, { wrapper })

    const heading = await screen.findByRole('heading', { level: 1, name: 'Customers' })
    expect(heading.style.fontSize).toBe('24px')
    expect(heading.style.fontWeight).toBe('500')
  })

  it('searches Customer identifiers and opens the 360 drawer from a row', async () => {
    render(<CustomersPage />, { wrapper })

    expect(await screen.findByText('crm-42')).toBeInTheDocument()
    expect(screen.getByText('a***@example.com')).toBeInTheDocument()
    fireEvent.change(screen.getByPlaceholderText('Search customer number, external ID, email or phone'), {
      target: { value: 'alice@example.com' }
    })
    fireEvent.click(screen.getByRole('button', { name: 'Search' }))

    await waitFor(() => {
      expect(customerApi.list).toHaveBeenLastCalledWith(
        expect.objectContaining({ workspace_id: 'ws1', search: 'alice@example.com' })
      )
    })
    fireEvent.click(await screen.findByText(/U00012026083010000008/))
    expect(screen.getByText('drawer:customer-1')).toBeInTheDocument()
  })

  it('uses the server cursor for the next page', async () => {
    render(<CustomersPage />, { wrapper })
    await screen.findByText('crm-42')

    fireEvent.click(screen.getByRole('button', { name: 'Next page' }))

    await waitFor(() => {
      expect(customerApi.list).toHaveBeenLastCalledWith(
        expect.objectContaining({ workspace_id: 'ws1', cursor: 'next-page' })
      )
    })
  })

  it('selects customers and opens the bulk list membership adjustment', async () => {
    render(<CustomersPage />, { wrapper })
    await screen.findByText('crm-42')

    fireEvent.click(screen.getByRole('checkbox', { name: /Select customer U0001/ }))

    expect(screen.getByText('1 customer selected')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Adjust list memberships' }))
    expect(await screen.findByRole('dialog', { name: 'Adjust list memberships' })).toBeInTheDocument()
    expect(screen.getByText('This operation applies to 1 selected customers.')).toBeInTheDocument()
  })

  it('offers the same customer selection action on narrow screens', async () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 375 })
    render(<CustomersPage />, { wrapper })

    const checkbox = await screen.findByRole('checkbox', { name: /Select customer U0001/ })
    fireEvent.click(checkbox)

    expect(screen.getByText('1 customer selected')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Adjust list memberships' })).toBeInTheDocument()
  })
})
