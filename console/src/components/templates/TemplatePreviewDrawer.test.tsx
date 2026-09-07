import { describe, expect, it, vi } from 'vitest'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import TemplatePreviewDrawer from './TemplatePreviewDrawer'
import { templatesApi } from '../../services/api/template'
import type { Template, Workspace } from '../../services/api/types'
import type { EmailBlock } from '../email_builder/types'

vi.mock('./EmailClientFrame', () => ({
  default: ({ html }: { html: string }) => <div data-testid="email-preview">{html}</div>
}))

describe('TemplatePreviewDrawer', () => {
  const workspace = {
    id: 'workspace-1', name: 'Workspace',
    settings: { timezone: 'UTC', email_tracking_enabled: false, default_language: 'en', languages: ['en'] },
    created_at: '', updated_at: ''
  } satisfies Workspace

  it('previews saved Web HTML without sending it to the email compiler', async () => {
    const compile = vi.spyOn(templatesApi, 'compile')
    render(<TemplatePreviewDrawer record={{ id: 'web', name: 'Landing page', channel: 'web', version: 1, category: 'marketing', web: { html: '<h1>Web offer</h1>' }, created_at: '', updated_at: '' }} workspace={workspace}>
      <button>Open Web preview</button>
    </TemplatePreviewDrawer>)
    await userEvent.click(screen.getByRole('button', { name: 'Open Web preview' }))
    expect(await screen.findByTestId('email-preview')).toHaveTextContent('<h1>Web offer</h1>')
    expect(compile).not.toHaveBeenCalled()
  })

  it('ignores an in-flight platform response after closing and reopening the preview', async () => {
    let resolveOld!: (value: Awaited<ReturnType<typeof templatesApi.preview>>) => void
    const oldPreview = new Promise<Awaited<ReturnType<typeof templatesApi.preview>>>((resolve) => { resolveOld = resolve })
    const response = (title: string) => ({ channel: 'push' as const, resolved_language: 'en', fallback_used: false, push: { title, body: 'Body', platform: 'android' as const, payload_bytes: 90, warnings: [] } })
    const preview = vi.spyOn(templatesApi, 'preview').mockResolvedValueOnce(response('Initial Android')).mockImplementationOnce(() => oldPreview).mockResolvedValueOnce(response('Current preview'))
    render(<TemplatePreviewDrawer record={{ id: 'push', name: 'Push', channel: 'push', version: 1, category: 'marketing', push: { title: 'Draft', body: 'Body' }, created_at: '', updated_at: '' }} workspace={workspace}>
      <button>Open push preview</button>
    </TemplatePreviewDrawer>)
    await userEvent.click(screen.getByRole('button', { name: 'Open push preview' }))
    expect(await screen.findByText('Initial Android')).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: 'iOS' }))
    await waitFor(() => expect(preview).toHaveBeenCalledTimes(2))
    // Closing while the request is in flight must invalidate it; reopening renders a new request.
    await userEvent.click(screen.getByRole('button', { name: 'Close' }))
    await userEvent.click(screen.getByRole('button', { name: 'Open push preview' }))
    expect(await screen.findByText('Current preview')).toBeVisible()
    await act(async () => resolveOld(response('Stale response')))
    expect(screen.queryByText('Stale response')).not.toBeInTheDocument()
    expect(screen.getByText('Current preview')).toBeVisible()
  })

  it('compiles a visual email template without requiring generic channel content', async () => {
    const visualEditorTree = {
      id: 'root',
      type: 'mjml',
      children: []
    } as unknown as EmailBlock
    const record = {
      id: 'welcome-template',
      name: 'Welcome template',
      version: 1,
      channel: 'email',
      category: 'welcome',
      email: {
        editor_mode: 'visual',
        subject: 'Welcome',
        compiled_preview: '<mjml />',
        visual_editor_tree: visualEditorTree
      },
      created_at: '2026-09-01T00:00:00Z',
      updated_at: '2026-09-01T00:00:00Z'
    } satisfies Template
    const workspace = {
      id: 'workspace-1',
      name: 'Workspace',
      settings: {
        timezone: 'UTC',
        email_tracking_enabled: false,
        default_language: 'en',
        languages: ['en']
      },
      created_at: '2026-09-01T00:00:00Z',
      updated_at: '2026-09-01T00:00:00Z'
    } satisfies Workspace
    const compile = vi.spyOn(templatesApi, 'compile').mockResolvedValue({
      mjml: '<mjml />',
      html: '<p>Welcome</p>'
    })

    render(
      <TemplatePreviewDrawer record={record} workspace={workspace}>
        <button type="button">Open preview</button>
      </TemplatePreviewDrawer>
    )

    await userEvent.click(screen.getByRole('button', { name: 'Open preview' }))

    await waitFor(() => expect(compile).toHaveBeenCalledTimes(1))
    expect(compile).toHaveBeenCalledWith(
      expect.objectContaining({
        workspace_id: 'workspace-1',
        visual_editor_tree: visualEditorTree
      })
    )
    expect(screen.queryByText('Missing workspace ID or template data.')).not.toBeInTheDocument()
  })
})
