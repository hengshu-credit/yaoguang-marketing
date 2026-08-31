import { App as AntApp } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChannelDefinition } from '../../services/api/channels'
import type { Workspace } from '../../services/api/types'
import { templatesApi } from '../../services/api/template'
import OmnichannelTemplateDrawer from './OmnichannelTemplateDrawer'

const templateApiMocks = vi.hoisted(() => ({ create: vi.fn(), update: vi.fn(), preview: vi.fn() }))
vi.mock('../../services/api/template', () => ({ templatesApi: templateApiMocks }))

const definitions: ChannelDefinition[] = [{
  id: 'telegram', label_key: 'Telegram', content_families: ['text'],
  preview_profiles: [
    { id: 'telegram_mobile', label_key: 'Telegram mobile', surface: 'mobile' },
    { id: 'telegram_desktop', label_key: 'Telegram desktop', surface: 'desktop' }
  ],
  delivery_modes: ['signed_webhook'], limits: { max_body_runes: 4096 }
}]

const workspace = {
  id: 'ws1', settings: { default_language: 'en', languages: ['en', 'ur'] }, integrations: []
} as unknown as Workspace

const renderDrawer = () => render(
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <AntApp><OmnichannelTemplateDrawer workspace={workspace} definitions={definitions} defaultChannel="telegram" livePreviewDelay={false} /></AntApp>
  </QueryClientProvider>
)

describe('OmnichannelTemplateDrawer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(templatesApi.preview).mockResolvedValue({
      channel: 'telegram', resolved_language: 'en', fallback_used: false,
      channel_preview: { profile: 'telegram_mobile', direction: 'ltr', payload_bytes: 24, message: { family: 'text', body: 'Hello Ada' }, warnings: [] }
    })
    vi.mocked(templatesApi.create).mockResolvedValue({ template: {} as never })
  })

  it('shows an initialized client surface immediately and mirrors draft content before the server responds', async () => {
    const user = userEvent.setup()
    vi.mocked(templatesApi.preview).mockReturnValue(new Promise(() => undefined))
    renderDrawer()
    await user.click(screen.getByRole('button', { name: /Create template/i }))
    expect(screen.getByText(/Simulated preview/i)).toBeVisible()
    fireEvent.change(screen.getByLabelText(/^Message$/i), { target: { value: 'Draft appears now' } })
    expect(screen.getByText('Draft appears now')).toBeVisible()
  })

  it('previews unsaved typed content through the server contract', async () => {
    const user = userEvent.setup()
    renderDrawer()
    await user.click(screen.getByRole('button', { name: /Create template/i }))
    await user.type(screen.getByLabelText(/API ID/i), 'telegram-welcome')
    await user.type(screen.getByLabelText(/^Name$/i), 'Telegram welcome')
    fireEvent.change(screen.getByLabelText(/^Message$/i), { target: { value: 'Hello {{ customer.name }}' } })
    fireEvent.change(screen.getByLabelText(/Template data/i), { target: { value: '{"customer":{"name":"Ada"}}' } })
    await user.click(screen.getByRole('button', { name: /^Preview$/i }))

    await waitFor(() => expect(templatesApi.preview).toHaveBeenCalledWith(expect.objectContaining({
      workspace_id: 'ws1', channel: 'telegram', content_schema_version: 1,
      profile: 'telegram_mobile', content: { family: 'text', body: 'Hello {{ customer.name }}' }
    })))
    expect(await screen.findByText('Hello Ada')).toBeVisible()
  })

  it('saves the same structured material that was previewed', async () => {
    const user = userEvent.setup()
    renderDrawer()
    await user.click(screen.getByRole('button', { name: /Create template/i }))
    await user.type(screen.getByLabelText(/API ID/i), 'telegram-offer')
    await user.type(screen.getByLabelText(/^Name$/i), 'Telegram offer')
    fireEvent.change(screen.getByLabelText(/^Message$/i), { target: { value: 'Offer today' } })
    await user.click(screen.getByRole('button', { name: /^Save$/i }))

    await waitFor(() => expect(templatesApi.create).toHaveBeenCalledWith(expect.objectContaining({
      id: 'telegram-offer', name: 'Telegram offer', channel: 'telegram', category: 'marketing',
      content_schema_version: 1, content: { family: 'text', body: 'Offer today' }
    })))
  })
})
