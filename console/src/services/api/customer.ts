import { api } from './client'

export interface CustomerIdentity {
  id: string
  type: string
  display_hint: string
  verified: boolean
  primary: boolean
  enabled: boolean
  metadata?: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface CustomerProfile {
  customer_id: string
  status?: string
  language?: string
  timezone?: string
  attributes: Record<string, unknown>
  version: number
  created_at: string
  updated_at: string
}

export interface CustomerListMembership {
  list_id: string
  status: string
  created_at: string
  updated_at: string
}

export interface CustomerConsent {
  id: string
  purpose: string
  channel: string
  status: string
  source?: string
  valid_from: string
  revoked_at?: string
  metadata?: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface CustomerSummary {
  customer_id: string
  customer_no: string
  external_user_id?: string
  merged_into_id?: string
  resolved_from_customer_id?: string
  version: number
  profile?: CustomerProfile
  identities: CustomerIdentity[]
  tags: string[]
  list_memberships?: CustomerListMembership[]
  consents?: CustomerConsent[]
  created_at: string
  updated_at: string
}

export type Customer = CustomerSummary

export interface CustomerListRequest {
  workspace_id: string
  search?: string
  cursor?: string
  limit?: number
  include_merged?: boolean
}

export interface CustomerListResponse {
  customers: CustomerSummary[]
  next_cursor?: string
}

export const customerQueryKeys = {
  all: (workspaceId: string) => ['customers', workspaceId] as const,
  list: (
    workspaceId: string,
    request: Pick<CustomerListRequest, 'search' | 'cursor' | 'limit' | 'include_merged'>
  ) => ['customers', workspaceId, 'list', request] as const,
  detail: (workspaceId: string, customerId: string) =>
    ['customers', workspaceId, 'detail', customerId] as const
}

export const customerApi = {
  list: async (request: CustomerListRequest): Promise<CustomerListResponse> => {
    const params = new URLSearchParams({ workspace_id: request.workspace_id })
    const search = request.search?.trim()
    if (search) params.set('search', search)
    if (request.cursor) params.set('cursor', request.cursor)
    if (request.limit) params.set('limit', String(request.limit))
    if (request.include_merged) params.set('include_merged', 'true')
    return api.get<CustomerListResponse>(`/api/customers.list?${params.toString()}`)
  },

  get: async (workspaceId: string, customerId: string): Promise<Customer> => {
    const response = await api.post<{ customer: Customer }>('/api/customers.get', {
      workspace_id: workspaceId,
      locator: { customer_id: customerId }
    })
    return response.customer
  }
}
