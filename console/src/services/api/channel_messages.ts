import { api } from './client'

export type ChannelMessageChannel = 'sms' | 'push'
export type ChannelSendStatus = 'reserved' | 'submitted' | 'confirmed' | 'failed' | 'unknown'

export interface SendChannelMessageRequest {
  workspace_id: string
  effect_key: string
  channel: ChannelMessageChannel
  integration_id: string
  contact_email: string
  endpoint_id?: string
  template_id: string
  template_version?: number
  language?: string
  data?: Record<string, unknown>
  metadata?: Record<string, unknown>
}

export interface ChannelSendExecution {
  effect_key: string
  request_hash: string
  message_id: string
  channel: ChannelMessageChannel
  integration_id: string
  contact_email: string
  endpoint_id: string
  template_id: string
  template_version: number
  language?: string
  status: ChannelSendStatus
  provider?: string
  provider_message_id?: string
  attempts: number
  last_error?: string
  created_at: string
  updated_at: string
}

export interface SendChannelMessageResponse {
  execution: ChannelSendExecution
  duplicate: boolean
}

export const channelMessagesApi = {
  send: (request: SendChannelMessageRequest): Promise<SendChannelMessageResponse> =>
    api.post<SendChannelMessageResponse>('/api/channelMessages.send', request)
}
