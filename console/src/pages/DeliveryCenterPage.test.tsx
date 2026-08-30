import '../__tests__/resizeObserverMock'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nProvider } from '@lingui/react'
import { i18n } from '../i18n'
import { deliveryApi } from '../services/api/delivery'
import { DeliveryCenterPage } from './DeliveryCenterPage'

vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual('@tanstack/react-router')
  return { ...actual, useParams: () => ({ workspaceId: 'ws1' }) }
})

vi.mock('../contexts/AuthContext', () => ({
  useWorkspacePermissions: () => ({ permissions: { message_history: { read: true, write: true } } })
}))

vi.mock('../components/customers/CustomerDrawer', () => ({
  CustomerDrawer: ({ open, customerId }: { open: boolean; customerId: string | null }) =>
    open ? <div>customer:{customerId}</div> : null
}))

vi.mock('../services/api/delivery', () => ({
  deliveryApi: {
    list: vi.fn(),
    get: vi.fn(),
    reconcile: vi.fn(),
    resolveUnknown: vi.fn()
  }
}))

const unknownDelivery = {
  id: 'delivery-1',
  effect_key: 'effect-1',
  source_type: 'automation',
  source_id: 'automation-1',
  source_version: '1',
  customer_id: 'customer-1',
  channel: 'sms',
  status: 'unknown' as const,
  created_at: '2026-08-30T08:00:00Z',
  updated_at: '2026-08-30T08:00:00Z'
}

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return (
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>{children}</I18nProvider>
    </QueryClientProvider>
  )
}

describe('DeliveryCenterPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(deliveryApi.list).mockResolvedValue({ deliveries: [unknownDelivery], total: 1 })
    vi.mocked(deliveryApi.get).mockResolvedValue({
      intent: unknownDelivery,
      attempts: [{ id: 'attempt-1', provider: 'aliyun', status: 'unknown', error_detail: 'provider timeout' }],
      reconciliations: []
    })
    vi.mocked(deliveryApi.resolveUnknown).mockResolvedValue({ status: 'resolved' })
  })

  it('focuses unresolved deliveries and passes business filters to the API', async () => {
    render(<DeliveryCenterPage />, { wrapper })

    expect(await screen.findByRole('button', { name: 'Review' })).toBeInTheDocument()
    expect(screen.getAllByText('Needs confirmation').length).toBeGreaterThan(0)
    expect(deliveryApi.list).toHaveBeenCalledWith(expect.objectContaining({ workspace_id: 'ws1', status: 'unknown' }))

    fireEvent.change(screen.getByPlaceholderText('Customer ID or number'), { target: { value: 'customer-1' } })
    fireEvent.change(screen.getByPlaceholderText('Provider'), { target: { value: 'aliyun' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply filters' }))
    await waitFor(() => {
      expect(deliveryApi.list).toHaveBeenLastCalledWith(
        expect.objectContaining({ workspace_id: 'ws1', customer_id: 'customer-1', provider: 'aliyun' })
      )
    })
  })

  it('requires a reason before an unknown delivery can be resolved', async () => {
    render(<DeliveryCenterPage />, { wrapper })
    fireEvent.click(await screen.findByRole('button', { name: 'Review' }))
    expect(await screen.findByText('provider timeout')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Resolve status' }))
    fireEvent.click(screen.getByRole('button', { name: 'Confirm resolution' }))
    expect(await screen.findByText('Please enter at least 8 characters')).toBeInTheDocument()
    expect(deliveryApi.resolveUnknown).not.toHaveBeenCalled()

    fireEvent.change(screen.getByPlaceholderText('Describe the verification evidence and reason'), {
      target: { value: 'Provider support confirmed the message was not accepted.' }
    })
    fireEvent.click(screen.getByRole('button', { name: 'Confirm resolution' }))
    await waitFor(() => expect(deliveryApi.resolveUnknown).toHaveBeenCalled())
  })
})
