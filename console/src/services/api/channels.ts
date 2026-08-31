import { api } from './client'

export type ContentFamily =
  | 'text'
  | 'notification'
  | 'rich_card'
  | 'carousel'
  | 'external_template'
  | 'work_message'
  | 'webhook_payload'

export interface PreviewProfile {
  id: string
  label_key: string
  surface: string
}

export interface ChannelLimits {
  max_title_runes?: number
  max_body_runes?: number
  max_actions?: number
  max_cards?: number
  max_payload_bytes?: number
}

export interface ChannelDefinition {
  id: string
  label_key: string
  regions?: string[]
  recommended_in?: string[]
  content_families: ContentFamily[]
  preview_profiles: PreviewProfile[]
  delivery_modes: Array<'native' | 'signed_webhook'>
  limits: ChannelLimits
}

export interface ChannelCatalogResponse {
  channels: ChannelDefinition[]
}

export const channelsApi = {
  list: (workspaceId: string): Promise<ChannelCatalogResponse> =>
    api.get<ChannelCatalogResponse>(
      `/api/channels.catalog?workspace_id=${encodeURIComponent(workspaceId)}`
    )
}

