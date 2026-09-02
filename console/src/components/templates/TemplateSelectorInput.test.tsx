import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { focusManager, QueryClient, QueryClientProvider } from '@tanstack/react-query'
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

const makeTemplate = (
  id: string,
  name: string,
  channel: Template['channel'] = 'email',
  category = 'marketing',
  categoryPurpose?: Template['category_purpose']
): Template =>
  ({ id, name, category, category_purpose: categoryPurpose, channel }) as unknown as Template

const queryClient = new QueryClient({
  defaultOptions: { queries: { refetchOnWindowFocus: false, retry: false } }
})

const Wrapper = ({
  value,
  channel = 'email',
  category,
  purpose,
  onChange = vi.fn()
}: {
  value: string | null
  channel?: Template['channel']
  category?: string
  purpose?: Template['category_purpose']
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
            category={category}
            purpose={purpose}
          />
        </App>
      </ConfigProvider>
    </I18nProvider>
  </QueryClientProvider>
)

describe('TemplateSelectorInput', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    queryClient.clear()
    focusManager.setFocused(undefined)
    ;(templatesApi.list as ReturnType<typeof vi.fn>).mockResolvedValue({
      templates: []
    })
    ;(templatesApi.get as ReturnType<typeof vi.fn>).mockImplementation(({ id }: { id: string }) =>
      Promise.resolve({
        template: makeTemplate(id, id === 'tpl-a' ? 'Template A' : 'Template B')
      })
    )
  })

  it('opens the corresponding material creation page for the selected node channel', async () => {
    const open = vi.spyOn(window, 'open').mockImplementation(() => null)
    render(<Wrapper value={null} channel="sms" />)
    const input = screen.getByPlaceholderText('Select a template')
    fireEvent.click(input)
    await waitFor(() => {
      expect(templatesApi.list).toHaveBeenCalledWith(expect.objectContaining({ channel: 'sms' }))
    })
    fireEvent.click(screen.getByRole('button', { name: /Create new SMS template/i }))
    expect(open).toHaveBeenCalledWith('/console/workspace/ws1/templates?create_channel=sms', '_blank', 'noopener,noreferrer')
  })

  it('shows templates from custom categories with the requested purpose', async () => {
    ;(templatesApi.list as ReturnType<typeof vi.fn>).mockImplementation(
      ({ category }: { category?: string }) =>
        Promise.resolve({
          templates: category
            ? []
            : [
                makeTemplate('welcome-hardware', 'Welcome Hardware', 'email', 'hardware', 'marketing'),
                makeTemplate('password-reset', 'Password Reset', 'email', 'account', 'transactional')
              ]
        })
    )

    render(<Wrapper value={null} purpose="marketing" />)
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))
    fireEvent.click(screen.getByPlaceholderText('Select a template'))

    expect(await screen.findByText('Welcome Hardware')).toBeInTheDocument()
    expect(screen.queryByText('Password Reset')).not.toBeInTheDocument()
  })

  it('keeps exact category filters for category-specific template pickers', async () => {
    ;(templatesApi.list as ReturnType<typeof vi.fn>).mockImplementation(
      ({ category }: { category?: string }) =>
        Promise.resolve({
          templates:
            category === 'opt_in'
              ? [makeTemplate('confirm-subscription', 'Confirm Subscription', 'email', 'opt_in')]
              : []
        })
    )

    render(<Wrapper value={null} category="opt_in" />)
    fireEvent.click(screen.getByPlaceholderText('Select a template'))

    expect(await screen.findByText('Confirm Subscription')).toBeInTheDocument()
  })

  it('refreshes an open template picker after returning from template creation', async () => {
    ;(templatesApi.list as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({ templates: [] })
      .mockResolvedValueOnce({ templates: [] })
      .mockResolvedValue({
        templates: [
          makeTemplate('welcome-hardware', 'Welcome Hardware', 'email', 'hardware', 'marketing')
        ]
      })

    render(<Wrapper value={null} purpose="marketing" />)
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))
    fireEvent.click(screen.getByPlaceholderText('Select a template'))
    expect(await screen.findByText(/No templates found/)).toBeInTheDocument()
    await waitFor(() => expect(templatesApi.list).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))

    focusManager.setFocused(false)
    focusManager.setFocused(true)

    await waitFor(() => expect(templatesApi.list).toHaveBeenCalledTimes(3))
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))
    expect(await screen.findByText('Welcome Hardware')).toBeInTheDocument()
  })

  it('refreshes the template list when the picker is reopened', async () => {
    ;(templatesApi.list as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({ templates: [] })
      .mockResolvedValueOnce({ templates: [] })
      .mockResolvedValue({
        templates: [
          makeTemplate('welcome-hardware', 'Welcome Hardware', 'email', 'hardware', 'marketing')
        ]
      })

    render(<Wrapper value={null} purpose="marketing" />)
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))
    const selector = screen.getByPlaceholderText('Select a template')
    fireEvent.click(selector)
    expect(await screen.findByText(/No templates found/)).toBeInTheDocument()
    await waitFor(() => expect(templatesApi.list).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))

    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    fireEvent.click(selector)

    await waitFor(() => expect(templatesApi.list).toHaveBeenCalledTimes(3))
    expect(await screen.findByText('Welcome Hardware')).toBeInTheDocument()
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
