import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { I18nProvider } from '@lingui/react'
import { i18n } from '@lingui/core'
import { JourneyCreateWizard } from './JourneyCreateWizard'

const setNodes = vi.fn()
const setName = vi.fn()

vi.mock('./context', () => ({
  useAutomation: () => ({
    name: '',
    setName,
    markAsChanged: vi.fn(),
    automation: null,
    workspace: { id: 'ws1' },
    canvasState: {
      nodes: [
        {
          id: 'trigger-1',
          type: 'trigger',
          position: { x: 0, y: 0 },
          data: { nodeType: 'trigger', config: { frequency: 'once' }, label: 'Trigger' }
        }
      ],
      edges: [],
      setNodes,
      setEdges: vi.fn()
    },
    save: vi.fn()
  })
}))

vi.mock('./AutomationFlowEditor', () => ({
  AutomationFlowEditor: () => <div>Advanced flow editor</div>
}))

vi.mock('./config/TriggerConfigForm', () => ({
  TriggerConfigForm: () => <div>Trigger configuration form</div>
}))

vi.mock('../frequency/FrequencyPolicyForm', () => ({
  FrequencyPolicyForm: () => <div>Message frequency policy form</div>
}))

vi.mock('./JourneyPreflightPanel', () => ({
  JourneyPreflightPanel: () => <div>Activation preflight panel</div>
}))

vi.mock('../../services/api/frequency_policy', () => ({
  frequencyPolicyApi: {
    list: vi.fn().mockResolvedValue({ policies: [] }),
    save: vi.fn()
  }
}))

const renderWizard = () =>
  render(
    <I18nProvider i18n={i18n}>
      <JourneyCreateWizard workspaceId="ws1" automationId="automation-1" />
    </I18nProvider>
  )

describe('JourneyCreateWizard', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
  })
  it('starts with six business-oriented journey templates', () => {
    renderWizard()

    for (const template of [
      'Event welcome',
      'Scheduled reminder',
      'Churn win-back',
      'Birthday care',
      'Audience notification',
      'Blank journey'
    ]) {
      expect(screen.getByText(template)).toBeInTheDocument()
    }
  })

  it('keeps entry semantics, entry protection, and message frequency on separate steps', async () => {
    const user = userEvent.setup()
    renderWizard()
    await user.click(screen.getByText('Event welcome'))
    await user.click(screen.getByRole('button', { name: 'Next' }))

    expect(screen.getByText('Trigger configuration form')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Next' }))

    expect(screen.getByText('Once per contact')).toBeInTheDocument()
    expect(screen.getByText('Every time')).toBeInTheDocument()
    expect(screen.getByText('Entry protection: Off')).toBeInTheDocument()
    expect(
      screen.getByText(/only limits Journey entry and is not message frequency control/i)
    ).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Next' }))
    expect(screen.getByText('Advanced flow editor')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Next' }))
    expect(screen.getByText('Message frequency policy form')).toBeInTheDocument()
  })
})
