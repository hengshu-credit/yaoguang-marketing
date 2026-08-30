import { api } from './client'

export type AudienceLeafType = 'list' | 'segment' | 'audience'
export type AudienceOperator = 'union' | 'intersection' | 'exclusion'

export interface AudienceExpression {
  leaf_type?: AudienceLeafType
  ref_id?: string
  operator?: AudienceOperator
  children?: AudienceExpression[]
}

export interface Audience {
  id: string
  name: string
  description?: string
  kind: 'static' | 'dynamic' | 'composite'
  active_version: number
  active_build_id?: string
}

export interface AudienceBuild {
  id: string
  audience_id: string
  audience_version: number
  status: 'pending' | 'building' | 'completed' | 'failed' | 'cancelled'
  member_count: number
  error_detail?: string
}

export interface ImportJob {
  id: string
  status: 'uploading' | 'staged' | 'processing' | 'completed' | 'rejected' | 'cancelled'
  filename: string
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
  create: (workspaceId: string, name: string, description: string, definition: AudienceExpression, kind: Audience['kind'] = 'static') =>
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
  }
}

export const importJobApi = {
  upload: (workspaceId: string, file: File) => {
    const params = new URLSearchParams({ workspace_id: workspaceId, filename: file.name })
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
