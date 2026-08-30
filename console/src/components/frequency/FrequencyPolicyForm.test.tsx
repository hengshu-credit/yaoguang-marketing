import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { i18n } from '@lingui/core'
import { FrequencyPolicyForm } from './FrequencyPolicyForm'

describe('FrequencyPolicyForm', () => {
  beforeEach(() => {
    i18n.load('en', {})
    i18n.activate('en')
  })

  it('keeps campaign, trigger, and workspace caps as separate business cards', () => {
    const onSave = vi.fn()
    const { rerender } = render(<FrequencyPolicyForm scope="campaign" onSave={onSave} />)
    expect(screen.getByText('Campaign limit')).toBeInTheDocument()
    rerender(<FrequencyPolicyForm scope="trigger" onSave={onSave} />)
    expect(screen.getByText('Event / scheduled trigger limit')).toBeInTheDocument()
    expect(screen.getByText('Entry frequency and message frequency control are separate rules')).toBeInTheDocument()
    rerender(<FrequencyPolicyForm scope="workspace_global" onSave={onSave} />)
    expect(screen.getByText('Workspace-wide limit')).toBeInTheDocument()
  })

  it('locks the technical trigger reference when used inside a journey wizard', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn()
    render(
      <FrequencyPolicyForm
        scope="trigger"
        defaultScopeRef="automation-1"
        fixedScopeRef="automation-1"
        onSave={onSave}
      />
    )

    expect(screen.queryByLabelText('Automation:trigger identifier')).not.toBeInTheDocument()
    await user.click(screen.getByRole('switch', { name: 'Toggle Event / scheduled trigger limit' }))
    await user.click(screen.getByRole('button', { name: 'Save this policy' }))

    await waitFor(() => {
      expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ scope_ref: 'automation-1' }))
    })
  })
})
