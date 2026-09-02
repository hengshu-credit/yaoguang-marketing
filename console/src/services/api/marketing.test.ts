import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from './client'
import { audienceApi, importJobApi } from './marketing'
import { automationApi } from './automation'
import type { TreeNode } from './segment'

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

  it('creates a dynamic audience from a structured condition tree without building members', async () => {
    vi.mocked(api.post).mockResolvedValue({ id: 'audience-1' } as never)
    const condition: TreeNode = {
      kind: 'leaf',
      leaf: {
        source: 'contacts',
        contact: {
          filters: [{
            field_name: 'profile_status', field_type: 'string', operator: 'equals',
            string_values: ['unpaid']
          }]
        }
      }
    }
    await audienceApi.create('workspace-1', '待还款客户', '', { condition })
    expect(api.post).toHaveBeenCalledWith('/api/audiences.create', {
      workspace_id: 'workspace-1', name: '待还款客户', description: '', kind: 'dynamic',
      definition: { condition }
    })
    expect(api.post).toHaveBeenCalledTimes(1)
  })

  it('evaluates a customer against the current dynamic audience definition', async () => {
    vi.mocked(api.post).mockResolvedValue({ matches: true } as never)

    await audienceApi.matchCustomer('workspace-1', 'audience-1', 'customer-1')

    expect(api.post).toHaveBeenCalledWith('/api/audiences.matchCustomer', {
      workspace_id: 'workspace-1', audience_id: 'audience-1', customer_id: 'customer-1'
    })
  })

  it('loads every audience page for live Customer 360 evaluation', async () => {
    vi.mocked(api.get)
      .mockResolvedValueOnce({
        items: Array.from({ length: 200 }, (_, index) => ({ id: `audience-${index}` })),
        total: 201
      } as never)
      .mockResolvedValueOnce({ items: [{ id: 'audience-200' }], total: 201 } as never)

    const audiences = await audienceApi.listAll('workspace-1')

    expect(audiences).toHaveLength(201)
    expect(api.get).toHaveBeenNthCalledWith(
      1, '/api/audiences.list?workspace_id=workspace-1&limit=200&offset=0'
    )
    expect(api.get).toHaveBeenNthCalledWith(
      2, '/api/audiences.list?workspace_id=workspace-1&limit=200&offset=200'
    )
  })

  it('serializes list member filters without dropping current-fact fields', async () => {
    vi.mocked(api.get).mockResolvedValue({ items: [], next: '' })

    await audienceApi.memberDetails({
      workspace_id: 'workspace-1',
      list_id: 'newsletter',
      status: 'unsubscribed',
      event_name: 'shop.purchase',
      joined_after: '2026-08-01T00:00:00Z',
      joined_before: '2026-09-01T00:00:00Z',
      attribute_key: 'tier',
      attribute_value: 'gold & vip',
      after: '22222222-2222-4222-8222-222222222222',
      limit: 25
    })

    expect(api.get).toHaveBeenCalledWith(
      '/api/audiences.members?workspace_id=workspace-1&list_id=newsletter&status=unsubscribed&event_name=shop.purchase&joined_after=2026-08-01T00%3A00%3A00Z&joined_before=2026-09-01T00%3A00%3A00Z&attribute_key=tier&attribute_value=gold+%26+vip&after=22222222-2222-4222-8222-222222222222&limit=25'
    )
  })

  it('starts a live automation from an audience without choosing a client-side version', async () => {
    vi.mocked(api.post).mockResolvedValue({ run: { build_id: 'build-7' } } as never)
    await automationApi.startAudience('workspace-1', 'automation-1', 'audience-1')
    expect(api.post).toHaveBeenCalledWith('/api/automations.startAudience', {
      workspace_id: 'workspace-1', automation_id: 'automation-1', audience_id: 'audience-1'
    })
  })

  it('uploads the original file as a stream with workspace and filename', async () => {
    vi.mocked(api.postRaw).mockResolvedValue({ id: 'job-1' } as never)
    const file = new File(['external_user_id,email\n1,a@example.com\n'], 'customers.csv', { type: 'text/csv' })
    await importJobApi.upload('workspace-1', file, ['news', 'vip'])
    const [url, body, contentType] = vi.mocked(api.postRaw).mock.calls[0]
    const params = new URLSearchParams(url.split('?')[1])
    expect(params.get('workspace_id')).toBe('workspace-1')
    expect(params.get('filename')).toBe('customers.csv')
    expect(params.getAll('list_id')).toEqual(['news', 'vip'])
    expect(body).toBe(file)
    expect(contentType).toBe('text/csv')
  })
})
