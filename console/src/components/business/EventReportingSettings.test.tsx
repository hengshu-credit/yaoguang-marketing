import { useState } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import type { BusinessDefinition } from '../../services/api/business'
import { EventReportingSettings } from './EventReportingSettings'
import { BusinessDefinitionSettings } from './BusinessDefinitionSettings'
import { reportingIssues } from './event-reporting'

const definition: BusinessDefinition = { kind: 'event', key: 'login', name: 'Login', status: 'draft', schema: {}, settings: { expiry_mode: 'duration', ttl_seconds: 90, authoritative: true } }
function Harness({ initial = definition }: { initial?: BusinessDefinition }) {
  const [value, setValue] = useState(initial)
  return <><EventReportingSettings value={value} catalogue={[]} onChange={setValue} issues={reportingIssues(value)} /><output data-testid="definition">{JSON.stringify(value)}</output></>
}
const stored = () => JSON.parse(screen.getByTestId('definition').textContent || '{}') as BusinessDefinition

describe('event reporting settings', () => {
  it('switches browser and trusted backend reporting atomically', async () => {
    render(<Harness />)
    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Reporting mode' }))
    await userEvent.click(screen.getByText('Browser behavior and authenticated backend'))
    expect(stored().settings).toMatchObject({ browser_enabled: true, authoritative: false })
    expect(screen.getByRole('combobox', { name: 'Allowed browser origins' })).toBeInTheDocument()
    expect(screen.getByText(/Add at least one exact HTTP origin/)).toBeVisible()
  })
  it('changes display units without rounding the persisted seconds', async () => {
    render(<Harness />)
    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Duration unit' }))
    await userEvent.click(screen.getByText('Minutes'))
    expect(screen.getByRole('spinbutton', { name: 'Default validity duration' })).toHaveValue('1.5')
    expect(stored().settings.ttl_seconds).toBe(90)
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Default validity duration' }), { target: { value: '2' } })
    await waitFor(() => expect(stored().settings.ttl_seconds).toBe(120))
  })
  it('displays the actual legacy expiration default', () => {
    render(<Harness initial={{ ...definition, settings: {} }} />)
    expect(screen.getByText('Require expiration in the report')).toBeVisible()
    expect(stored().settings).not.toHaveProperty('expiry_mode')
  })
  it('removes a source restriction only after an explicit mode change', () => {
    render(<Harness initial={{ ...definition, settings: { allowed_sources: [] } }} />)
    expect(screen.getByText(/Select at least one source/)).toBeVisible()
    fireEvent.click(screen.getByRole('switch', { name: 'Restrict backend source systems' }))
    expect(stored().settings).not.toHaveProperty('allowed_sources')
  })
  it('clears an optional external mapping without saving an empty path', () => {
    let value: BusinessDefinition | undefined
    render(<BusinessDefinitionSettings value={{ ...definition, kind: 'event_mapping', settings: { fields: { expires_at: '$.expiry' } } }} catalogue={[]} onChange={next => { value = next }} />)
    fireEvent.change(screen.getByRole('textbox', { name: 'Event expiration time path' }), { target: { value: '' } })
    expect(value?.settings.fields).toEqual({})
  })
})
