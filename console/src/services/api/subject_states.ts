import { api } from './client'
import type { StateValue } from './funnels'
export interface SubjectStateInterval { subject_type: string; subject_id: string; customer_id?: string; state_key: string; value: StateValue; valid_from: string; valid_to?: string; rule_revision: number; projection_revision: number; source: string; reason?: string; event_ids?: string[]; computed_at: string }
export interface SubjectStateCoverage { subject_type: string; as_of: string; watermark: string; total: number; intersection: number; union: number; states: Array<{ state_key: string; satisfied: number; false: number; unknown: number }> }
export const subjectStatesApi = {
 coverage(workspaceId: string, subjectType: string, selectedStateKeys: string[], asOf: string, watermark: string, signal?: AbortSignal) { return api.post<SubjectStateCoverage>('/api/subjectStates.coverage', { workspace_id: workspaceId, subject_type: subjectType, selected_state_keys: selectedStateKeys, as_of: asOf, watermark }, { signal }) },
 list(workspaceId: string, subjectType: string, subjectId: string, history = false) { return api.get<{ states: SubjectStateInterval[] }>(`/api/subjectStates.list?${new URLSearchParams({ workspace_id: workspaceId, subject_type: subjectType, subject_id: subjectId, history: String(history) })}`) },
 set(workspaceId: string, subjectType: string, subjectId: string, stateKey: string, value: StateValue, reason: string, validTo?: string) { return api.post<{ success: boolean }>('/api/subjectStates.set', { workspace_id: workspaceId, subject_type: subjectType, subject_id: subjectId, override: { state_key: stateKey, value, reason, ...(validTo ? { valid_to: validTo } : {}) } }) }
}
