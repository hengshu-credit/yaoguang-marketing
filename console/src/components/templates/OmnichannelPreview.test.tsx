import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { ChannelDefinition } from '../../services/api/channels'
import type { GenericChannelPreview } from '../../services/api/template'
import OmnichannelPreview from './OmnichannelPreview'

const definition: ChannelDefinition = {
  id: 'whatsapp', label_key: 'WhatsApp', content_families: ['rich_card'],
  preview_profiles: [
    { id: 'whatsapp_android', label_key: 'WhatsApp Android', surface: 'mobile' },
    { id: 'whatsapp_web', label_key: 'WhatsApp Web', surface: 'desktop' }
  ],
  delivery_modes: ['signed_webhook'], limits: {}
}

const preview: GenericChannelPreview = {
  profile: 'whatsapp_android', direction: 'ltr', payload_bytes: 312,
  message: {
    family: 'rich_card', title: 'Member offer', body: 'Save 20% today',
    media: { type: 'image', url: 'https://cdn.example/offer.png', alt_text: 'Offer' },
    actions: [{ type: 'url', label: 'Shop now', value: 'https://shop.example' }]
  },
  warnings: []
}

describe('OmnichannelPreview', () => {
  it('renders structured content as a simulated client surface', () => {
    render(<OmnichannelPreview definition={definition} preview={preview} onProfileChange={vi.fn()} />)
    expect(screen.getByText(/Simulated preview/i)).toBeVisible()
    expect(screen.getByText('Member offer')).toBeVisible()
    expect(screen.getByText('Save 20% today')).toBeVisible()
    expect(screen.getByRole('img', { name: 'Offer' })).toHaveAttribute('src', 'https://cdn.example/offer.png')
    expect(screen.getByRole('link', { name: 'Shop now' })).toHaveAttribute('href', 'https://shop.example')
    expect(screen.getByText(/312 bytes/i)).toBeVisible()
  })

  it('switches among the profiles declared by the channel', async () => {
    const user = userEvent.setup()
    const onProfileChange = vi.fn()
    render(<OmnichannelPreview definition={definition} preview={preview} onProfileChange={onProfileChange} />)
    await user.selectOptions(screen.getByLabelText(/Client preview/i), 'whatsapp_web')
    expect(onProfileChange).toHaveBeenCalledWith('whatsapp_web')
  })

  it('renders RTL content without mirroring the profile controls', () => {
    render(<OmnichannelPreview definition={definition} preview={{ ...preview, direction: 'rtl', message: { family: 'text', body: 'خوش آمدید' } }} onProfileChange={vi.fn()} />)
    expect(screen.getByText('خوش آمدید')).toHaveAttribute('dir', 'rtl')
    expect(screen.getByLabelText(/Client preview/i)).not.toHaveAttribute('dir', 'rtl')
  })

  it('shows canonical request content for the Webhook profile', () => {
    const webhookDefinition: ChannelDefinition = {
      id: 'webhook', label_key: 'Webhook', content_families: ['webhook_payload'],
      preview_profiles: [{ id: 'http_request', label_key: 'HTTP request', surface: 'developer' }],
      delivery_modes: ['signed_webhook'], limits: {}
    }
    const webhookPreview: GenericChannelPreview = {
      profile: 'http_request', direction: 'ltr', payload_bytes: 90,
      message: { family: 'webhook_payload', webhook: { content_type: 'application/json', body: '{"customer":"42"}' } },
      warnings: []
    }
    render(<OmnichannelPreview definition={webhookDefinition} preview={webhookPreview} onProfileChange={vi.fn()} />)
    expect(screen.getByText('{"customer":"42"}')).toBeVisible()
    expect(screen.getByText(/X-Yaoguang-Signature/i)).toBeVisible()
  })
})

