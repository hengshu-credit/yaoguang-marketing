import '../../__tests__/resizeObserverMock'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ContactDeliveriesTab } from './ContactDeliveriesTab'
import { deliveryApi } from '../../services/api/delivery'
import { broadcastApi } from '../../services/api/broadcast'

vi.mock('../../services/api/delivery', () => ({ deliveryApi: { list: vi.fn() } }))
vi.mock('../../services/api/broadcast', () => ({ broadcastApi: { get: vi.fn() } }))

function wrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>{children}</QueryClientProvider>
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(deliveryApi.list).mockImplementation(async (request) => ({
    deliveries: [{ id: String(typeof request === 'string' ? 0 : request.offset), effect_key: '', source_type: 'broadcast', source_id: 'campaign-1', source_version: '1', channel: 'sms', status: 'confirmed', created_at: '2026-09-01T00:00:00Z', updated_at: '' }],
    total: 21
  }))
  vi.mocked(broadcastApi.get).mockResolvedValue({ broadcast: { name: 'September offer' } } as Awaited<ReturnType<typeof broadcastApi.get>>)
})

describe('customer-scoped deliveries', () => {
  it('requests campaigns separately and paginates without dropping the customer filter', async () => {
    render(<ContactDeliveriesTab workspaceId="ws1" customerId="customer-1" sourceType="broadcast" active canReadCampaigns />, { wrapper })
    await screen.findByText('September offer')
    fireEvent.click(screen.getByTitle('2'))
    await waitFor(() => expect(deliveryApi.list).toHaveBeenLastCalledWith({
      workspace_id: 'ws1', customer_id: 'customer-1', source_type: 'broadcast', limit: 20, offset: 20
    }))
  })

  it('requests only automation deliveries and never campaign metadata', async () => {
    render(<ContactDeliveriesTab workspaceId="ws1" customerId="customer-2" sourceType="automation" active canReadCampaigns={false} />, { wrapper })
    await waitFor(() => expect(deliveryApi.list).toHaveBeenCalledWith(expect.objectContaining({
      customer_id: 'customer-2', source_type: 'automation'
    })))
    expect(broadcastApi.get).not.toHaveBeenCalled()
  })
})
