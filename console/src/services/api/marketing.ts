import { api } from './client'
import type { TreeNode } from './segment'
import type { CustomerListMembership, CustomerSummary } from './customer'

export type AudienceLeafType = 'list' | 'segment' | 'audience'
export type AudienceOperator = 'union' | 'intersection' | 'exclusion'

export interface AudienceExpression {
  leaf_type?: AudienceLeafType
  ref_id?: string
  operator?: AudienceOperator
  children?: AudienceExpression[]
  condition?: TreeNode
}

export interface Audience {
  id: string
  name: string
  description?: string
  kind: 'static' | 'dynamic' | 'composite'
  active_version: number
  active_build_id?: string
  definition?: AudienceExpression
}

export interface AudienceBuild {
  id: string
  audience_id: string
  audience_version: number
  status: 'pending' | 'building' | 'completed' | 'failed' | 'cancelled'
  member_count: number
  error_detail?: string
}

export interface AudienceMember {
  customer: CustomerSummary
  subscriptions: CustomerListMembership[]
  joined_at?: string
}

export interface AudienceMemberRequest {
  workspace_id: string
  list_id?: string
  audience_id?: string
  build_id?: string
  status?: string
  event_name?: string
  joined_after?: string
  joined_before?: string
  attribute_key?: string
  attribute_value?: string
  after?: string
  limit?: number
}

export interface ImportJob {
  id: string
  status: 'uploading' | 'staged' | 'processing' | 'completed' | 'rejected' | 'cancelled'
  filename: string
  list_ids?: string[]
  counters: { total: number; pending: number; processing: number; succeeded: number; failed: number }
  created_at?: string
  updated_at?: string
}

export interface ImportJobError {
  ordinal: number
  external_user_id?: string
  display_identity?: string
  error_code: string
  error_detail?: string
}

export const audienceApi = {
  list: (workspaceId: string, limit = 100, offset = 0) => {
    const params = new URLSearchParams({ workspace_id: workspaceId, limit: String(limit), offset: String(offset) })
    return api.get<{ items: Audience[]; total: number }>(`/api/audiences.list?${params}`)
  },
  get: (workspaceId: string, audienceId: string) => {
    const params = new URLSearchParams({ workspace_id: workspaceId, audience_id: audienceId })
    return api.get<Audience>(`/api/audiences.get?${params}`)
  },
  create: (workspaceId: string, name: string, description: string, definition: AudienceExpression, kind: Audience['kind'] = 'dynamic') =>
    api.post<Audience>('/api/audiences.create', {
      workspace_id: workspaceId,
      name,
      description,
      kind,
      definition
    }),
  update: (workspaceId: string, audienceId: string, definition: AudienceExpression) =>
    api.post<{ audience_id: string; version: number }>('/api/audiences.update', { workspace_id: workspaceId, audience_id: audienceId, definition }),
  delete: (workspaceId: string, audienceId: string) =>
    api.post<{ deleted: boolean }>('/api/audiences.delete', { workspace_id: workspaceId, audience_id: audienceId }),
  preview: (workspaceId: string, definition: AudienceExpression) =>
    api.post<{ customers: Array<Record<string, unknown>>; total: number }>('/api/audiences.preview', {
      workspace_id: workspaceId,
      definition
    }),
  build: (workspaceId: string, audienceId: string, version: number) =>
    api.post<{ build_id: string; member_count: number }>('/api/audiences.build', {
      workspace_id: workspaceId,
      audience_id: audienceId,
      version
    }),
  buildStatus: (workspaceId: string, buildId: string) => {
    const params = new URLSearchParams({ workspace_id: workspaceId, build_id: buildId })
    return api.get<AudienceBuild>(`/api/audiences.buildStatus?${params}`)
  },
  members: (workspaceId: string, buildId: string, after = '', limit = 50) => {
    const params = new URLSearchParams({ workspace_id: workspaceId, build_id: buildId, after, limit: String(limit) })
    return api.get<{ items: Array<Record<string, unknown>>; next: string }>(`/api/audiences.members?${params}`)
  },
  memberDetails: (request: AudienceMemberRequest) => {
    const params = new URLSearchParams({ workspace_id: request.workspace_id })
    if (request.list_id) params.set('list_id', request.list_id)
    if (request.audience_id) params.set('audience_id', request.audience_id)
    if (request.build_id) params.set('build_id', request.build_id)
    if (request.status) params.set('status', request.status)
    if (request.event_name) params.set('event_name', request.event_name)
    if (request.joined_after) params.set('joined_after', request.joined_after)
    if (request.joined_before) params.set('joined_before', request.joined_before)
    if (request.attribute_key) params.set('attribute_key', request.attribute_key)
    if (request.attribute_value) params.set('attribute_value', request.attribute_value)
    if (request.after) params.set('after', request.after)
    if (request.limit) params.set('limit', String(request.limit))
    return api.get<{ items: AudienceMember[]; next: string }>(`/api/audiences.members?${params}`)
  }
}

export const importJobApi = {
  upload: (workspaceId: string, file: File, listIds: string[] = []) => {
    const params = new URLSearchParams({ workspace_id: workspaceId, filename: file.name })
    listIds.forEach((listId) => params.append('list_id', listId))
    return api.postRaw<ImportJob>(`/api/imports.upload?${params}`, file, 'text/csv')
  },
  get: (workspaceId: string, jobId: string) => {
    const params = new URLSearchParams({ workspace_id: workspaceId, job_id: jobId })
    return api.get<ImportJob>(`/api/imports.get?${params}`)
  },
  list: (workspaceId: string, limit = 50, offset = 0) => {
    const params = new URLSearchParams({ workspace_id: workspaceId, limit: String(limit), offset: String(offset) })
    return api.get<{ items: ImportJob[]; total: number; limit: number; offset: number }>(`/api/imports.list?${params}`)
  },
  cancel: (workspaceId: string, jobId: string) => {
    const params = new URLSearchParams({ workspace_id: workspaceId, job_id: jobId })
    return api.postRaw<{ cancelled: boolean }>(`/api/imports.cancel?${params}`, '', 'text/plain')
  },
  errors: (workspaceId: string, jobId: string, limit = 100, offset = 0) => {
    const params = new URLSearchParams({ workspace_id: workspaceId, job_id: jobId, limit: String(limit), offset: String(offset) })
    return api.get<{ items: ImportJobError[]; total: number }>(`/api/imports.errors?${params}`)
  },
  downloadErrors: async (workspaceId: string, jobId: string) => {
    const params = new URLSearchParams({ workspace_id: workspaceId, job_id: jobId, limit: '10000', format: 'csv' })
    const blob = await api.getBlob(`/api/imports.errors?${params}`)
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `import-${jobId}-errors.csv`
    anchor.click()
    URL.revokeObjectURL(url)
  },
  process: (workspaceId: string, jobId: string) => {
    const params = new URLSearchParams({ workspace_id: workspaceId, job_id: jobId })
    return api.postRaw<{ processed: number }>(`/api/imports.process?${params}`, '', 'text/plain')
  }
}
