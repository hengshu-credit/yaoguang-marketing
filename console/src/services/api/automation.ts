import { api } from './client'
import { analyticsService } from './analytics'
import { TreeNode } from './segment'
import type { EmailProviderKind } from './workspace'
import type { DeliveryIntent } from './delivery'

// Email provider kinds that can ingest inbound replies (used to gate the automation
// "Exit on reply" feature). Keep in sync with the backend: a provider belongs here once
// it has a reply parser + a matchable stored Message-ID (and route registration).
// More ESPs will be added as their inbound support ships.
export const INBOUND_REPLY_PROVIDER_KINDS: EmailProviderKind[] = ['mailgun', 'ses']

// AWS regions where Amazon SES supports inbound email RECEIVING (receipt rules). SES inbound
// only works in these; in a sending-only region stop-on-reply can't be provisioned. Mirror of
// the backend sesReceivingRegions allowlist (internal/service/ses_service.go) — keep in sync.
export const SES_RECEIVING_REGIONS: ReadonlySet<string> = new Set([
  'us-east-1',
  'us-east-2',
  'us-west-1',
  'us-west-2',
  'af-south-1',
  'ap-southeast-3',
  'ap-south-1',
  'ap-northeast-3',
  'ap-northeast-2',
  'ap-southeast-1',
  'ap-southeast-2',
  'ap-northeast-1',
  'ca-central-1',
  'eu-central-1',
  'eu-west-1',
  'eu-west-2',
  'eu-south-1',
  'eu-west-3',
  'eu-north-1',
  'il-central-1',
  'me-south-1',
  'sa-east-1'
])

// supportsInboundReplies reports whether an integration can ingest inbound replies (gating the
// automation "Exit on reply" feature). SES additionally requires a receiving-capable region.
export function supportsInboundReplies(provider: {
  kind: EmailProviderKind
  ses?: { region?: string }
}): boolean {
  if (!INBOUND_REPLY_PROVIDER_KINDS.includes(provider.kind)) return false
  if (provider.kind === 'ses') return SES_RECEIVING_REGIONS.has(provider.ses?.region || '')
  return true
}

// Automation status types
export type AutomationStatus = 'draft' | 'live' | 'paused'

// Trigger frequency types
export type TriggerFrequency = 'once' | 'every_time'

export interface JourneyEntryGuard {
  enabled: boolean
  /** Cooldown in nanoseconds, matching Go's time.Duration JSON representation. */
  cooldown?: number
  max_concurrent?: number
}

// Node types
export type NodeType =
  | 'trigger'
  | 'delay'
  | 'email'
  | 'sms'
  | 'push'
  | 'branch'
  | 'filter'
  | 'add_to_list'
  | 'remove_from_list'
  | 'ab_test'
  | 'webhook'
  | 'list_status_branch'

// Contact automation status
export type ContactAutomationStatus = 'active' | 'completed' | 'exited' | 'failed'

// Node action types
export type NodeAction = 'entered' | 'processing' | 'completed' | 'failed' | 'skipped'

// Valid event kinds for automation triggers
export const VALID_EVENT_KINDS = [
  // Contact events
  'contact.created',
  'contact.updated',
  'contact.deleted',
  'contact.profile_created',
  'contact.profile_updated',
  'contact.tagged',
  'contact.untagged',
  // List events (require list_id)
  'list.subscribed',
  'list.unsubscribed',
  'list.confirmed',
  'list.resubscribed',
  'list.bounced',
  'list.complained',
  'list.pending',
  'list.removed',
  // Segment events (require segment_id)
  'segment.joined',
  'segment.left',
  // Email events (sent/delivered omitted: no matching timeline kind, triggers never fire)
  'email.opened',
  'email.clicked',
  'email.bounced',
  'email.complained',
  'email.unsubscribed',
  // Custom events (require custom_event_name)
  'custom_event'
] as const

export type EventKind = (typeof VALID_EVENT_KINDS)[number]

// Trigger configuration
export interface TimelineTriggerConfig {
  event_kind: string
  list_id?: string // Required for list.* events
  segment_id?: string // Required for segment.* events
  custom_event_name?: string // Required for custom_event
  tag?: string // Required for contact.tagged and contact.untagged
  updated_fields?: string[] // For contact.updated: only trigger on these field changes
  conditions?: TreeNode
  frequency: TriggerFrequency
  entry_guard?: JourneyEntryGuard
}

