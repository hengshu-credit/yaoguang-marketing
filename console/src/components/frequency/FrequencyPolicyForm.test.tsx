import { render, screen } from '@testing-library/react'
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
})
