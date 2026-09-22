import { api } from './client'
export interface CustomerTrackedEvent {
  source: string; report_id: string; event_id: string; customer_id?: string; reported_at: string; duplicate: boolean; processing_status: string
  event?: { event_code: string; event_name: string; data: Record<string, unknown>; customer_attributes: Record<string, unknown>; triggered_at: string; reported_at: string; expires_at: string | null; definition_revision: number; mapping_revision: number; processing_status: string; trusted_touchpoint_id?: string }
}
export interface CustomerBusinessObject { id: string; object_type: string; source: string; external_key: string; customer_id: string; attributes: Record<string, unknown>; definition_revision: number; created_at: string; updated_at: string }
export const businessActivityApi = {
  events(workspaceId: string, customerId: string) { return api.get<{events: CustomerTrackedEvent[]}>(`/api/events.list?${new URLSearchParams({workspace_id:workspaceId,customer_id:customerId,limit:'100'})}`) },
  objects(workspaceId: string, customerId: string) { return api.get<{objects: CustomerBusinessObject[]}>(`/api/businessObjects.list?${new URLSearchParams({workspace_id:workspaceId,customer_id:customerId,limit:'100'})}`) },
  pending(workspaceId: string, limit = 100) { return api.get<{events: CustomerTrackedEvent[]}>(`/api/events.pending?${new URLSearchParams({workspace_id: workspaceId, limit: String(limit)})}`) },
  resolvePending(request: { workspace_id: string; source: string; report_id: string; customer_id: string; reason: string }) {
    return api.post<CustomerTrackedEvent>('/api/events.resolvePending', request)
  }
}