// Automation statistics
export interface AutomationStats {
  enrolled: number
  completed: number
  exited: number
  failed: number
}

// Node position for visual editor
export interface NodePosition {
  x: number
  y: number
}

// Node configuration types

// Every node config carries an optional author-facing description, shown under the node's title on
// the canvas so a flow can be read without opening each node. It lives in the config bag because the
// backend stores that verbatim (AutomationNode.Config is an untyped map), so no schema change.
export interface NodeConfigBase {
  description?: string
}

export interface DelayNodeConfig extends NodeConfigBase {
  duration: number
  unit: 'minutes' | 'hours' | 'days'
}

export interface EmailNodeConfig extends NodeConfigBase {
  template_id: string
  template_version?: number
  integration_id?: string
  subject_override?: string
  from_override?: string
}

export interface ChannelNodeConfig extends NodeConfigBase {
  template_id: string
  template_version?: number
  integration_id: string
  endpoint_id?: string
  language?: string
  data?: Record<string, unknown>
}

export interface BranchPath {
  id: string
  name: string
  conditions?: TreeNode
  next_node_id: string
}

export interface BranchNodeConfig extends NodeConfigBase {
  paths: BranchPath[]
  default_path_id: string
}

export interface FilterNodeConfig extends NodeConfigBase {
  conditions?: TreeNode
  continue_node_id: string
  exit_node_id: string
}

export interface AddToListNodeConfig extends NodeConfigBase {
  list_id: string
  status: 'active' | 'pending'
  metadata?: Record<string, unknown>
}

export interface RemoveFromListNodeConfig extends NodeConfigBase {
  list_id: string
}

export interface ListStatusBranchNodeConfig extends NodeConfigBase {
  list_id: string
  not_in_list_node_id: string
  active_node_id: string
  non_active_node_id: string
}

export interface ABTestVariant {
  id: string
  name: string
  weight: number
  next_node_id: string
}

export interface ABTestNodeConfig extends NodeConfigBase {
  variants: ABTestVariant[]
}

export interface WebhookNodeConfig extends NodeConfigBase {
  url: string
  secret?: string // Optional Authorization Bearer token
}

// Union type for node configs
export type NodeConfig =
  | DelayNodeConfig
  | EmailNodeConfig
  | ChannelNodeConfig
  | BranchNodeConfig
  | FilterNodeConfig
  | AddToListNodeConfig
  | RemoveFromListNodeConfig
  | ListStatusBranchNodeConfig
  | ABTestNodeConfig
  | WebhookNodeConfig
  | Record<string, unknown> // For trigger nodes with no config

// Automation node
export interface AutomationNode {
  id: string
  automation_id: string
  type: NodeType
  config: Record<string, unknown>
  next_node_id?: string
  position: NodePosition
  created_at: string
}

// Main automation interface
export interface Automation {
  id: string
  workspace_id: string
  name: string
  status: AutomationStatus
  list_id: string
  exit_on_reply?: boolean // Stop the journey when the contact replies (requires inbound reply setup at the ESP)
  trigger?: TimelineTriggerConfig
  trigger_sql?: string
  root_node_id: string
  nodes: AutomationNode[]
  stats?: AutomationStats
  created_at: string
  updated_at: string
  deleted_at?: string
}

// Contact automation tracking
export interface ContactAutomation {
  id: string
  automation_id: string
  contact_email: string
  current_node_id?: string
  status: ContactAutomationStatus
  exit_reason?: string
  entered_at: string
  scheduled_at?: string
  context?: Record<string, unknown>
  retry_count: number
  last_error?: string
  last_retry_at?: string
  max_retries: number
}

// Node execution log
export interface NodeExecution {
  id: string
  contact_automation_id: string
  node_id: string
  node_type: NodeType
  action: NodeAction
  entered_at: string
  completed_at?: string
  duration_ms?: number
  output?: Record<string, unknown>
  error?: string
}

