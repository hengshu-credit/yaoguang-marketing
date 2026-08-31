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
    window.history.replaceState({}, '', '/console/workspace/ws1/deliveries')
    vi.mocked(deliveryApi.list).mockResolvedValue({ deliveries: [unknownDelivery], total: 1 })
    vi.mocked(deliveryApi.get).mockResolvedValue({
      intent: unknownDelivery,
      attempts: [{ id: 'attempt-1', provider: 'aliyun', status: 'unknown', error_detail: 'provider timeout' }],
      reconciliations: []
    })
    vi.mocked(deliveryApi.resolveUnknown).mockResolvedValue({ status: 'resolved' })
  })

  it('restores compact popover filters and persists applied business filters in the URL', async () => {
    render(<DeliveryCenterPage />, { wrapper })

    expect(await screen.findByRole('button', { name: 'Review' })).toBeInTheDocument()
    expect(screen.getAllByText('Needs confirmation').length).toBeGreaterThan(0)
    expect(deliveryApi.list).toHaveBeenCalledWith(expect.objectContaining({ workspace_id: 'ws1', status: 'unknown' }))
    expect(screen.queryByPlaceholderText('Customer ID or number')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Customer' }))
    fireEvent.change(await screen.findByPlaceholderText('Customer ID or number'), { target: { value: ' customer-1 ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply' }))

    await waitFor(() => {
      expect(deliveryApi.list).toHaveBeenLastCalledWith(
        expect.objectContaining({ workspace_id: 'ws1', customer_id: 'customer-1' })
      )
    })
    expect(screen.getByRole('button', { name: 'Customer: customer-1' })).toBeInTheDocument()
    expect(new URLSearchParams(window.location.search).get('customer_id')).toBe('customer-1')

    fireEvent.click(screen.getByRole('button', { name: 'Customer: customer-1' }))
    fireEvent.click(screen.getByRole('button', { name: 'Clear' }))
    await waitFor(() => {
      expect(deliveryApi.list).toHaveBeenLastCalledWith(
        expect.not.objectContaining({ customer_id: expect.anything() })
      )
    })
    expect(new URLSearchParams(window.location.search).has('customer_id')).toBe(false)
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

  it('keeps an invalid delivery time range out of the API request', async () => {
    render(<DeliveryCenterPage />, { wrapper })
    await screen.findByRole('button', { name: 'Review' })

    fireEvent.click(screen.getByRole('button', { name: 'Delivery time' }))
    fireEvent.change(await screen.findByLabelText('From'), { target: { value: '2026-08-30T12:00' } })
    fireEvent.change(screen.getByLabelText('To'), { target: { value: '2026-08-30T11:00' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply' }))

    expect(await screen.findByText('End time must be after start time')).toBeInTheDocument()
    expect(deliveryApi.list).toHaveBeenLastCalledWith(
      expect.not.objectContaining({ from: expect.anything(), to: expect.anything() })
    )
  })

  it('ignores a malformed delivery time restored from the URL', async () => {
    window.history.replaceState({}, '', '/console/workspace/ws1/deliveries?from=not-a-date')

    render(<DeliveryCenterPage />, { wrapper })

    expect(await screen.findByRole('button', { name: 'Review' })).toBeInTheDocument()
    expect(deliveryApi.list).toHaveBeenLastCalledWith(
      expect.not.objectContaining({ from: expect.anything() })
    )
  })
})
