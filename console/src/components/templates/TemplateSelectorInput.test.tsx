import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nProvider } from '@lingui/react'
import { i18n } from '@lingui/core'
import TemplateSelectorInput from './TemplateSelectorInput'
import { templatesApi } from '../../services/api/template'
import type { Template } from '../../services/api/types'

// Mock the template API — list (for the drawer) and get (to resolve the current value)
vi.mock('../../services/api/template', () => ({
  templatesApi: {
    list: vi.fn(),
    get: vi.fn()
  }
}))

// Provide the current workspace so the component renders past its loading guard
vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({
    workspaces: [{ id: 'ws1', name: 'WS', integrations: [] }]
  })
}))

// Heavy child drawers/popovers are not under test — stub them out
vi.mock('./TemplatePreviewDrawer', () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>
}))
const createDrawerProps = vi.hoisted(() => vi.fn())
vi.mock('./CreateTemplateDrawer', () => ({
  CreateTemplateDrawer: (props: { forceChannel?: string }) => {
    createDrawerProps(props)
    return <button type="button">create-template</button>
  }
}))

const makeTemplate = (id: string, name: string, channel: Template['channel'] = 'email'): Template =>
  ({ id, name, category: 'marketing', channel }) as unknown as Template

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false } }
})

const Wrapper = ({
  value,
  channel = 'email',
  onChange = vi.fn()
}: {
  value: string | null
  channel?: Template['channel']
  onChange?: (value: string | null) => void
}) => (
  <QueryClientProvider client={queryClient}>
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <TemplateSelectorInput
            value={value}
            onChange={onChange}
            workspaceId="ws1"
            channel={channel}
          />
        </App>
      </ConfigProvider>
    </I18nProvider>
  </QueryClientProvider>
)

describe('TemplateSelectorInput', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(templatesApi.list as ReturnType<typeof vi.fn>).mockResolvedValue({
      templates: []
    })
    ;(templatesApi.get as ReturnType<typeof vi.fn>).mockImplementation(({ id }: { id: string }) =>
      Promise.resolve({
        template: makeTemplate(id, id === 'tpl-a' ? 'Template A' : 'Template B')
      })
    )
  })

  it('passes the selected node channel to list and create flows', async () => {
    render(<Wrapper value={null} channel="sms" />)
    const input = screen.getByPlaceholderText('Select a template')
    fireEvent.click(input)
    await waitFor(() => {
      expect(templatesApi.list).toHaveBeenCalledWith(expect.objectContaining({ channel: 'sms' }))
      expect(createDrawerProps).toHaveBeenCalledWith(
        expect.objectContaining({ forceChannel: 'sms' })
      )
    })
  })

  it('clears a controlled template that belongs to another channel', async () => {
    const onChange = vi.fn()
    ;(templatesApi.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      template: makeTemplate('email-template', 'Email Template', 'email')
    })
    render(<Wrapper value="email-template" channel="push" onChange={onChange} />)
    await waitFor(() => expect(onChange).toHaveBeenCalledWith(null))
    expect(screen.queryByDisplayValue('Email Template')).not.toBeInTheDocument()
  })

  // Regression for issue #353: the same instance is reused while `value` changes
  // (switching between email nodes in the automation editor). The displayed
  // template must follow `value`, not stay on the first one resolved.
  it('updates the displayed template when the value prop changes', async () => {
    const { rerender } = render(<Wrapper value="tpl-a" />)
    expect(await screen.findByDisplayValue('Template A')).toBeInTheDocument()

    rerender(<Wrapper value="tpl-b" />)
    expect(await screen.findByDisplayValue('Template B')).toBeInTheDocument()
  })

  it('clears the displayed template when the value is cleared', async () => {
    const { rerender } = render(<Wrapper value="tpl-a" />)
    expect(await screen.findByDisplayValue('Template A')).toBeInTheDocument()

    rerender(<Wrapper value={null} />)
    await waitFor(() => {
      expect(screen.queryByDisplayValue('Template A')).not.toBeInTheDocument()
    })
  })
})
