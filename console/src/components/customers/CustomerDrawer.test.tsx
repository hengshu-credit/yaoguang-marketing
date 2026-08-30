import '../../__tests__/resizeObserverMock'
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nProvider } from '@lingui/react'
import { i18n } from '../../i18n'
import { CustomerDrawer } from './CustomerDrawer'

vi.mock('../../services/api/customer', async () => {
  const actual = await vi.importActual('../../services/api/customer')
  return {
    ...actual,
    customerApi: {
      get: vi.fn(async () => ({
        customer_id: 'customer-1',
        customer_no: 'U0001202608301000000811111111111141118111111111111111',
        external_user_id: 'crm-42',
        version: 3,
        profile: { customer_id: 'customer-1', status: 'active', attributes: { tier: 'gold' } },
        identities: [
          {
            id: 'identity-1',
            type: 'email',
            display_hint: 'a***@example.com',
            value_ciphertext: 'must-never-render',
            verified: true,
            primary: true,
            enabled: true
          }
        ],
        tags: ['vip'],
        list_memberships: [{ list_id: 'newsletter', status: 'active' }],
        consents: [
          {
            id: 'consent-1',
            purpose: 'marketing',
            channel: 'email',
            status: 'granted',
            valid_from: '2026-08-30T00:00:00Z'
          }
        ],
        created_at: '2026-08-30T00:00:00Z',
        updated_at: '2026-08-30T00:00:00Z'
      }))
    }
  }
})

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>{children}</I18nProvider>
    </QueryClientProvider>
  )
}

describe('CustomerDrawer', () => {
  it('renders the Customer 360 foundation with masked identities and consent', async () => {
    render(
      <CustomerDrawer
        workspaceId="ws1"
        customerId="customer-1"
        open
        onClose={vi.fn()}
      />,
      { wrapper }
    )

    expect(await screen.findByText('crm-42')).toBeInTheDocument()
    expect(screen.getByText(/a\*\*\*@example\.com/)).toBeInTheDocument()
    expect(screen.getByText('vip')).toBeInTheDocument()
    expect(screen.getByText(/newsletter/)).toBeInTheDocument()
    expect(screen.getByText(/marketing \/ email/)).toBeInTheDocument()
    expect(screen.queryByText('must-never-render')).not.toBeInTheDocument()
  })
})
