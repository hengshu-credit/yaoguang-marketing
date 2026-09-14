import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { i18n } from '@lingui/core'
import { BusinessDefinitionEditor } from './BusinessDefinitionEditor'
import { getBuiltinEventReference } from './builtin-event-reference'
import type { BusinessDefinition } from '../../services/api/business'

vi.mock('../../services/api/business', () => ({ businessApi: { save: vi.fn(), preview: vi.fn() } }))

const definition: BusinessDefinition = { id: 'bounce', kind: 'event', key: 'email.bounced', name: 'Email bounced', status: 'active', schema: { type: 'object' }, settings: { builtin: true, subject_type: 'customer' } }

describe('Built-in event producer reference', () => {
  it('covers every seeded built-in event from the current backend catalogue', () => {
    const seed = readFileSync(resolve(process.cwd(), '../internal/database/schema/business_config.go'), 'utf8')
    const native = readFileSync(resolve(process.cwd(), '../internal/domain/native_business_events.go'), 'utf8')
    const seeded = [...seed.matchAll(/'((?:contact|email|state)\.[a-z_]+)'/g)].map(match => match[1])
    const channels = [...native.matchAll(/\{"([a-z_]+)",\s*"[^"]+"\}/g)].map(match => match[1])
    const suffixes = [...native.match(/for _, suffix := range \[\]string\{([^}]+)\}/)![1].matchAll(/"([a-z_]+)"/g)].map(match => match[1])
    expect(channels.length).toBeGreaterThan(0)
    expect(suffixes.length).toBeGreaterThan(0)
    for (const key of new Set([...seeded, ...channels.flatMap(channel => suffixes.map(suffix => `${channel}.${suffix}`))])) {
      const reference = getBuiltinEventReference(key, i18n)
      expect(reference, key).toBeDefined()
      expect(reference?.variants.length, key).toBeGreaterThan(0)
    }
  })

  it('opens bounce trigger, callback entry and actual timeline fields immediately', () => {
    render(<BusinessDefinitionEditor workspaceId="ws" definition={definition} catalogue={[]} onClose={vi.fn()} onSaved={vi.fn()} />)
    expect(screen.getByRole('tab', { name: 'Trigger and payload' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByText('Provider feedback')).toBeInTheDocument()
    expect(screen.getByText(/classified as a hard bounce/)).toBeInTheDocument()
    expect(screen.getByText(/POST \/webhooks\/email\?provider=/)).toBeInTheDocument()
    expect(screen.getByText('bounced_at')).toBeInTheDocument()
    expect(screen.getByText('old')).toBeInTheDocument()
    expect(screen.getByText('new')).toBeInTheDocument()
    expect(screen.queryByText('data.changes.bounced_at')).not.toBeInTheDocument()
    const changesRow = screen.getByText('changes').closest('tr')!
    fireEvent.click(changesRow.querySelector('button')!)
    expect(screen.queryByText('bounced_at')).not.toBeInTheDocument()
    fireEvent.click(changesRow.querySelector('button')!)
    expect(screen.getByText('bounced_at')).toBeVisible()
    const sample = JSON.parse(screen.getByLabelText('Internal event example').textContent!)
    expect(sample.type).toBe('email.bounced')
    expect(sample.source).toBe('contact_timeline')
    expect(sample.data).toEqual({ operation: 'update', entity_type: 'message_history', entity_id: expect.any(String), changes: { bounced_at: { old: null, new: expect.any(String) } } })
    expect(sample.data).not.toHaveProperty('bounce_diagnostic')
    expect(screen.queryByRole('button', { name: 'Validate sample' })).not.toBeInTheDocument()
    expect(screen.queryByRole('textbox', { name: 'Definition key' })).not.toBeInTheDocument()
  })

  it('distinguishes stored extension declarations from fields emitted by the producer', () => {
    render(<BusinessDefinitionEditor workspaceId="ws" definition={{ ...definition, schema: { type: 'object', properties: { device_fingerprint: { type: 'string' } } } }} catalogue={[]} onClose={vi.fn()} onSaved={vi.fn()} />)
    expect(screen.queryByLabelText('Field key device_fingerprint')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('tab', { name: 'Declared extension fields' }))
    expect(screen.getByLabelText('Field key device_fingerprint')).toBeDisabled()
    expect(screen.getByText(/producer must supply their values/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Save definition' })).not.toBeInTheDocument()
  })

  it('documents path-dependent delivery names and the reduced no-email payload', () => {
    const email = getBuiltinEventReference('email.delivered', i18n)!
    expect(email.variants[0].example.type).toBe('email.updated')
    expect(email.notes.join(' ')).toContain('channel.updated')
    const push = getBuiltinEventReference('push.delivered', i18n)!
    expect(push.variants[1].example.type).toBe('push.delivered')
    expect(push.variants[1].example.data).toEqual(expect.objectContaining({ changes: { message_id: expect.any(String) } }))
    expect(push.endpoints).not.toContain('POST /webhooks/delivery/twilio')
    expect(getBuiltinEventReference('push.unsubscribed', i18n)?.receiptExample).toBeUndefined()
  })

  it('keeps contact creation and state transitions distinct from delivery receipts', () => {
    const created = getBuiltinEventReference('contact.created', i18n)!
    expect(created.variants[0].example.data).toEqual({ operation: 'insert', entity_type: 'contact', entity_id: null, changes: {} })
    const state = getBuiltinEventReference('state.exited', i18n)!
    expect(state.variants[0].example.source).toBe('platform.state_projector')
    expect(state.variants[0].example.data).toEqual(expect.objectContaining({ state_key: 'eligible', previous_value: 'true', value: 'false', rule_revision: 2 }))
    expect(state.notes.join(' ')).toContain('inspection does not emit')
    expect(getBuiltinEventReference('unknown.event', i18n)).toBeUndefined()
  })

  it('retains the existing editor for custom events', () => {
    render(<BusinessDefinitionEditor workspaceId="ws" definition={{ ...definition, key: 'customer.login', settings: {} }} catalogue={[]} onClose={vi.fn()} onSaved={vi.fn()} />)
    expect(screen.getByLabelText('Definition key')).toHaveValue('customer.login')
    expect(screen.getByRole('button', { name: 'Save definition' })).toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: 'Trigger and payload' })).not.toBeInTheDocument()
  })
})
