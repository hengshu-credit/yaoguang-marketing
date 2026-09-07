import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { BusinessDefinitionEditor } from './BusinessDefinitionEditor'
import { businessApi, type BusinessDefinition } from '../../services/api/business'

vi.mock('../../services/api/business', () => ({ businessApi: { save: vi.fn(), preview: vi.fn() } }))
const definition: BusinessDefinition = { kind: 'object_type', key: 'contract', name: 'Service contract', schema: { type: 'object', properties: { contract_no: { type: 'string' } }, required: ['contract_no'] }, settings: { key_fields: ['contract_no'] }, status: 'active', revision: 4, id: 'definition-1' }

describe('BusinessDefinitionEditor', () => {
  beforeEach(() => vi.clearAllMocks())

  it('saves the actual workspace definition and optimistic revision while preserving custom object settings', async () => {
    vi.mocked(businessApi.save).mockResolvedValue({ ...definition, revision: 5 })
    const onSaved = vi.fn()
    render(<BusinessDefinitionEditor workspaceId="workspace-a" definition={definition} catalogue={[]} onSaved={onSaved} onClose={() => undefined} />)
    fireEvent.change(screen.getByRole('textbox', { name: 'Definition name' }), { target: { value: 'Renewal contract' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save definition' }))
    await waitFor(() => expect(businessApi.save).toHaveBeenCalledWith('workspace-a', expect.objectContaining({ key: 'contract', name: 'Renewal contract', revision: 4, settings: { key_fields: ['contract_no'] } })))
    await waitFor(() => expect(onSaved).toHaveBeenCalled())
  })

  it('keeps edits visible on a concurrent-write error', async () => {
    vi.mocked(businessApi.save).mockRejectedValue(new Error('Definition changed. Reload before saving.'))
    const onSaved = vi.fn()
    render(<BusinessDefinitionEditor workspaceId="workspace-a" definition={definition} catalogue={[]} onSaved={onSaved} onClose={() => undefined} />)
    fireEvent.click(screen.getByRole('button', { name: 'Save definition' }))
    expect(await screen.findByText('Definition changed. Reload before saving.')).toBeVisible()
    expect(onSaved).not.toHaveBeenCalled()
    expect(screen.getByRole('textbox', { name: 'Definition name' })).toHaveValue('Service contract')
  })

  it('previews an unsaved sample without persisting or emitting an event', async () => {
    vi.mocked(businessApi.preview).mockResolvedValue({ valid: true, errors: [], normalized_event: { event_code: 'contract.signed' } })
    render(<BusinessDefinitionEditor workspaceId="workspace-b" definition={definition} catalogue={[]} onSaved={() => undefined} onClose={() => undefined} />)
    fireEvent.click(screen.getByRole('tab', { name: 'Sample validation' }))
    fireEvent.change(screen.getByRole('textbox', { name: 'Sample JSON' }), { target: { value: '{"contract_no":"C-123"}' } })
    fireEvent.click(screen.getByRole('button', { name: 'Validate sample' }))
    await waitFor(() => expect(businessApi.preview).toHaveBeenCalledWith('workspace-b', definition, { contract_no: 'C-123' }))
    expect(await screen.findByText('Sample is valid')).toBeVisible()
    expect(businessApi.save).not.toHaveBeenCalled()
  })

  it('does not permit read-only viewers to save', () => {
    render(<BusinessDefinitionEditor workspaceId="workspace-a" definition={definition} catalogue={[]} readOnly onSaved={() => undefined} onClose={() => undefined} />)
    expect(screen.queryByRole('button', { name: 'Save definition' })).not.toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: 'Definition name' })).toBeDisabled()
  })
})
