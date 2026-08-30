import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from './client'
import { audienceApi, importJobApi } from './marketing'

vi.mock('./client', () => ({ api: { get: vi.fn(), post: vi.fn(), postRaw: vi.fn() } }))

beforeEach(() => vi.clearAllMocks())

describe('marketing APIs', () => {
  it('keeps audience definitions structured instead of accepting SQL', async () => {
    vi.mocked(api.post).mockResolvedValue({ id: 'audience-1' } as never)
    await audienceApi.create('workspace-1', '高意向客户', '', { leaf_type: 'list', ref_id: 'list-1' })
    expect(api.post).toHaveBeenCalledWith('/api/audiences.create', expect.objectContaining({
      workspace_id: 'workspace-1',
      definition: { leaf_type: 'list', ref_id: 'list-1' }
    }))
  })

  it('uploads the original file as a stream with workspace and filename', async () => {
    vi.mocked(api.postRaw).mockResolvedValue({ id: 'job-1' } as never)
    const file = new File(['external_user_id,email\n1,a@example.com\n'], 'customers.csv', { type: 'text/csv' })
    await importJobApi.upload('workspace-1', file)
    const [url, body, contentType] = vi.mocked(api.postRaw).mock.calls[0]
    const params = new URLSearchParams(url.split('?')[1])
    expect(params.get('workspace_id')).toBe('workspace-1')
    expect(params.get('filename')).toBe('customers.csv')
    expect(body).toBe(file)
    expect(contentType).toBe('text/csv')
  })
})

