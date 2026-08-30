import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from './client'
import { customerApi, customerQueryKeys } from './customer'

vi.mock('./client', () => ({ api: { get: vi.fn(), post: vi.fn() } }))

beforeEach(() => {
  vi.mocked(api.get).mockReset()
  vi.mocked(api.post).mockReset()
})

describe('customerApi', () => {
  it('serializes the complete stable list cursor request', async () => {
    vi.mocked(api.get).mockResolvedValue({ customers: [], next_cursor: 'next' })

    await customerApi.list({
      workspace_id: 'ws1',
      search: ' alice ',
      cursor: 'cursor-1',
      include_merged: true,
      limit: 50
    })

    const url = vi.mocked(api.get).mock.calls[0][0]
    const params = new URLSearchParams(url.split('?')[1])
    expect(url.startsWith('/api/customers.list?')).toBe(true)
    expect(params.get('workspace_id')).toBe('ws1')
    expect(params.get('search')).toBe('alice')
    expect(params.get('cursor')).toBe('cursor-1')
    expect(params.get('include_merged')).toBe('true')
    expect(params.get('limit')).toBe('50')
  })

  it('loads one customer by UUID without exposing another workspace', async () => {
    vi.mocked(api.post).mockResolvedValue({ customer: { customer_id: 'customer-1' } })

    await expect(customerApi.get('ws1', 'customer-1')).resolves.toEqual({
      customer_id: 'customer-1'
    })
    expect(api.post).toHaveBeenCalledWith('/api/customers.get', {
      workspace_id: 'ws1',
      locator: { customer_id: 'customer-1' }
    })
  })

  it('scopes list and detail cache keys by workspace', () => {
    expect(customerQueryKeys.list('ws1', { search: 'a', cursor: '' })).not.toEqual(
      customerQueryKeys.list('ws2', { search: 'a', cursor: '' })
    )
    expect(customerQueryKeys.detail('ws1', 'customer-1')).not.toEqual(
      customerQueryKeys.detail('ws2', 'customer-1')
    )
  })
})
