import '../../__tests__/resizeObserverMock'
import { App } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ContactCustomerProvider, ContactCustomerAttributes, ContactCustomerActivity } from './ContactCustomerDetails'
import { customerApi } from '../../services/api/customer'
import { deliveryApi } from '../../services/api/delivery'
import { ApiError } from '../../services/api/errors'

const state = vi.hoisted(() => ({ read: true, write: true, deliveries: true }))
vi.mock('../../contexts/AuthContext', () => ({
  useWorkspacePermissions: () => ({ loading: false, permissions: {
    customers: { read: state.read, write: state.write },
    automations: { read: false },
    broadcasts: { read: false },
    message_history: { read: state.deliveries }
  } })
}))
vi.mock('../../services/api/customer', async (original) => ({
  ...await original<typeof import('../../services/api/customer')>(),
  customerApi: { getByEmail: vi.fn(), update: vi.fn() }
}))
vi.mock('../../services/api/delivery', () => ({ deliveryApi: { list: vi.fn() } }))
vi.mock('../../services/api/broadcast', () => ({ broadcastApi: { get: vi.fn() } }))
vi.mock('../automations/JourneyTraceDrawer', () => ({ JourneyTraceDrawer: () => null }))
vi.mock('../customers/CustomerJourneysTab', () => ({ CustomerJourneysTab: () => null }))
// Isolate the API modules from the app's router during component tests.
vi.mock('../../services/api/client', () => ({ api: { post: vi.fn(), get: vi.fn() } }))

function View({ email = 'alice@example.com', active = false, onUpdated }: { email?: string; active?: boolean; onUpdated?: () => void }) {
  return <ContactCustomerProvider key={email} workspaceId="ws1" email={email} onUpdated={onUpdated}>
    <ContactCustomerAttributes />
    <ContactCustomerActivity sourceType="broadcast" active={active} />
  </ContactCustomerProvider>
}

function wrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <App>{children}</App>
  </QueryClientProvider>
}

beforeEach(() => {
  vi.clearAllMocks()
  state.read = true
  state.write = true
  state.deliveries = true
  vi.mocked(customerApi.getByEmail).mockImplementation(async (_ws, email) => ({
    customer_id: email, customer_no: 'C01', version: 1,
    identities: [], tags: ['VIP'], created_at: '', updated_at: '',
    profile: { customer_id: email, status: 'active', attributes: { tier: email === 'alice@example.com' ? 'gold' : 'silver' }, version: 1, created_at: '', updated_at: '' }
  }))
  vi.mocked(customerApi.update).mockResolvedValue({ customer_id: 'alice@example.com', action: 'updated' })
  vi.mocked(deliveryApi.list).mockResolvedValue({ deliveries: [], total: 0 })
})

describe('contact customer extensions', () => {
  it('edits typed attributes on the exactly resolved customer and loads deliveries only on demand', async () => {
    const onUpdated = vi.fn()
    const view = render(<View onUpdated={onUpdated} />, { wrapper })
    await screen.findByText('gold')
    expect(customerApi.getByEmail).toHaveBeenCalledWith('ws1', 'alice@example.com')
    expect(deliveryApi.list).not.toHaveBeenCalled()
    expect(screen.queryByText('Customer 360')).not.toBeInTheDocument()
    expect(screen.queryByText('Identity aliases')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Add profile attribute' }))
    fireEvent.change(screen.getByPlaceholderText('Attribute name'), { target: { value: 'score' } })
    fireEvent.change(screen.getByPlaceholderText('Value (text or JSON)'), { target: { value: '88' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add' }))
    await waitFor(() => expect(customerApi.update).toHaveBeenCalledWith('ws1', 'alice@example.com', {
      profile: { attributes: { merge: { score: 88 } } }
    }, expect.any(String)))
    await waitFor(() => expect(onUpdated).toHaveBeenCalledTimes(1))
    view.rerender(<View active />)
    await waitFor(() => expect(deliveryApi.list).toHaveBeenCalledWith(expect.objectContaining({
      workspace_id: 'ws1', customer_id: 'alice@example.com', source_type: 'broadcast'
    })))
  })

  it('does not keep another contact profile after switching contacts', async () => {
    const view = render(<View />, { wrapper })
    await screen.findByText('gold')
    view.rerender(<View email="bob@example.com" />)
    await screen.findByText('silver')
    expect(screen.queryByText('gold')).not.toBeInTheDocument()
  })

  it('keeps a missing customer explicit and never requests unfiltered deliveries', async () => {
    vi.mocked(customerApi.getByEmail).mockRejectedValue(new ApiError('not found', 404))
    render(<View active />, { wrapper })
    expect((await screen.findAllByText('No customer is linked to this contact')).length).toBeGreaterThan(0)
    expect(deliveryApi.list).not.toHaveBeenCalled()
  })

  it('honors read-only profile and delivery permissions', async () => {
    state.write = false
    state.deliveries = false
    render(<View active />, { wrapper })
    await screen.findByText('gold')
    expect(screen.queryByRole('button', { name: /^(Edit|Remove|Add profile attribute)/ })).not.toBeInTheDocument()
    expect(deliveryApi.list).not.toHaveBeenCalled()
  })

  it('does not load customer data without read permission', async () => {
    state.read = false
    render(<View active />, { wrapper })
    expect((await screen.findAllByText('Customer read permission is required')).length).toBeGreaterThan(0)
    expect(customerApi.getByEmail).not.toHaveBeenCalled()
    expect(deliveryApi.list).not.toHaveBeenCalled()
  })
})
