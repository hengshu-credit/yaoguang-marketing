import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { I18nProvider } from '@lingui/react'
import { i18n } from '@lingui/core'
import { JourneyPreflightPanel } from './JourneyPreflightPanel'
import { automationApi } from '../../services/api/automation'

vi.mock('../../services/api/automation', () => ({
  automationApi: {
    preflight: vi.fn(),
    activate: vi.fn()
  }
}))

const warningResult = {
  workspace_id: 'ws1',
  automation_id: 'automation-1',
  blocking_count: 0,
  warning_count: 1,
  summary_hash: 'summary.123',
  generated_at: '2026-08-30T00:00:00Z',
  expires_at: '2026-08-30T00:05:00Z',
  issues: [
    {
      code: 'frequency_policy_missing',
      severity: 'warning' as const,
      title: 'Missing message frequency policy',
      description: 'Configure a channel policy before activation.',
      node_id: 'email-1',
      fix_path: '/automations/automation-1'
    }
  ]
}

const renderPanel = (props: Record<string, unknown> = {}) =>
  render(
    <I18nProvider i18n={i18n}>
      <JourneyPreflightPanel
        workspaceId="ws1"
        automationId="automation-1"
        {...props}
      />
    </I18nProvider>
  )

describe('JourneyPreflightPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(automationApi.preflight).mockResolvedValue(warningResult)
    vi.mocked(automationApi.activate).mockResolvedValue({ automation: {} } as never)
  })

  it('locates a blocking issue and prevents activation', async () => {
    const user = userEvent.setup()
    const onFixIssue = vi.fn()
    vi.mocked(automationApi.preflight).mockResolvedValue({
      ...warningResult,
      blocking_count: 1,
      warning_count: 0,
      issues: [
        {
          ...warningResult.issues[0],
          severity: 'blocking',
          title: 'Template is unavailable'
        }
      ]
    })

    renderPanel({ onFixIssue })

    await user.click(await screen.findByRole('button', { name: /Fix Template is unavailable/i }))
    expect(onFixIssue).toHaveBeenCalledWith(expect.objectContaining({ node_id: 'email-1' }))
    expect(screen.getByRole('button', { name: 'Activate journey' })).toBeDisabled()
  })

  it('requires warning confirmation and activates with the sealed hash', async () => {
    const user = userEvent.setup()
    const onActivated = vi.fn()
    renderPanel({ onActivated })

    const activate = await screen.findByRole('button', { name: 'Activate journey' })
    expect(activate).toBeDisabled()
    await user.click(screen.getByRole('checkbox'))
    expect(automationApi.preflight).toHaveBeenCalledTimes(1)
    expect(activate).toBeEnabled()
    await user.click(activate)

    await waitFor(() => {
      expect(automationApi.activate).toHaveBeenCalledWith({
        workspace_id: 'ws1',
        automation_id: 'automation-1',
        preflight_hash: 'summary.123',
        confirm_warnings: true
      })
    })
    expect(onActivated).toHaveBeenCalled()
  })
})
