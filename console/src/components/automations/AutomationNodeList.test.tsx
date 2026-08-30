import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nProvider } from '@lingui/react'
import { i18n } from '../../i18n'
import { AutomationNodeList } from './AutomationNodeList'

describe('AutomationNodeList', () => {
  it('offers a keyboard-operable alternative to selecting nodes on the canvas', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    render(
      <I18nProvider i18n={i18n}>
        <AutomationNodeList
          nodes={[
            { id: 'trigger-1', data: { label: 'Loan approved', nodeType: 'trigger' } },
            { id: 'sms-1', data: { label: 'Send reminder', nodeType: 'sms' } }
          ]}
          selectedNodeId="trigger-1"
          orphanNodeIds={new Set(['sms-1'])}
          onSelect={onSelect}
        />
      </I18nProvider>
    )

    const target = screen.getByRole('button', { name: /2\. Send reminder.*Not connected/ })
    target.focus()
    await user.keyboard('{Enter}')

    expect(onSelect).toHaveBeenCalledWith('sms-1')
    expect(screen.getByRole('button', { name: /1\. Loan approved/ })).toHaveAttribute('aria-current', 'step')
  })
})
