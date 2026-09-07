import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { FunnelEditor } from './FunnelEditor'
vi.mock('@lingui/react/macro', () => ({ useLingui: () => ({ t: (strings: TemplateStringsArray, ...values: unknown[]) => strings.reduce((s, part, i) => s + part + (values[i] ?? ''), '') }) }))
describe('FunnelEditor', () => {
 it('uses the configured object catalogue and edits steps without fixed business types', () => {
  const change = vi.fn()
  render(<FunnelEditor value={{ subject_type: 'contract', window_seconds: 3600, steps: [{ id: 'a', events: ['signed'] }] }} onChange={change} objects={[{ key: 'contract', name: 'Contract' }]} events={[{ key: 'signed', name: 'Contract signed' }]} />)
  expect(screen.getByRole('combobox', { name: 'Counting object' }).parentElement).toHaveTextContent('Contract')
  expect(screen.getByRole('combobox', { name: 'Step object' })).toBeDisabled()
  fireEvent.click(screen.getByRole('button', { name: 'Duplicate step' }))
  expect(change.mock.calls[0][0].steps).toHaveLength(2)
  expect(change.mock.calls[0][0].steps[1].id).not.toBe('a')
 })
})
