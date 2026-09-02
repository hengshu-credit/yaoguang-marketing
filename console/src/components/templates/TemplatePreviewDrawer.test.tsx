import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import TemplatePreviewDrawer from './TemplatePreviewDrawer'
import { templatesApi } from '../../services/api/template'
import type { Template, Workspace } from '../../services/api/types'
import type { EmailBlock } from '../email_builder/types'

vi.mock('./EmailClientFrame', () => ({
  default: ({ html }: { html: string }) => <div data-testid="email-preview">{html}</div>
}))

describe('TemplatePreviewDrawer', () => {
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
