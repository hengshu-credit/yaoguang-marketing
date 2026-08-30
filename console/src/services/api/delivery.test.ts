import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from './client'
import { deliveryApi } from './delivery'

vi.mock('./client', () => ({ api: { get: vi.fn(), post: vi.fn() } }))

beforeEach(() => {
  vi.mocked(api.get).mockReset()
  vi.mocked(api.post).mockReset()
})

describe('deliveryApi', () => {
  it('keeps delivery queries workspace scoped', async () => {
    vi.mocked(api.get).mockResolvedValue({ deliveries: [], total: 0 })
    await deliveryApi.list('workspace-1', 'unknown', 25, 50)
    const url = vi.mocked(api.get).mock.calls[0][0]
    const params = new URLSearchParams(url.split('?')[1])
    expect(params.get('workspace_id')).toBe('workspace-1')
    expect(params.get('status')).toBe('unknown')
    expect(params.get('limit')).toBe('25')
    expect(params.get('offset')).toBe('50')
  })

  it('sends only an audited unknown resolution action', async () => {
    vi.mocked(api.post).mockResolvedValue({ status: 'resolved' })
    await deliveryApi.resolveUnknown(
      'workspace-1',
      'intent-1',
      'retry_after_verified_not_accepted',
      'provider portal verified rejection before acceptance'
    )
    expect(api.post).toHaveBeenCalledWith('/api/deliveries.resolveUnknown', {
      workspace_id: 'workspace-1',
      intent_id: 'intent-1',
      action: 'retry_after_verified_not_accepted',
      reason: 'provider portal verified rejection before acceptance'
    })
  })
})
