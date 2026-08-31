import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AudienceDrawer } from './AudienceDrawer'
import { audienceApi } from '../../services/api/marketing'
import type { TreeNode } from '../../services/api/segment'

const unpaidTree: TreeNode = {
  kind: 'leaf',
  leaf: {
    source: 'contacts',
    contact: {
      filters: [{
        field_name: 'profile_status', field_type: 'string', operator: 'equals',
        string_values: ['unpaid']
      }]
    }
  }
}

vi.mock('../../services/api/marketing', () => ({
  audienceApi: {
    preview: vi.fn(),
    create: vi.fn(),
    update: vi.fn()
  }
}))

vi.mock('../segment/input', () => ({
  TreeNodeInput: ({ onChange, onDraftTreeChange }: {
    onChange?: (tree: TreeNode) => void
    onDraftTreeChange?: (tree: TreeNode | undefined) => void
  }) => (
    <>
      <button onClick={() => onDraftTreeChange?.(unpaidTree)}>Draft unpaid condition</button>
      <button onClick={() => onChange?.(unpaidTree)}>Confirm unpaid condition</button>
    </>
  )
}))

const renderDrawer = () => render(
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <AudienceDrawer open workspaceId="workspace-1" lists={[]} onClose={vi.fn()} />
  </QueryClientProvider>
)

describe('AudienceDrawer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useRealTimers()
  })

  it('previews the whole draft condition and saves only the dynamic definition', async () => {
    vi.mocked(audienceApi.preview).mockResolvedValue({ customers: [], total: 12 })
    vi.mocked(audienceApi.create).mockResolvedValue({ id: 'audience-1', name: '待还款客户', kind: 'dynamic', active_version: 1 })
    renderDrawer()

    fireEvent.change(screen.getByLabelText('Audience name'), { target: { value: '待还款客户' } })
    fireEvent.click(screen.getByRole('button', { name: 'Draft unpaid condition' }))
    await waitFor(() => expect(audienceApi.preview).toHaveBeenCalledWith('workspace-1', { condition: unpaidTree }))
    expect(await screen.findByText('12 customers match')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Confirm unpaid condition' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save audience' }))
    await waitFor(() => expect(audienceApi.create).toHaveBeenCalledWith(
      'workspace-1', '待还款客户', '', { condition: unpaidTree }, 'dynamic'
    ))
    expect(audienceApi).not.toHaveProperty('build')
  })
})
