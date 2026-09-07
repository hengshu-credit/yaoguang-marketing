import { api } from './client'
export interface ExecutionSnapshot {
 id: string
 execution_id: string
 source_id: string
 kind: 'campaign' | 'journey' | 'decision'
 configuration: Record<string, unknown>
 materials: unknown
 dependencies: Record<string, unknown>
 content_hash: string
 completeness: 'complete' | 'partial' | 'legacy'
 missing: string[]
 created_at: string
}
export interface ExecutionEvidence {
 id: string
 execution_id: string
 key: string
 kind: string
 node_id?: string
 customer_id?: string
 delivery_id?: string
 status: string
 payload: Record<string, unknown>
 occurred_at: string
}
export interface ExecutionDetailsResponse { snapshot: ExecutionSnapshot; evidence: ExecutionEvidence[]; next?: string }
export interface ObservationCounts { occurrences: number; customers: number; objects: number }
export interface ObservationMetric { key: string; goal: boolean; observed: ObservationCounts; attributed: ObservationCounts; event_ids: string[] }
export interface ExecutionObservationsResponse { execution_id: string; plan: { subject_type: string; window_seconds: number } | null; watermark: string; cohort_customers: number; events: ObservationMetric[]; states: ObservationMetric[] }
export const executionsApi = {
 observations: (workspaceId: string, executionId: string) => api.get<ExecutionObservationsResponse>(`/api/executions.observations?${new URLSearchParams({ workspace_id: workspaceId, execution_id: executionId })}`),
 asset: (workspaceId: string, executionId: string, hash: string) => api.getBlob(`/api/executions.asset?${new URLSearchParams({ workspace_id: workspaceId, execution_id: executionId, hash })}`),
 get: (workspaceId: string, executionId: string, after?: string) => api.get<ExecutionDetailsResponse>(`/api/executions.get?${new URLSearchParams({ workspace_id: workspaceId, execution_id: executionId, ...(after ? { after } : {}) })}`),
 list: (workspaceId: string, sourceId: string, before?: string) => api.get<{ items: ExecutionSnapshot[]; next?: string }>(`/api/executions.list?${new URLSearchParams({ workspace_id: workspaceId, source_id: sourceId, ...(before ? { before } : {}) })}`)
}
