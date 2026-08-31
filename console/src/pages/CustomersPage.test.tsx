import '../__tests__/resizeObserverMock'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nProvider } from '@lingui/react'
import { i18n } from '../i18n'
import { customerApi } from '../services/api/customer'
import { CustomersPage } from './CustomersPage'

vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual('@tanstack/react-router')
  return { ...actual, useParams: () => ({ workspaceId: 'ws1' }) }
})

vi.mock('../services/api/customer', () => ({
  customerQueryKeys: {
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
      <I18nProvider i18n={i18n}>{children}</I18nProvider>
    </QueryClientProvider>
  )
}

describe('CustomersPage', () => {
  beforeEach(() => vi.clearAllMocks())

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
})
