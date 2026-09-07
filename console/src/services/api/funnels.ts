import { api } from './client'
export type StateValue = 'true' | 'false' | 'unknown'
export interface StateRule { kind: 'event' | 'attribute' | 'state' | 'all' | 'any' | 'not' | 'object_any'; children?: StateRule[]; event?: string; revoke_event?: string; window_seconds?: number; minimum?: number; field?: string; operator?: string; operand?: unknown; state_key?: string; value?: StateValue; subject_type?: string; relation_key?: string }
export interface FunnelStep { id: string; subject_type?: string; relation_key?: string; name?: string; events?: string[]; all_events?: string[]; optional?: boolean; window_seconds?: number; filter?: StateRule; state_filters?: Record<string, StateValue> }
export interface FunnelDefinition { subject_type: string; customer_via_object?: boolean; window_seconds: number; late_grace_seconds?: number; allow_same_time?: boolean; steps: FunnelStep[]; scope?: 'all_site' | 'recipients' | 'attributed'; run_id?: string; attribution_window_seconds?: number }
export interface FunnelExplanation { subject_type: string; subject_id: string; customer_id: string; reached: number; event_ids: string[]; started_at: string; last_start: string; deadline: string; mature: boolean; attribution?: { run_id?: string; touchpoint_id?: string; model?: string } }
export interface FunnelResult { steps: Array<{ id: string; objects: number; occurrences: number; pending: number; expired: number; step_rate: number | null; cumulative_rate: number | null; amount_minor: Record<string, number>; branches: Record<string, number> }>; entered: number; converted: number; mature: number; pending: number; excluded: number; watermark: string; computed_at: string; details?: FunnelExplanation[] }
export interface FunnelQueryJob { id: string; status: 'running' | 'queued' | 'completed' | 'failed' | 'cancelled'; result?: FunnelResult; error?: string; created_at: string; updated_at: string }
export const funnelsApi = {
 query: (workspaceId: string, definition: FunnelDefinition, start: string, end: string, includeDetails = false, signal?: AbortSignal) => api.post<FunnelResult>('/api/funnels.query', { workspace_id: workspaceId, definition, start, end, include_details: includeDetails }, { signal }),
 start: (workspaceId: string, definition: FunnelDefinition, start: string, end: string, includeDetails = false, signal?: AbortSignal) => api.post<FunnelQueryJob>('/api/funnels.start', { workspace_id: workspaceId, definition, start, end, include_details: includeDetails }, { signal }),
 get: (workspaceId: string, id: string) => api.get<FunnelQueryJob>(`/api/funnels.get?${new URLSearchParams({ workspace_id: workspaceId, id })}`),
 cancel: (workspaceId: string, id: string) => api.post<{ success: boolean }>('/api/funnels.cancel', { workspace_id: workspaceId, id })
}
