import { App as AntApp } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Workspace } from '../../services/api/types'
import { templatesApi } from '../../services/api/template'
import MessageTemplateDrawer from './MessageTemplateDrawer'

const workspace = {
  id: 'ws1',
  settings: { default_language: 'en', languages: ['en', 'fr'] }
} as unknown as Workspace

function renderDrawer(defaultChannel?: 'sms' | 'push') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <AntApp>
        <MessageTemplateDrawer
          workspace={workspace}
          defaultChannel={defaultChannel}
          buttonContent={defaultChannel === 'push' ? 'Continue with Push' : undefined}
        />
      </AntApp>
    </QueryClientProvider>
  )
}

describe('MessageTemplateDrawer', () => {
  afterEach(() => vi.mocked(templatesApi.preview).mockRestore())

  it('previews an unsaved localized SMS draft through the server', async () => {
    const previewSpy = vi.spyOn(templatesApi, 'preview').mockResolvedValue({
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
    })

    renderDrawer()
    await userEvent.click(screen.getByRole('button', { name: /Create SMS.*Push/i }))
    fireEvent.change(screen.getByLabelText('API ID'), { target: { value: 'sms-welcome' } })
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'SMS welcome' } })
    fireEvent.change(screen.getByLabelText('Message'), {
      target: { value: 'Hello {{ contact.first_name }}' }
    })
    await userEvent.click(screen.getByRole('button', { name: 'Preview' }))

    await waitFor(() => {
      expect(previewSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          workspace_id: 'ws1',
          channel: 'sms',
          sms: expect.objectContaining({ body: 'Hello {{ contact.first_name }}' })
        })
      )
    })
    expect(await screen.findByText('Hello Alice')).toBeInTheDocument()
  })

  it('initializes the selected Push client preview and mirrors draft fields immediately', async () => {
    const user = userEvent.setup()
    const previewSpy = vi.spyOn(templatesApi, 'preview')
    renderDrawer('push')

    await user.click(screen.getByRole('button', { name: 'Continue with Push' }))

    expect(screen.getByRole('radio', { name: 'Push' })).toBeChecked()
    expect(screen.getByRole('radio', { name: 'Android' })).toBeChecked()
    await user.type(screen.getByLabelText('Title'), 'Draft push title')
    await user.type(screen.getByLabelText('Message'), 'Draft push body')

    expect(await screen.findByText('Draft push title')).toBeVisible()
    expect(await screen.findAllByText('Draft push body')).not.toHaveLength(0)
    expect(previewSpy).not.toHaveBeenCalled()
  })
})