// API Request types
export interface ListAutomationsRequest {
  workspace_id: string
  status?: AutomationStatus[]
  list_id?: string
  limit?: number
  offset?: number
}

export interface ListAutomationsResponse {
  automations: Automation[]
  total: number
}

export interface GetAutomationRequest {
  workspace_id: string
  automation_id: string
}

export interface GetAutomationResponse {
  automation: Automation
}

export interface CreateAutomationRequest {
  workspace_id: string
  automation: Automation
}

export interface UpdateAutomationRequest {
  workspace_id: string
  automation: Automation
}

export interface DeleteAutomationRequest {
  workspace_id: string
  automation_id: string
}

export interface ActivateAutomationRequest {
  workspace_id: string
  automation_id: string
  preflight_hash: string
  confirm_warnings?: boolean
}

export interface JourneyPreflightIssue {
  code: string
  severity: 'blocking' | 'warning'
  title: string
  description: string
  node_id?: string
  fix_path?: string
}

export interface JourneyPreflightResult {
  workspace_id: string
  automation_id: string
  issues: JourneyPreflightIssue[]
  blocking_count: number
  warning_count: number
  summary_hash: string
  generated_at: string
  expires_at: string
}

export interface JourneyInstanceSummary {
  id: string
  enrollment_id?: string
  contact_automation_id?: string
  automation_id: string
  automation_name: string
  customer_id: string
  customer_no: string
  external_user_id?: string
  contact_email?: string
  frequency: TriggerFrequency
  origin_event_id?: string
  entry_decision: string
  entry_reason?: string
  status: ContactAutomationStatus
  current_node_id?: string
  waiting_reason?: string
  next_scheduled_at?: string
  started_at: string
  completed_at?: string
}

export interface JourneyEntryDecision {
  id: string
  automation_id: string
  customer_id: string
  origin_event_id?: string
  decision: string
  reason?: string
  retry_at?: string
  decided_at: string
}

export interface JourneyTraceEvent {
  id: string
  node_id?: string
  event_type: string
  status: string
  reason?: string
  payload?: Record<string, unknown>
  occurred_at: string
}

export interface JourneyDeliveryLink {
  intent: DeliveryIntent
  attempts?: Array<Record<string, unknown>>
  receipts?: Array<Record<string, unknown>>
}

export interface JourneyTrace {
  instance: JourneyInstanceSummary
  entry_decisions: JourneyEntryDecision[]
  events: JourneyTraceEvent[]
  deliveries: JourneyDeliveryLink[]
}

export interface ListJourneyInstancesRequest {
  workspace_id: string
  customer_id?: string
  customer_no?: string
  external_user_id?: string
  email?: string
  automation_id?: string
  status?: ContactAutomationStatus
  limit?: number
  offset?: number
}

export interface PauseAutomationRequest {
  workspace_id: string
  automation_id: string
}

export interface GetNodeExecutionsRequest {
  workspace_id: string
  automation_id: string
  email: string
}

export interface GetNodeExecutionsResponse {
  contact_automation: ContactAutomation
  node_executions: NodeExecution[]
}

// Node stats for flow viewer
export interface AutomationNodeStats {
  node_id: string
  node_type: NodeType
  entered: number
  completed: number
  failed: number
  skipped: number
}

export interface GetNodeStatsRequest {
  workspace_id: string
  automation_id: string
}

export interface GetNodeStatsResponse {
  node_stats: Record<string, AutomationNodeStats>
}

export interface AutomationAudienceRunResult {
  automation_id: string
  audience_id: string
  audience_version: number
  build_id: string
  candidate_count: number
  enrolled_count: number
}

