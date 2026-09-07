import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { ChannelDefinition } from '../../services/api/channels'
import type { Workspace } from '../../services/api/types'
import { CreateTemplateButton, TemplateEditorButton } from './UnifiedTemplateActions'
import { useState } from 'react'

vi.mock('./CreateTemplateDrawer', () => ({ CreateTemplateDrawer: () => <button>Email editor</button> }))
vi.mock('./MessageTemplateDrawer', () => ({ default: () => <button>SMS Push editor</button> }))
vi.mock('./WebTemplateDrawer', () => ({ default: () => <button>Web editor</button> }))
vi.mock('./OmnichannelTemplateDrawer', () => ({ default: function MockOmnichannelDrawer({ defaultChannel }: { defaultChannel?: string }) {
  const [initialChannel] = useState(defaultChannel)
  return <button>Omnichannel editor {initialChannel}</button>
} }))

const workspace = { id: 'ws1', settings: { default_language: 'en', languages: ['en'] } } as unknown as Workspace
const definitions: ChannelDefinition[] = [
  { id: 'email', label_key: 'Email', content_families: ['rich_card'], preview_profiles: [], delivery_modes: ['native'], limits: {} },
  { id: 'web', label_key: 'Web', content_families: ['rich_card'], preview_profiles: [{ id: 'web_page', label_key: 'Web page', surface: 'web' }], delivery_modes: ['native'], limits: {} },
  { id: 'line', label_key: 'LINE', content_families: ['text'], preview_profiles: [{ id: 'line_mobile', label_key: 'LINE mobile', surface: 'mobile' }], delivery_modes: ['signed_webhook'], limits: {} },
  { id: 'telegram', label_key: 'Telegram', content_families: ['text'], preview_profiles: [{ id: 'telegram_mobile', label_key: 'Telegram mobile', surface: 'mobile' }], delivery_modes: ['signed_webhook'], limits: {} }
]

describe('UnifiedTemplateActions', () => {
  it('uses the native Web editor and resets generic editor state when choosing another channel', async () => {
    const user = userEvent.setup()
    render(<CreateTemplateButton workspace={workspace} definitions={definitions} />)
    await user.click(screen.getByRole('button', { name: /Create template/i }))
    await user.click(screen.getByRole('tab', { name: /All channels/i }))
    await user.click(screen.getByRole('button', { name: /^Web / }))
    expect(screen.getByRole('button', { name: 'Web editor' })).toBeVisible()
    expect(screen.queryByRole('button', { name: /Omnichannel editor/i })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /^Telegram / }))
    expect(screen.getByRole('button', { name: 'Omnichannel editor telegram' })).toBeVisible()
    await user.click(screen.getByRole('button', { name: /^LINE / }))
    expect(screen.getByRole('button', { name: 'Omnichannel editor line' })).toBeVisible()
  })

  it('routes saved Web templates to the native editor', () => {
    render(<TemplateEditorButton workspace={workspace} definitions={definitions} template={{ id: 'web', name: 'Web', version: 1, channel: 'web', category: 'marketing', web: { content: { type: 'doc' }, html: '<p>Page</p>' }, created_at: '', updated_at: '' }} />)
    expect(screen.getByRole('button', { name: 'Web editor' })).toBeVisible()
  })
  it('starts from one create action and routes a generic channel to its editor', async () => {
    const user = userEvent.setup()
    render(<CreateTemplateButton workspace={workspace} definitions={definitions} />)
    expect(screen.getAllByRole('button')).toHaveLength(1)
    await user.click(screen.getByRole('button', { name: /Create template/i }))
    await user.click(screen.getByRole('tab', { name: /All channels/i }))
    await user.click(screen.getByRole('button', { name: /Telegram/ }))
    expect(screen.getByRole('button', { name: /Omnichannel editor/i })).toBeVisible()
    expect(screen.queryByRole('button', { name: /SMS Push editor/i })).not.toBeInTheDocument()
  })

  it('routes an existing generic template to the omnichannel editor', () => {
    render(<TemplateEditorButton workspace={workspace} definitions={definitions} template={{ id: 't1', name: 'T', version: 1, channel: 'telegram', category: 'marketing', content_schema_version: 1, content: { family: 'text', body: 'Hi' }, created_at: '', updated_at: '' }} />)
    expect(screen.getByRole('button', { name: /Omnichannel editor/i })).toBeVisible()
  })
})
