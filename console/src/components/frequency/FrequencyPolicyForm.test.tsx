import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { FrequencyPolicyForm } from './FrequencyPolicyForm'

describe('FrequencyPolicyForm', () => {
  it('keeps campaign, trigger, and workspace caps as separate business cards', () => {
    const onSave = vi.fn()
    const { rerender } = render(<FrequencyPolicyForm scope="campaign" onSave={onSave} />)
    expect(screen.getByText('本活动限制')).toBeInTheDocument()
    rerender(<FrequencyPolicyForm scope="trigger" onSave={onSave} />)
    expect(screen.getByText('事件 / 定时触发限制')).toBeInTheDocument()
    expect(screen.getByText('入场频次与消息频控是两套规则')).toBeInTheDocument()
    rerender(<FrequencyPolicyForm scope="workspace_global" onSave={onSave} />)
    expect(screen.getByText('Workspace 全量限制')).toBeInTheDocument()
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

    expect(screen.queryByLabelText('自动化:触发器标识')).not.toBeInTheDocument()
    await user.click(screen.getByRole('switch', { name: '事件 / 定时触发限制开关' }))
    await user.click(screen.getByRole('button', { name: '保存此层策略' }))

    await waitFor(() => {
      expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ scope_ref: 'automation-1' }))
    })
  })
})