// API client
export const automationApi = {
	startAudience: async (
		workspaceId: string,
		automationId: string,
		audienceId: string
	): Promise<AutomationAudienceRunResult> => {
		const response = await api.post<AutomationAudienceRunResult | { run: AutomationAudienceRunResult }>(
			'/api/automations.startAudience',
			{ workspace_id: workspaceId, automation_id: automationId, audience_id: audienceId }
		)
		return 'run' in response ? response.run : response
	},
  list: async (params: ListAutomationsRequest): Promise<ListAutomationsResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    if (params.status && params.status.length > 0) {
      params.status.forEach((s) => searchParams.append('status', s))
    }
    if (params.list_id) searchParams.append('list_id', params.list_id)
    if (params.limit) searchParams.append('limit', params.limit.toString())
    if (params.offset) searchParams.append('offset', params.offset.toString())

    return api.get<ListAutomationsResponse>(`/api/automations.list?${searchParams.toString()}`)
  },

  get: async (params: GetAutomationRequest): Promise<GetAutomationResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('automation_id', params.automation_id)

    return api.get<GetAutomationResponse>(`/api/automations.get?${searchParams.toString()}`)
  },

  create: async (params: CreateAutomationRequest): Promise<GetAutomationResponse> => {
    return api.post<GetAutomationResponse>('/api/automations.create', params)
  },

  update: async (params: UpdateAutomationRequest): Promise<GetAutomationResponse> => {
    return api.post<GetAutomationResponse>('/api/automations.update', params)
  },

  delete: async (params: DeleteAutomationRequest): Promise<{ success: boolean }> => {
    return api.post<{ success: boolean }>('/api/automations.delete', params)
  },

  preflight: async (params: {
    workspace_id: string
    automation_id: string
  }): Promise<JourneyPreflightResult> => {
    const response = await api.post<JourneyPreflightResult | { preflight: JourneyPreflightResult }>(
      '/api/automations.preflight',
      params
    )
    return 'preflight' in response ? response.preflight : response
  },

  listJourneyInstances: async (
    params: ListJourneyInstancesRequest
  ): Promise<{ instances: JourneyInstanceSummary[]; total: number; limit: number; offset: number }> => {
    const searchParams = new URLSearchParams({ workspace_id: params.workspace_id })
    for (const key of ['customer_id', 'customer_no', 'external_user_id', 'email', 'automation_id', 'status'] as const) {
      const value = params[key]
      if (value) searchParams.set(key, String(value))
    }
    if (params.limit !== undefined) searchParams.set('limit', String(params.limit))
    if (params.offset !== undefined) searchParams.set('offset', String(params.offset))
    return api.get(`/api/journeys.instances?${searchParams.toString()}`)
  },

  getJourneyTrace: async (workspaceId: string, journeyInstanceId: string): Promise<JourneyTrace> => {
    const searchParams = new URLSearchParams({
      workspace_id: workspaceId,
      journey_instance_id: journeyInstanceId
    })
    const response = await api.get<{ trace: JourneyTrace }>(
      `/api/journeys.trace?${searchParams.toString()}`
    )
    return response.trace
  },

  activate: async (params: ActivateAutomationRequest): Promise<GetAutomationResponse> => {
    return api.post<GetAutomationResponse>('/api/automations.activate', params)
  },

  pause: async (params: PauseAutomationRequest): Promise<GetAutomationResponse> => {
    return api.post<GetAutomationResponse>('/api/automations.pause', params)
  },

  getNodeExecutions: async (
    params: GetNodeExecutionsRequest
  ): Promise<GetNodeExecutionsResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('automation_id', params.automation_id)
    searchParams.append('email', params.email)

    return api.get<GetNodeExecutionsResponse>(
      `/api/automations.nodeExecutions?${searchParams.toString()}`
    )
  },

  getNodeStats: async (params: GetNodeStatsRequest): Promise<GetNodeStatsResponse> => {
    const response = await analyticsService.query(
      {
        schema: 'automation_node_executions',
        measures: ['count_entered', 'count_completed', 'count_failed', 'count_skipped'],
        dimensions: ['node_id', 'node_type'],
        filters: [
          {
            member: 'automation_id',
            operator: 'equals',
            values: [params.automation_id]
          }
        ]
      },
      params.workspace_id
    )

    // Transform analytics response (array) to map format expected by components
    const node_stats: Record<string, AutomationNodeStats> = {}
    for (const row of response.data) {
      const nodeId = row.node_id as string
      node_stats[nodeId] = {
        node_id: nodeId,
        node_type: row.node_type as NodeType,
        entered: (row.count_entered as number) || 0,
        completed: (row.count_completed as number) || 0,
        failed: (row.count_failed as number) || 0,
        skipped: (row.count_skipped as number) || 0
      }
    }
    return { node_stats }
  }
}
