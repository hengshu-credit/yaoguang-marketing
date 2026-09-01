import { api } from './client'

export type TemplateCategoryPurpose = 'marketing' | 'transactional'

export interface TemplateCategoryDefinition {
  id: string
  name: string
  purpose: TemplateCategoryPurpose
  sort_order: number
  is_system: boolean
  is_active: boolean
  usage_count: number
  created_at: string
  updated_at: string
}

export interface TemplateCategoryListResponse {
  categories: TemplateCategoryDefinition[]
}

export interface CreateTemplateCategoryRequest {
  workspace_id: string
  id: string
  name: string
  purpose: TemplateCategoryPurpose
  sort_order: number
}

export interface UpdateTemplateCategoryRequest {
  workspace_id: string
  id: string
  name: string
  sort_order: number
  is_active: boolean
}

export const templateCategoriesApi = {
  list: (workspaceId: string, includeInactive = false): Promise<TemplateCategoryListResponse> =>
    api.get(`/api/templateCategories.list?workspace_id=${encodeURIComponent(workspaceId)}&include_inactive=${includeInactive}`),
  create: (request: CreateTemplateCategoryRequest): Promise<{ category: TemplateCategoryDefinition }> =>
    api.post('/api/templateCategories.create', request),
  update: (request: UpdateTemplateCategoryRequest): Promise<{ category: TemplateCategoryDefinition }> =>
    api.post('/api/templateCategories.update', request),
  delete: (workspaceId: string, id: string): Promise<{ success: boolean }> =>
    api.post('/api/templateCategories.delete', { workspace_id: workspaceId, id })
}

