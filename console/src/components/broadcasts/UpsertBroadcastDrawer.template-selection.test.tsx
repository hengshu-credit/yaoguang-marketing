import React from 'react'
import { describe, beforeEach, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App, ConfigProvider } from 'antd'
import { I18nProvider } from '@lingui/react'
import { i18n } from '@lingui/core'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { UpsertBroadcastDrawer } from './UpsertBroadcastDrawer'
import { templatesApi } from '../../services/api/template'
import type { Broadcast } from '../../services/api/broadcast'
import type { List } from '../../services/api/list'
import type { Template, Workspace } from '../../services/api/types'

vi.mock('../../services/api/template', () => ({
  templatesApi: {
    list: vi.fn(),
    get: vi.fn()
  }
}))

vi.mock('../../services/api/marketing', () => ({
  audienceApi: {
    list: vi.fn().mockResolvedValue({ items: [], total: 0 })
  }
}))

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({
    workspaces: [{ id: 'ws1', name: 'Workspace', integrations: [] }]
  })
}))

vi.mock('../templates/TemplatePreviewDrawer', () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>
}))

const makeTemplate = (
  id: string,
  name: string,
  channel: Template['channel'],
  category: string,
  categoryPurpose: Template['category_purpose']
): Template =>
  ({
    id,
    name,
    version: 1,
    channel,
    category,
    category_purpose: categoryPurpose,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z'
  }) as Template

const workspace = {
  id: 'ws1',
  settings: {
    website_url: 'https://example.com',
    email_tracking_enabled: true
  }
} as unknown as Workspace

const lists = [{ id: 'list1', name: 'Newsletter' }] as unknown as List[]

const broadcast = {
  id: 'broadcast1',
  workspace_id: 'ws1',
  name: 'All templates campaign',
  channel_type: 'email',
  status: 'draft',
  audience: { list: 'list1', exclude_unsubscribed: true },
  schedule: { is_scheduled: false, use_recipient_timezone: false },
  test_settings: {
    enabled: false,
    sample_percentage: 50,
    auto_send_winner: false,
    variations: [{ variation_name: 'default', template_id: 'marketing-template' }]
  },
  data_feed: {
    global_feed: { enabled: false, url: '', headers: [] },
    recipient_feed: { enabled: false, url: '', headers: [] }
  },
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z'
} as unknown as Broadcast

describe('UpsertBroadcastDrawer template selection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    const marketingTemplate = makeTemplate(
      'marketing-template',
      'Marketing Template',
      'email',
      'marketing',
      'marketing'
    )
    ;(templatesApi.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      template: marketingTemplate
    })
    ;(templatesApi.list as ReturnType<typeof vi.fn>).mockResolvedValue({
      templates: [
        marketingTemplate,
        makeTemplate(
          'transactional-template',
          'Transactional Template',
          'email',
          'transactional',
          'transactional'
        ),
        makeTemplate(
          'welcome-hardware',
          'Welcome Hardware',
          'email',
          'welcome',
          'transactional'
        ),
        makeTemplate('sms-template', 'SMS Template', 'sms', 'marketing', 'marketing')
      ]
    })
  })

  it('shows every email material template regardless of category purpose', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } }
    })

    render(
      <QueryClientProvider client={queryClient}>
        <I18nProvider i18n={i18n}>
          <ConfigProvider>
            <App>
              <UpsertBroadcastDrawer
                workspace={workspace}
                broadcast={broadcast}
                lists={lists}
              />
            </App>
          </ConfigProvider>
        </I18nProvider>
      </QueryClientProvider>
    )

    await userEvent.click(screen.getByRole('button', { name: /Edit Broadcast/i }))
    await userEvent.click(screen.getByRole('tab', { name: '4. Content' }))
    await userEvent.click(await screen.findByDisplayValue('Marketing Template'))

    expect(await screen.findByText('Transactional Template')).toBeInTheDocument()
    expect(screen.getByText('Welcome Hardware')).toBeInTheDocument()
    expect(screen.queryByText('SMS Template')).not.toBeInTheDocument()
  })
})
