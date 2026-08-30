import { api } from './client'
import type { DeliveryProgress } from './broadcast'

export type DeliveryStatus =
  | 'planned'
  | 'reserved'
  | 'queued'
  | 'submitting'
  | 'provider_accepted'
  | 'confirmed'
  | 'suppressed'
  | 'deferred'
  | 'transient_failed'
  | 'terminal_failed'
  | 'unknown'
  | 'cancelled'

export interface DeliveryIntent {
  id: string
  effect_key: string
  source_type: string
  source_id: string
  source_version: string
  customer_id?: string
  legacy_identity?: string
  channel: string
  template_id?: string
  template_version?: number
  node_or_phase?: string
  occurrence?: string
  variant?: string
  status: DeliveryStatus
  suppression_reason?: string
  metadata?: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface DeliveryDetail {
  intent: DeliveryIntent
  attempts: Array<Record<string, unknown>>
  reconciliations: Array<Record<string, unknown>>
}

export interface DeliveryListRequest {
  workspace_id: string
  status?: DeliveryStatus
  channel?: string
  source_type?: string
  source_id?: string
  provider?: string
  customer_id?: string
  from?: string
  to?: string
  limit?: number
  offset?: number
}

export const deliveryApi = {
  list: async (
    requestOrWorkspaceId: DeliveryListRequest | string,
    legacyStatus?: DeliveryStatus,
    legacyLimit = 50,
    legacyOffset = 0
  ): Promise<{ deliveries: DeliveryIntent[]; total: number }> => {
    const request: DeliveryListRequest = typeof requestOrWorkspaceId === 'string'
      ? { workspace_id: requestOrWorkspaceId, status: legacyStatus, limit: legacyLimit, offset: legacyOffset }
      : requestOrWorkspaceId
    const params = new URLSearchParams({ workspace_id: request.workspace_id })
    for (const key of ['status', 'channel', 'source_type', 'source_id', 'provider', 'customer_id', 'from', 'to'] as const) {
      const value = request[key]
      if (value) params.set(key, String(value))
    }
    params.set('limit', String(request.limit ?? 50))
    params.set('offset', String(request.offset ?? 0))
    return api.get<{ deliveries: DeliveryIntent[]; total: number }>(`/api/deliveries.list?${params}`)
  },
  get: async (workspaceId: string, intentId: string) => {
    const params = new URLSearchParams({ workspace_id: workspaceId, intent_id: intentId })
    const response = await api.get<{ delivery: DeliveryDetail }>(`/api/deliveries.get?${params}`)
    return response.delivery
  },
  reconcile: (workspaceId: string, intentId: string) =>
    api.post<{ status: 'pending' }>('/api/deliveries.reconcile', { workspace_id: workspaceId, intent_id: intentId }),
  resolveUnknown: (
    workspaceId: string,
    intentId: string,
    action: 'mark_confirmed' | 'mark_terminal_failed' | 'retry_after_verified_not_accepted',
    reason: string
  ) => api.post<{ status: 'resolved' }>('/api/deliveries.resolveUnknown', {
    workspace_id: workspaceId,
    intent_id: intentId,
    action,
    reason
  })
}

export type { DeliveryProgress }
