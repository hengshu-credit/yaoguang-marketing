import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'
import { AutomationAudienceRunModal } from './AutomationAudienceRunModal'
import { automationApi } from '../../services/api/automation'

vi.mock('../../services/api/automation', () => ({
  automationApi: { startAudience: vi.fn() }
}))

beforeEach(() => vi.clearAllMocks())

it('starts the chosen live automation from the chosen audience and reports counts', async () => {
  vi.mocked(automationApi.startAudience).mockResolvedValue({
    automation_id: 'automation-1', audience_id: 'audience-1', audience_version: 7,
    build_id: 'build-7', candidate_count: 3, enrolled_count: 2
  })
  render(
    <AutomationAudienceRunModal
      open
      workspaceId="workspace-1"
      automations={[{ id: 'automation-1', name: '还款提醒', status: 'live' }]}
      audiences={[{ id: 'audience-1', name: '待还款客户' }]}
      onClose={vi.fn()}
    />
  )
  const selects = screen.getAllByRole('combobox')
  await userEvent.click(selects[0])
  await userEvent.click(await screen.findByText('还款提醒'))
  await userEvent.click(selects[1])
  await userEvent.click(await screen.findByText('待还款客户'))
  fireEvent.click(screen.getByRole('button', { name: 'Start audience run' }))

  await waitFor(() => expect(automationApi.startAudience).toHaveBeenCalledWith(
    'workspace-1', 'automation-1', 'audience-1'
  ))
  expect(await screen.findByText('3 candidates, 2 enrolled')).toBeInTheDocument()
})
