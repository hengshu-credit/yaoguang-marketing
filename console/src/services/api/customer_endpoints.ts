import { api } from './client'
export interface CustomerEndpoint { endpoint_id: string; channel: string; provider: string; platform: string; locale?: string; enabled: boolean }
export interface CustomerEndpointMutation { operation: 'upsert' | 'disable'; endpoint_id: string; channel: string; provider?: string; platform?: string; address?: string; locale?: string }
export const customerEndpointsApi = {
 list: (workspaceId: string, customerId: string, channel: string) => api.get<{ endpoints: CustomerEndpoint[] }>(`/api/contactEndpoints.list?${new URLSearchParams({ workspace_id: workspaceId, customer_id: customerId, channel })}`),
 save: (workspaceId: string, customerId: string, endpoint: CustomerEndpointMutation, marketingConsent?: boolean) => api.post('/api/contactEndpoints.upsert', { workspace_id: workspaceId, customer_id: customerId, endpoint, ...(marketingConsent !== undefined ? { marketing_consent: marketingConsent } : {}) })
}
