import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from './client'
import { templateCategoriesApi } from './templateCategories'

vi.mock('./client', () => ({ api: { get: vi.fn(), post: vi.fn() } }))

describe('templateCategoriesApi', () => {
  beforeEach(() => vi.clearAllMocks())

  it('lists inactive categories for administration', async () => {
    vi.mocked(api.get).mockResolvedValue({ categories: [] })
    await templateCategoriesApi.list('workspace 1', true)
    expect(api.get).toHaveBeenCalledWith('/api/templateCategories.list?workspace_id=workspace%201&include_inactive=true')
  })

  it('sends category creation to the workspace API', async () => {
    vi.mocked(api.post).mockResolvedValue({ category: { id: 'vip' } })
    await templateCategoriesApi.create({ workspace_id: 'ws1', id: 'vip', name: 'VIP', purpose: 'marketing', sort_order: 20 })
    expect(api.post).toHaveBeenCalledWith('/api/templateCategories.create', expect.objectContaining({ id: 'vip' }))
  })
})
