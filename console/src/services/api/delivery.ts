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
  channel: string
  status: DeliveryStatus
  created_at: string
  updated_at: string
}

export interface DeliveryDetail {
  intent: DeliveryIntent
  attempts: Array<Record<string, unknown>>
  reconciliations: Array<Record<string, unknown>>
}

export const deliveryApi = {
  list: async (workspaceId: string, status?: DeliveryStatus, limit = 50, offset = 0) => {
    const params = new URLSearchParams({ workspace_id: workspaceId, limit: String(limit), offset: String(offset) })
    if (status) params.set('status', status)
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
