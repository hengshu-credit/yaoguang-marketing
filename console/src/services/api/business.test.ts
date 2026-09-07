import { describe, it, expect, vi } from 'vitest'
import { api } from './client'
import { businessApi } from './business'

vi.mock('./client', () => ({ api: { get: vi.fn(), post: vi.fn() } }))

describe('business definitions API', () => {
  it('sends the actual dynamic object definition and expected revision to the workspace', async () => {
    const definition = { kind: 'object_type' as const, key: 'contract', name: 'Contract', schema: { type: 'object' }, settings: { identity_fields: ['contract_no'] }, status: 'active' as const, revision: 4 }
    vi.mocked(api.post).mockResolvedValue({ definition: { ...definition, id: 'd1' } })
    await expect(businessApi.save('ws1', definition)).resolves.toMatchObject({ id: 'd1' })
    expect(api.post).toHaveBeenCalledWith('/api/businessDefinitions.save', { workspace_id: 'ws1', definition })
  })

  it('scopes catalogue reads and safely encodes query parameters', async () => {
    vi.mocked(api.get).mockResolvedValue({ definitions: [], total: 0 })
    await businessApi.list('ws1', 'event', { search: 'a & b', limit: 20, offset: 20 })
    const url = vi.mocked(api.get).mock.calls.at(-1)![0]
    const params = new URLSearchParams(url.split('?')[1])
    expect(params.get('workspace_id')).toBe('ws1')
    expect(params.get('kind')).toBe('event')
    expect(params.get('search')).toBe('a & b')
    expect(params.get('offset')).toBe('20')
  })

  it('previews an unsaved mapping with the provided sample without tracking a real event', async () => {
    vi.mocked(api.post).mockResolvedValue({ valid: true, errors: [], normalized_event: { report_id: 'a1' } })
    const definition = { kind: 'event_mapping' as const, key: 'partner', name: 'Partner', schema: {}, settings: { fields: { report_id: '$.id' } }, status: 'draft' as const }
    await businessApi.preview('ws1', definition, { id: 'a1' })
    expect(api.post).toHaveBeenCalledWith('/api/businessDefinitions.preview', { workspace_id: 'ws1', definition, sample: { id: 'a1' } })
  })
})
