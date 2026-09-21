import { App } from 'antd'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { CustomerAttributesSettings } from './CustomerAttributesSettings'
import { EventFieldsExtensionDrawer } from './EventFieldsExtensionDrawer'
import { workspaceService } from '../../services/api/workspace'
import { businessApi, type BusinessDefinition } from '../../services/api/business'

vi.mock('../../services/api/workspace', () => ({ workspaceService: { get: vi.fn(), setCustomFieldLabels: vi.fn() } }))
vi.mock('../../services/api/business', () => ({ businessApi: { extendEvents: vi.fn() } }))
const event: BusinessDefinition = { id: 'one', kind: 'event', key: 'contact.created', name: 'Created', status: 'active', schema: { type: 'object' }, settings: { builtin: true } }
describe('Consolidated business configuration', () => {
 beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(workspaceService.get).mockResolvedValue({ workspace: { id: 'ws', settings: { custom_field_labels: { custom_string_1: 'Member ID', custom_number_1: 'Score' } } } } as never)
  vi.mocked(workspaceService.setCustomFieldLabels).mockResolvedValue({} as never)
  vi.mocked(businessApi.extendEvents).mockResolvedValue({ definitions: [event] })
 })
 it('edits legacy labels in the unified attribute table without dropping other labels', async () => {
  render(<App><CustomerAttributesSettings workspaceId="ws" definitions={[]} canManage onEdit={vi.fn()} onCreate={vi.fn()} /></App>)
  const cell = await screen.findByText('Member ID')
  fireEvent.click(within(cell.closest('tr')!).getByRole('button', { name: 'Edit display name' }))
  fireEvent.change(screen.getByRole('textbox', { name: 'Display name' }), { target: { value: 'Membership number' } })
  fireEvent.click(screen.getByRole('button', { name: 'OK' }))
  await waitFor(() => expect(workspaceService.setCustomFieldLabels).toHaveBeenCalledWith({ workspace_id: 'ws', custom_field_labels: { custom_string_1: 'Membership number', custom_number_1: 'Score' } }))
 })
 it('adds a fingerprint to selected built-in and custom events as optional fields', async () => {
  render(<EventFieldsExtensionDrawer workspaceId="ws" events={[event, { ...event, id: 'two', key: 'login', settings: {} }]} onClose={vi.fn()} onSaved={vi.fn()} />)
  fireEvent.click(screen.getByRole('button', { name: 'Add device fingerprint example' }))
  expect(screen.getByRole('button', { name: 'Apply to selected events' })).toBeDisabled()
  fireEvent.click(screen.getByRole('button', { name: 'Select all events' }))
  expect(screen.getByRole('switch', { name: 'Required device_fingerprint' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Add device fingerprint example' })).toBeDisabled()
  expect(screen.getByLabelText('Report data example')).toHaveTextContent('device_fingerprint')
  fireEvent.click(screen.getByRole('button', { name: 'Apply to selected events' }))
  await waitFor(() => expect(businessApi.extendEvents).toHaveBeenCalledWith('ws', ['contact.created', 'login'], expect.objectContaining({ properties: { device_fingerprint: expect.objectContaining({ type: 'string' }) } })))
 })
 it('keeps an overlapping existing attribute definition accessible in the same row', async () => {
  const definition: BusinessDefinition = { ...event, kind: 'customer_attribute', key: 'custom_string_1', schema: { type: 'string' }, settings: { writable: false } }
  const edit = vi.fn()
  render(<App><CustomerAttributesSettings workspaceId="ws" definitions={[definition]} canManage onEdit={edit} onCreate={vi.fn()} /></App>)
  const cell = await screen.findByText('Member ID')
  fireEvent.click(within(cell.closest('tr')!).getByRole('button', { name: 'Configuration' }))
  expect(edit).toHaveBeenCalledWith(definition)
 })
})
