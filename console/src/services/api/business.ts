import { api } from './client'
import type { BusinessSchema } from '../../components/business/business-schema'

export type BusinessDefinitionKind = 'customer_attribute' | 'object_type' | 'object_relation' | 'event' | 'event_mapping' | 'state' | 'funnel'
export interface BusinessDefinition {
  id?: string
  kind: BusinessDefinitionKind
  key: string
  name: string
  description?: string
  schema: BusinessSchema
  settings: Record<string, unknown>
  status: 'draft' | 'active' | 'archived'
  revision?: number
  builtin?: boolean
  created_at?: string
  updated_at?: string
}

export interface BusinessPreviewResult {
  valid: boolean
  errors: Array<{ field: string; message: string }>
  normalized_event?: Record<string, unknown>
  resolved_customer_id?: string
  attribute_changes?: Record<string, unknown>
}

export const businessApi = {
  list(workspaceId: string, kind: BusinessDefinitionKind, options: { search?: string; limit?: number; offset?: number } = {}) {
    const query = new URLSearchParams({ workspace_id: workspaceId, kind, limit: String(options.limit ?? 100), offset: String(options.offset ?? 0) })
    if (options.search) query.set('search', options.search)
    return api.get<{ definitions: BusinessDefinition[]; total: number }>(`/api/businessDefinitions.list?${query}`)
  },
  async get(workspaceId: string, kind: BusinessDefinitionKind, key: string) {
    const query = new URLSearchParams({ workspace_id: workspaceId, kind, key })
    return (await api.get<{ definition: BusinessDefinition }>(`/api/businessDefinitions.get?${query}`)).definition
  },
  async save(workspaceId: string, definition: BusinessDefinition) {
    return (await api.post<{ definition: BusinessDefinition }>('/api/businessDefinitions.save', { workspace_id: workspaceId, definition })).definition
  },
  archive(workspaceId: string, definition: BusinessDefinition) {
    return api.post<{ success: boolean }>('/api/businessDefinitions.archive', { workspace_id: workspaceId, kind: definition.kind, key: definition.key, revision: definition.revision })
  },
  preview(workspaceId: string, definition: BusinessDefinition, sample: Record<string, unknown>) {
    return api.post<BusinessPreviewResult>('/api/businessDefinitions.preview', { workspace_id: workspaceId, definition, sample })
  }
}
