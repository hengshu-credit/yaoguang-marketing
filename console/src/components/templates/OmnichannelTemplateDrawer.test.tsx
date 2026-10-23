import { App as AntApp } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
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

const renderDrawer = (overrides: Partial<React.ComponentProps<typeof OmnichannelTemplateDrawer>> = {}) => render(
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <AntApp><OmnichannelTemplateDrawer workspace={workspace} definitions={definitions} defaultChannel="telegram" livePreviewDelay={false} {...overrides} /></AntApp>
  </QueryClientProvider>
)

describe('OmnichannelTemplateDrawer', () => {
  it('switches empty drafts between device buttons without sending invalid preview requests', async () => {
    vi.mocked(templatesApi.preview).mockRejectedValue(new Error('body is required'))
    const { container } = renderDrawer({ livePreviewDelay: 0 })
    await userEvent.click(screen.getByRole('button', { name: 'Create template' }))
    for (const [label, device] of [['Computer', 'desktop'], ['Pad', 'tablet'], ['Android', 'android'], ['iOS', 'ios']]) {
      await userEvent.click(screen.getByRole('button', { name: label }))
      expect(container.ownerDocument.querySelector('[data-preview-device]')).toHaveAttribute('data-preview-device', device)
      expect(screen.queryByText('body is required')).not.toBeInTheDocument()
    }
    expect(templatesApi.preview).not.toHaveBeenCalled()
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Untitled draft' } })
    await act(async () => { await new Promise(resolve => setTimeout(resolve, 30)) })
    expect(templatesApi.preview).not.toHaveBeenCalled()
  })
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(templatesApi.preview).mockResolvedValue({
      channel: 'telegram', resolved_language: 'en', fallback_used: false,
      channel_preview: { profile: 'telegram_mobile', direction: 'ltr', payload_bytes: 24, message: { family: 'text', body: 'Hello Ada' }, warnings: [] }
    })
    vi.mocked(templatesApi.create).mockResolvedValue({ template: {} as never })
  })

  it('automatically renders an existing draft with its saved test data when opened', async () => {
    renderDrawer({ livePreviewDelay: 0, template: { id: 'existing', name: 'Existing', version: 1, channel: 'telegram', category: 'marketing', content: { family: 'text', body: 'Hello {{ customer.name }}' }, test_data: { customer: { name: 'Ada' } }, created_at: '', updated_at: '' } })
    await userEvent.click(screen.getByRole('button', { name: 'Edit Template' }))
    expect(await screen.findByText('Hello Ada', { selector: 'p' })).toBeVisible()
    expect(templatesApi.preview).toHaveBeenCalledWith(expect.objectContaining({ profile: 'telegram_mobile', test_data: { customer: { name: 'Ada' } } }))
  })

  it('initializes an existing template from its own channel even when Email is first in the catalogue', async () => {
    const saved = { id: 'whatsapp', name: 'Offer', version: 1, channel: 'whatsapp' as const, category: 'marketing', content: { family: 'rich_card' as const, body: 'Offer', actions: [{ type: 'url' as const, label: 'Open', value: 'https://example.com' }] }, created_at: '', updated_at: '' }
    renderDrawer({ defaultChannel: undefined, template: saved, definitions: [
      { id: 'email', label_key: 'Email', content_families: ['rich_card'], preview_profiles: [{ id: 'email_mobile', label_key: 'Email mobile', surface: 'mobile' }], delivery_modes: ['native'], limits: {} },
      ...definitions,
      { id: 'whatsapp', label_key: 'WhatsApp', content_families: ['rich_card'], preview_profiles: [{ id: 'whatsapp_ios', label_key: 'WhatsApp iOS', surface: 'mobile' }], delivery_modes: ['signed_webhook'], limits: {} }
    ] })
    await userEvent.click(screen.getByRole('button', { name: 'Edit Template' }))
    expect(screen.getByRole('button', { name: 'iOS' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.queryByText('This preview profile is not supported.')).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Open' })).toHaveAttribute('href', 'https://example.com')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(templatesApi.update).toHaveBeenCalledWith(expect.objectContaining({ content: saved.content })))
  })

  it('previews content without requiring save-only metadata and ignores an old failure after a new edit', async () => {
    let rejectOld!: (error: Error) => void
    vi.mocked(templatesApi.preview).mockImplementationOnce(() => new Promise((_, reject) => { rejectOld = reject }))
    renderDrawer()
    await userEvent.click(screen.getByRole('button', { name: 'Create template' }))
    fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'Original draft' } })
    await userEvent.click(screen.getByRole('button', { name: 'Preview' }))
    await waitFor(() => expect(templatesApi.preview).toHaveBeenCalledTimes(1))
    fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'Current draft' } })
    await act(async () => rejectOld(new Error('Stale failure')))
    expect(screen.queryByText('Stale failure')).not.toBeInTheDocument()
    expect(screen.getByText('Current draft', { selector: 'p' })).toBeVisible()
    expect(screen.queryByText("'id' is required")).not.toBeInTheDocument()
  })

  it('does not let an old success replace current draft content during the debounce period', async () => {
    let finishOld!: (value: Awaited<ReturnType<typeof templatesApi.preview>>) => void
    vi.mocked(templatesApi.preview).mockImplementationOnce(() => new Promise(resolve => { finishOld = resolve }))
    renderDrawer()
    await userEvent.click(screen.getByRole('button', { name: 'Create template' }))
    fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'Old draft' } })
    await userEvent.click(screen.getByRole('button', { name: 'Preview' }))
    await waitFor(() => expect(templatesApi.preview).toHaveBeenCalledTimes(1))
    fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'New draft' } })
    await act(async () => finishOld({ channel: 'telegram', resolved_language: 'en', fallback_used: false, channel_preview: { profile: 'telegram_mobile', direction: 'ltr', message: { family: 'text', body: 'Outdated render' }, payload_bytes: 20, warnings: [] } }))
    expect(screen.getByText('New draft', { selector: 'p' })).toBeVisible()
    expect(screen.queryByText('Outdated render')).not.toBeInTheDocument()
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
