import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ChannelConfigForm } from './ChannelConfigForm'
import type { Workspace } from '../../../services/api/types'

const selectorProps = vi.hoisted(() => vi.fn())
vi.mock('../../templates/TemplateSelectorInput', () => ({
  default: (props: { channel?: string }) => {
    selectorProps(props)
    return <div data-testid="template-channel">{props.channel}</div>
  }
}))

const workspace = {
  integrations: [
    { id: 'sms-provider', name: 'SMS provider', type: 'sms' },
    { id: 'push-provider', name: 'Push provider', type: 'push' }
  ],
  settings: { languages: ['zh-CN'] }
} as unknown as Workspace

describe('ChannelConfigForm', () => {
  it('keeps SMS and Push template selectors scoped to their node channel', () => {
    const onChange = vi.fn()
    const { rerender } = render(
      <ChannelConfigForm
        nodeType="sms"
        config={{ template_id: '', integration_id: '' }}
        onChange={onChange}
        workspaceId="ws1"
        workspace={workspace}
      />
    )
    expect(screen.getByTestId('template-channel')).toHaveTextContent('sms')

    rerender(
      <ChannelConfigForm
        nodeType="push"
        config={{ template_id: '', integration_id: '' }}
        onChange={onChange}
        workspaceId="ws1"
        workspace={workspace}
      />
    )
    expect(screen.getByTestId('template-channel')).toHaveTextContent('push')
    expect(selectorProps).toHaveBeenLastCalledWith(expect.objectContaining({ channel: 'push' }))
  })
})
