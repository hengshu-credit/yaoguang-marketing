import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { CustomFieldsConfiguration } from './CustomFieldsConfiguration'
import type { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'

// Mock the workspace API service. The component must call setCustomFieldLabels
// (the dedicated, permission-checked endpoint) and NOT update (owner-only).
vi.mock('../../services/api/workspace', () => ({
  workspaceService: {
    setCustomFieldLabels: vi.fn().mockResolvedValue({ status: 'success' }),
    update: vi.fn(),
    get: vi.fn().mockResolvedValue({ workspace: { id: 'ws1', settings: { custom_field_labels: {} } } })
  }
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const makeWorkspace = (labels: Record<string, string> = {}): Workspace =>
  ({
    id: 'ws1',
    name: 'WS',
    settings: { custom_field_labels: labels }
  }) as unknown as Workspace

const renderComponent = (canManage: boolean, labels: Record<string, string> = {}) =>
  render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <CustomFieldsConfiguration
            workspace={makeWorkspace(labels)}
            onWorkspaceUpdate={vi.fn()}
            canManage={canManage}
          />
        </App>
      </ConfigProvider>
    </I18nProvider>
  )

describe('CustomFieldsConfiguration', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('disables the Add Label button when the user cannot manage', () => {
    renderComponent(false)
    expect(screen.getByRole('button', { name: /Add Label/i })).toBeDisabled()
  })

  it('enables the Add Label button when the user can manage', () => {
    renderComponent(true)
    expect(screen.getByRole('button', { name: /Add Label/i })).toBeEnabled()
  })

  it('renders existing labels with disabled edit/delete controls when the user cannot manage', () => {
    renderComponent(false, { custom_string_1: 'Company Name' })
    expect(screen.getByText('Company Name')).toBeInTheDocument()
    // Add Label + Edit + Delete should all be disabled.
    const disabled = screen.getAllByRole('button').filter((b) => b.hasAttribute('disabled'))
    expect(disabled.length).toBeGreaterThanOrEqual(3)
  })

  it('enables edit/delete controls when the user can manage', () => {
    renderComponent(true, { custom_string_1: 'Company Name' })
    expect(screen.getByText('Company Name')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Add Label/i })).toBeEnabled()
  })

  it('saves a new label via setCustomFieldLabels (not workspace.update)', async () => {
    renderComponent(true)

    fireEvent.click(screen.getByRole('button', { name: /Add Label/i }))

    // Select a field and enter a label in the modal.
    fireEvent.click(screen.getByText('custom_string_1'))
    fireEvent.change(screen.getByPlaceholderText(/e\.g\., Company Name/i), {
      target: { value: 'My Company' }
    })
    fireEvent.click(screen.getByRole('button', { name: /^Save$/i }))

    await waitFor(() => {
      expect(workspaceService.setCustomFieldLabels).toHaveBeenCalledWith({
        workspace_id: 'ws1',
        custom_field_labels: { custom_string_1: 'My Company' }
      })
    })
    expect(workspaceService.update).not.toHaveBeenCalled()
  })
})
