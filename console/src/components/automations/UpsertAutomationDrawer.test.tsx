import { App } from 'antd'
import { I18nProvider } from '@lingui/react'
import { i18n } from '@lingui/core'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { UpsertAutomationDrawer } from './UpsertAutomationDrawer'

vi.mock('./context', () => ({
  AutomationProvider: ({ children }: { children: React.ReactNode }) => children,
  useAutomation: () => ({
    isEditing: false,
    name: '',
    setName: vi.fn(),
    listId: undefined,
    setListId: vi.fn(),
    exitOnReply: false,
    setExitOnReply: vi.fn(),
    lists: [],
    workspace: { id: 'ws1', integrations: [] },
    hasUnsavedChanges: false,
    isSaving: false,
    save: vi.fn(),
    validate: vi.fn().mockReturnValue([]),
    canUndo: false,
    canRedo: false,
    undo: vi.fn(),
    redo: vi.fn()
  })
}))

vi.mock('./AutomationFlowEditor', () => ({
  AutomationFlowEditor: () => <div>Flow editor</div>
}))

vi.mock('./JourneyCreateWizard', () => ({
  JourneyCreateWizard: ({
    automationId,
    onActivated
  }: {
    automationId: string
    onActivated: () => void
  }) => (
    <div>
      <output aria-label="Draft automation ID">{automationId}</output>
      <button type="button" onClick={onActivated}>
        Complete activation
      </button>
    </div>
  )
}))

const workspace = { id: 'ws1', name: 'Workspace', integrations: [] }

function renderDrawer() {
  return render(
    <I18nProvider i18n={i18n}>
      <App>
        <UpsertAutomationDrawer
          workspace={workspace as never}
          buttonContent="Create journey"
        />
      </App>
    </I18nProvider>
  )
}

describe('UpsertAutomationDrawer journey draft identity', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('keeps the draft ID after closing and rotates it only after activation completes', async () => {
    const user = userEvent.setup()
    const firstRender = renderDrawer()

    await user.click(screen.getByRole('button', { name: 'Create journey' }))
    const firstId = screen.getByRole('status', { name: 'Draft automation ID' }).textContent

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    firstRender.unmount()
    renderDrawer()
    await user.click(screen.getByRole('button', { name: 'Create journey' }))
    expect(screen.getByRole('status', { name: 'Draft automation ID' })).toHaveTextContent(firstId!)

    await user.click(screen.getByRole('button', { name: 'Complete activation' }))
    await user.click(screen.getByRole('button', { name: 'Create journey' }))
    expect(screen.getByRole('status', { name: 'Draft automation ID' }).textContent).not.toBe(firstId)
  })
})
