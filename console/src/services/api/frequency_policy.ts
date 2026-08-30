import { api } from './client'

export type FrequencyPolicyScope = 'campaign' | 'trigger' | 'workspace_global'
export type FrequencyWindowKind = 'sliding' | 'calendar'

export interface FrequencyPolicy {
  id: string
  version: number
  name: string
  scope: FrequencyPolicyScope
  scope_ref?: string
  channel: string
  max_events: number
  window_kind: FrequencyWindowKind
  window_seconds: number
  timezone?: string
  deny_action: 'suppress' | 'defer'
  priority: number
  enabled: boolean
  created_at: string
}

export type SaveFrequencyPolicyRequest = Omit<FrequencyPolicy, 'id' | 'version' | 'created_at'> & { workspace_id: string; id?: string }

export const frequencyPolicyApi = {
  list: (workspaceId: string) => {
    const query = new URLSearchParams({ workspace_id: workspaceId })
    return api.get<{ policies: FrequencyPolicy[] }>(`/api/frequencyPolicies.list?${query}`)
  },
  save: (request: SaveFrequencyPolicyRequest) =>
    api.post<{ policy: FrequencyPolicy }>('/api/frequencyPolicies.save', request)
}
