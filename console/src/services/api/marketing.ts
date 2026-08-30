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

export interface ImportJob {
  id: string
  status: 'uploading' | 'staged' | 'processing' | 'completed' | 'rejected' | 'cancelled'
  filename: string
  counters: { total: number; pending: number; processing: number; succeeded: number; failed: number }
}

export const audienceApi = {
  create: (workspaceId: string, name: string, description: string, definition: AudienceExpression) =>
    api.post<Audience>('/api/audiences.create', {
      workspace_id: workspaceId,
      name,
      description,
      kind: 'static',
      definition
    }),
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
    })
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
  process: (workspaceId: string, jobId: string) => {
    const params = new URLSearchParams({ workspace_id: workspaceId, job_id: jobId })
    return api.postRaw<{ processed: number }>(`/api/imports.process?${params}`, '', 'text/plain')
  }
}

