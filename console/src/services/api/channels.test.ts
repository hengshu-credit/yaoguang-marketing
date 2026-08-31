import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from './client'
import { channelsApi } from './channels'

vi.mock('./client', () => ({ api: { get: vi.fn() } }))

describe('channelsApi.list', () => {
  beforeEach(() => vi.mocked(api.get).mockReset())

  it('requests the authenticated workspace catalogue and preserves server order', async () => {
    vi.mocked(api.get).mockResolvedValue({
      channels: [
        { id: 'line', label_key: 'LINE', content_families: ['text'], preview_profiles: [], delivery_modes: ['signed_webhook'], limits: {} },
        { id: 'zalo', label_key: 'Zalo', content_families: ['text'], preview_profiles: [], delivery_modes: ['signed_webhook'], limits: {} }
      ]
    })

    const response = await channelsApi.list('workspace 1')

    expect(api.get).toHaveBeenCalledWith('/api/channels.catalog?workspace_id=workspace%201')
    expect(response.channels.map((channel) => channel.id)).toEqual(['line', 'zalo'])
  })
})

