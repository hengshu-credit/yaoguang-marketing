import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { EmailConfigForm } from './EmailConfigForm'
import type { Workspace } from '../../../services/api/types'

const selectorProps = vi.hoisted(() => vi.fn())
vi.mock('../../templates/TemplateSelectorInput', () => ({ default: (props: { onChange: (value: string) => void }) => { selectorProps(props); return null } }))
vi.mock('../../integrations/EmailProviders', () => ({ emailProviders: [] }))

describe('EmailConfigForm', () => {
  it('clears a pinned version when a different template is selected', () => {
    const onChange = vi.fn()
    render(<EmailConfigForm config={{ template_id: 'old', template_version: 4 }} onChange={onChange} workspaceId="ws1" workspace={{ integrations: [], settings: {} } as unknown as Workspace} />)
    const props = selectorProps.mock.calls[0][0] as { onChange: (value: string) => void }
    props.onChange('new-template')
    expect(onChange).toHaveBeenCalledWith({ template_id: 'new-template' })
  })
})

