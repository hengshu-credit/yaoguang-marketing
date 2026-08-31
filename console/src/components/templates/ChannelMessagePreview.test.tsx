import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import userEvent from '@testing-library/user-event'
import ChannelMessagePreview from './ChannelMessagePreview'

vi.mock('@lingui/react/macro', () => ({
  useLingui: () => ({ t: (strings: TemplateStringsArray) => strings[0] })
}))

describe('ChannelMessagePreview', () => {
  it('renders SMS body and segmentation diagnostics', () => {
    render(
      <ChannelMessagePreview
        preview={{
          channel: 'sms',
          resolved_language: 'en',
          fallback_used: false,
          sms: {
            body: 'Hello Alice',
            encoding: 'gsm-7',
            character_count: 11,
            unit_count: 11,
            segment_count: 1,
            per_segment: 160,
            remaining: 149
          }
        }}
        platform="android"
        onPlatformChange={vi.fn()}
      />
    )

    expect(screen.getByText('Hello Alice')).toBeInTheDocument()
    expect(screen.getByText(/GSM-7/i)).toBeInTheDocument()
    expect(screen.getByText(/1 segment/i)).toBeInTheDocument()
  })

  it('renders push payload and warnings', () => {
    render(
      <ChannelMessagePreview
        preview={{
          channel: 'push',
          resolved_language: 'en',
          fallback_used: false,
          push: {
            title: 'Order shipped',
            body: 'Your order is on the way',
            platform: 'ios',
            payload_bytes: 120,
            warnings: [{ code: 'title_may_truncate', message: 'iOS may truncate this title' }]
          }
        }}
        platform="ios"
        onPlatformChange={vi.fn()}
      />
    )

    expect(screen.getByText('Order shipped')).toBeInTheDocument()
    expect(screen.getByText('Your order is on the way')).toBeInTheDocument()
    expect(screen.getByText('iOS may truncate this title')).toBeInTheDocument()
  })

  it('switches among domestic Android OEM client simulations', async () => {
    const user = userEvent.setup()
    const onPlatformChange = vi.fn()
    render(
      <ChannelMessagePreview
        preview={{ channel: 'push', resolved_language: 'zh-CN', fallback_used: false, push: { title: '优惠', body: '今日可用', platform: 'android', payload_bytes: 80, warnings: [] } }}
        platform="android"
        onPlatformChange={onPlatformChange}
      />
    )
    await user.click(screen.getByText('Huawei'))
    expect(onPlatformChange).toHaveBeenCalledWith('huawei')
  })
})
