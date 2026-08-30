import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { i18n } from '@lingui/core'
import { FrequencyPoliciesSettings } from './FrequencyPoliciesSettings'
import { frequencyPolicyApi } from '../../services/api/frequency_policy'

vi.mock('../../services/api/frequency_policy', async () => {
  const actual = await vi.importActual<typeof import('../../services/api/frequency_policy')>(
    '../../services/api/frequency_policy'
  )
  return {
    ...actual,
    frequencyPolicyApi: {
      list: vi.fn(),
      save: vi.fn()
    }
  }
})

describe('FrequencyPoliciesSettings', () => {
  beforeEach(() => {
    i18n.load('en', {})
    i18n.activate('en')
    vi.mocked(frequencyPolicyApi.list).mockResolvedValue({ policies: [] })
  })

  it('renders the frequency settings page from the active language catalog', async () => {
    render(<FrequencyPoliciesSettings workspaceId="workspace-1" />)

    const heading = await screen.findByRole('heading', { name: 'Message frequency control' })
    expect(heading).toBeInTheDocument()
    expect(heading.nextElementSibling).toHaveTextContent('Limit marketing messages per customer and channel.')
    expect(screen.getByText('The three levels are independent')).toBeInTheDocument()
  })
})
