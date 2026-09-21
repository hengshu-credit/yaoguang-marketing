import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
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

  it('ignores an old successful validation after the operator changes the sample', async () => {
    let resolve!: (result: { valid: boolean; errors: [] }) => void
    vi.mocked(businessApi.preview).mockImplementation(() => new Promise(done => { resolve = done }))
    render(<BusinessDefinitionEditor workspaceId="workspace-a" definition={definition} catalogue={[]} onSaved={vi.fn()} onClose={vi.fn()} />)
    fireEvent.click(screen.getByRole('tab', { name: 'Sample validation' }))
    fireEvent.click(screen.getByRole('button', { name: 'Validate sample' }))
    fireEvent.change(screen.getByRole('textbox', { name: 'Sample JSON' }), { target: { value: '{"contract_no":42}' } })
    await act(async () => resolve({ valid: true, errors: [] }))
    expect(screen.queryByText('Sample is valid')).not.toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: 'Sample JSON' })).toHaveValue('{"contract_no":42}')
  })

  it('asks before discarding a definition edit or a pasted sample', async () => {
    const close = vi.fn()
    render(<BusinessDefinitionEditor workspaceId="workspace-a" definition={definition} catalogue={[]} onSaved={vi.fn()} onClose={close} />)
    fireEvent.change(screen.getByRole('textbox', { name: 'Definition name' }), { target: { value: 'Unsaved' } })
    fireEvent.click(screen.getAllByRole('button', { name: 'Close' })[0])
    expect(await screen.findByText('Discard unsaved changes?')).toBeVisible()
    expect(close).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Keep editing' }))
    expect(screen.getByRole('textbox', { name: 'Definition name' })).toHaveValue('Unsaved')
  })

  it('does not carry a preview response across a workspace change', async () => {
    let finish!: (result: { valid: boolean; errors: []; resolved_customer_id: string }) => void
    vi.mocked(businessApi.preview).mockImplementation(() => new Promise(resolve => { finish = resolve }))
    const props = { definition, catalogue: [], onSaved: vi.fn(), onClose: vi.fn() }
    const { rerender } = render(<BusinessDefinitionEditor {...props} workspaceId="old" />)
    fireEvent.click(screen.getByRole('tab', { name: 'Sample validation' }))
    fireEvent.click(screen.getByRole('button', { name: 'Validate sample' }))
    rerender(<BusinessDefinitionEditor {...props} workspaceId="new" />)
    await act(async () => finish({ valid: true, errors: [], resolved_customer_id: 'old-private-customer' }))
    fireEvent.click(screen.getByRole('tab', { name: 'Sample validation' }))
    expect(screen.queryByText('old-private-customer')).not.toBeInTheDocument()
    expect(screen.queryByText('Sample is valid')).not.toBeInTheDocument()
  })

  it('keeps an incomplete event as a draft but blocks activation and duplicate saves', async () => {
    const draft: BusinessDefinition = { kind: 'event', key: 'login', name: 'Login', status: 'draft', schema: { type: 'object' }, settings: { expiry_mode: 'duration' } }
    let finish!: (value: BusinessDefinition) => void
    vi.mocked(businessApi.save).mockImplementation(() => new Promise(resolve => { finish = resolve }))
    const saved = vi.fn()
    render(<BusinessDefinitionEditor workspaceId="ws" definition={draft} catalogue={[]} onSaved={saved} onClose={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Save and activate' }))
    expect(businessApi.save).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Save draft' }))
    fireEvent.click(screen.getByRole('button', { name: /Save draft/ }))
    expect(businessApi.save).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('textbox', { name: 'Definition name' })).toBeDisabled()
    await act(async () => finish(draft))
    expect(saved).toHaveBeenCalledWith(draft)
  })

  it('generates an event example and copies the actual backend envelope, without sending it', async () => {
    const event: BusinessDefinition = { kind: 'event', key: 'login', name: 'Login', status: 'draft', schema: { type: 'object', properties: { method: { type: 'string', enum: ['password'] } } }, settings: { expiry_mode: 'never' } }
    render(<BusinessDefinitionEditor workspaceId="workspace-a" definition={event} catalogue={[]} onSaved={vi.fn()} onClose={vi.fn()} />)
    fireEvent.click(screen.getByRole('tab', { name: 'Sample validation' }))
    const sample = JSON.parse((screen.getByRole('textbox', { name: 'Sample JSON' }) as HTMLTextAreaElement).value)
    expect(sample).toMatchObject({ event_code: 'login', data: { method: 'password' } })
    fireEvent.change(screen.getByRole('textbox', { name: 'Sample JSON' }), { target: { value: '{"my":"payload"}' } })
    fireEvent.click(screen.getByRole('button', { name: 'Generate sample from configuration' }))
    expect(await screen.findByText('Replace the edited sample?')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Keep sample' }))
    expect(screen.getByRole('textbox', { name: 'Sample JSON' })).toHaveValue('{"my":"payload"}')
    fireEvent.click(screen.getByRole('tab', { name: 'Integration instructions' }))
    const request = JSON.parse(screen.getByLabelText('Backend request JSON').textContent || '{}')
    expect(request).toMatchObject({ workspace_id: 'workspace-a', source: 'business-app', event: { my: 'payload' } })
    expect(businessApi.save).not.toHaveBeenCalled()
    expect(businessApi.preview).not.toHaveBeenCalled()
  })
})
