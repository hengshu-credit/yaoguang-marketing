import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from './client'
import { customEventApi } from './custom_event'

vi.mock('./client', () => ({ api: { get: vi.fn(), post: vi.fn() } }))

describe('customEventApi', () => {
  beforeEach(() => vi.mocked(api.post).mockReset())

  it('uses the backend upsert route for single custom events', async () => {
    vi.mocked(api.post).mockResolvedValue({ event: { external_id: 'event-1' } })

    await customEventApi.create({
      workspace_id: 'ws1',
      email: 'alice@example.com',
      event_name: 'purchase',
      external_id: 'event-1'
    })

    expect(api.post).toHaveBeenCalledWith('/api/customEvents.upsert', expect.objectContaining({
      workspace_id: 'ws1',
      external_id: 'event-1'
    }))
  })
})
