import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { BusinessConfigurationSettings } from './BusinessConfigurationSettings'
import { businessApi, type BusinessDefinition } from '../../services/api/business'

vi.mock('../../services/api/business', () => ({ businessApi: { list: vi.fn(), save: vi.fn(), preview: vi.fn(), archive: vi.fn() } }))
const example: BusinessDefinition = { id: 'contract-id', kind: 'object_type', key: 'contract', name: 'Contract', status: 'active', revision: 1, schema: { type: 'object', properties: {} }, settings: {} }

describe('BusinessConfigurationSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(businessApi.list).mockImplementation(async (_workspace, kind) => ({ definitions: kind === 'object_type' ? [example] : [], total: kind === 'object_type' ? 1 : 0 }))
  })
  it('loads arbitrary object types and opens their real persisted definition', async () => {
    render(<BusinessConfigurationSettings workspaceId="workspace-a" canManage />)
    fireEvent.click(screen.getByRole('tab', { name: 'Business object types' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Contract' }))
    expect(await screen.findByRole('textbox', { name: 'Definition key' })).toHaveValue('contract')
    expect(screen.getByRole('textbox', { name: 'Definition key' })).toBeDisabled()
    expect(businessApi.list).toHaveBeenCalledWith('workspace-a', 'object_type', expect.anything())
  })
  it('hides configuration writes when the member cannot manage workspace settings', async () => {
    render(<BusinessConfigurationSettings workspaceId="workspace-a" canManage={false} />)
    expect(screen.queryByRole('button', { name: 'New definition' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('tab', { name: 'Business object types' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Contract' }))
    expect(screen.queryByRole('button', { name: 'Save definition' })).not.toBeInTheDocument()
  })
  it('discards late responses and closes an editor when changing workspace', async () => {
    let resolveOld: ((result: { definitions: BusinessDefinition[]; total: number }) => void) | undefined
    vi.mocked(businessApi.list).mockImplementation((workspace, kind) => workspace === 'old' && kind === 'event' ? new Promise((resolve) => { resolveOld = resolve }) : Promise.resolve({ definitions: [], total: 0 }))
    const { rerender } = render(<BusinessConfigurationSettings workspaceId="old" canManage />)
    await waitFor(() => expect(resolveOld).toBeDefined())
    rerender(<BusinessConfigurationSettings workspaceId="new" canManage />)
    await act(async () => resolveOld?.({ definitions: [{ ...example, kind: 'event', name: 'Old private event' }], total: 1 }))
    expect(screen.queryByText('Old private event')).not.toBeInTheDocument()
    await waitFor(() => expect(businessApi.list).toHaveBeenCalledWith('new', 'event', expect.anything()))
  })
})
