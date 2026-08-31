import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { ChannelDefinition } from '../../services/api/channels'
import ChannelPicker from './ChannelPicker'

const definitions: ChannelDefinition[] = [
  {
    id: 'line', label_key: 'LINE', regions: ['southeast_asia'], recommended_in: ['TH'],
    content_families: ['text', 'rich_card'], preview_profiles: [{ id: 'line_mobile', label_key: 'LINE mobile', surface: 'mobile' }],
    delivery_modes: ['signed_webhook'], limits: {}
  },
  {
    id: 'zalo', label_key: 'Zalo', regions: ['southeast_asia'], recommended_in: ['VN'],
    content_families: ['text'], preview_profiles: [{ id: 'zalo_mobile', label_key: 'Zalo mobile', surface: 'mobile' }],
    delivery_modes: ['signed_webhook'], limits: {}
  },
  {
    id: 'telegram', label_key: 'Telegram', regions: ['global'], recommended_in: ['KZ'],
    content_families: ['text'], preview_profiles: [{ id: 'telegram_mobile', label_key: 'Telegram mobile', surface: 'mobile' }],
    delivery_modes: ['signed_webhook'], limits: {}
  }
]

describe('ChannelPicker', () => {
  it('shows the selected country recommendations before the full catalogue', async () => {
    const user = userEvent.setup()
    render(<ChannelPicker definitions={definitions} country="VN" onSelect={vi.fn()} />)

    expect(screen.getByRole('button', { name: /Zalo/ })).toBeVisible()
    expect(screen.queryByRole('button', { name: /Telegram/ })).not.toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: /All channels/i }))
    expect(screen.getByRole('button', { name: /Telegram/ })).toBeVisible()
  })

  it('selects a channel and identifies signed Webhook delivery', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    render(<ChannelPicker definitions={definitions} country="TH" onSelect={onSelect} />)

    const line = screen.getByRole('button', { name: /LINE/ })
    expect(line).toHaveTextContent(/Signed Webhook/i)
    await user.click(line)
    expect(onSelect).toHaveBeenCalledWith('line')
  })

  it('filters the full catalogue and exposes an explicit empty state', async () => {
    const user = userEvent.setup()
    render(<ChannelPicker definitions={definitions} onSelect={vi.fn()} />)
    await user.click(screen.getByRole('tab', { name: /All channels/i }))
    await user.type(screen.getByPlaceholderText(/Search channels/i), 'missing')
    expect(screen.getByText(/No channels match/i)).toBeVisible()
  })
})
