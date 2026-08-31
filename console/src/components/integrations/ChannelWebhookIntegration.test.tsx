import { App as AntApp } from 'antd'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChannelDefinition } from '../../services/api/channels'
import type { Workspace } from '../../services/api/workspace'
import { workspaceService } from '../../services/api/workspace'
import ChannelWebhookIntegration from './ChannelWebhookIntegration'

const definitions: ChannelDefinition[] = [{
  id: 'telegram', label_key: 'Telegram', content_families: ['text'],
  preview_profiles: [{ id: 'telegram_mobile', label_key: 'Telegram mobile', surface: 'mobile' }],
  delivery_modes: ['signed_webhook'], limits: {}
}]
const workspace = {
  id: 'ws1', name: 'Workspace', created_at: '', updated_at: '', settings: { timezone: 'UTC', email_tracking_enabled: false, default_language: 'en', languages: ['en'] },
  integrations: [{
    id: 'bridge-1', name: 'Regional bridge', type: 'channel_webhook',
    channel_webhook_settings: { endpoint_url: 'https://bridge.example/send', channels: ['telegram'], timeout_seconds: 5 },
    credential_hints: { 'channel_webhook.secret': 'cret' }, created_at: '', updated_at: ''
  }]
} as Workspace

describe('ChannelWebhookIntegration', () => {
  beforeEach(() => vi.clearAllMocks())

  it('edits metadata without replacing the stored secret with a blank value', async () => {
    const user = userEvent.setup()
    const updateIntegration = vi.spyOn(workspaceService, 'updateIntegration').mockResolvedValue({ status: 'ok' })
    render(<AntApp><ChannelWebhookIntegration workspace={workspace} definitions={definitions} isOwner onSaved={vi.fn().mockResolvedValue(undefined)} /></AntApp>)
    await user.click(screen.getByRole('button', { name: /Edit Regional bridge/i }))
    expect(screen.getByLabelText(/^HTTPS endpoint$/i)).toHaveValue('https://bridge.example/send')
    expect(screen.getByLabelText(/^Replace secret$/i)).toHaveValue('')
    expect(screen.getByLabelText(/^Timeout \(seconds\)$/i)).toHaveValue('5')
    fireEvent.change(screen.getByLabelText(/^Name$/i), { target: { value: 'Central Asia bridge' } })
    expect(screen.getByLabelText(/^Name$/i)).toHaveValue('Central Asia bridge')
    await user.click(screen.getByRole('button', { name: /^Save$/i }))
    await waitFor(() => expect(updateIntegration).toHaveBeenCalled())
    const request = updateIntegration.mock.calls[0][0]
    expect(request.name).toBe('Central Asia bridge')
    expect(request.channel_webhook_settings).not.toHaveProperty('secret')
    expect(request.channel_webhook_settings?.channels).toEqual(['telegram'])
  })

  it('shows a credential hint without exposing plaintext or ciphertext', () => {
    render(<AntApp><ChannelWebhookIntegration workspace={workspace} definitions={definitions} isOwner={false} onSaved={vi.fn().mockResolvedValue(undefined)} /></AntApp>)
    expect(screen.getByText(/ends in cret/i)).toBeVisible()
    expect(screen.queryByText(/plain-secret|ciphertext/i)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Edit Regional bridge/i })).not.toBeInTheDocument()
  })
})
