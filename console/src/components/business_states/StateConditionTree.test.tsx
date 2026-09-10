import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { StateRuleEditor } from './StateConditionTree'
import { normalizeStateRule } from './state-rule-utils'

const catalogue = { objects: [], events: [], states: [] }
describe('State condition tree', () => {
 it('persists the displayed equality operator and typed value immediately', () => {
  const change = vi.fn()
  const { rerender } = render(<StateRuleEditor {...catalogue} value={{ kind: 'attribute', field: 'tier', operand: 'old' }} onChange={change} attributes={[{ key: 'tier', name: 'Tier', type: 'string' }]} />)
  fireEvent.change(screen.getByRole('textbox', { name: 'Comparison value' }), { target: { value: 'new' } })
  expect(change).toHaveBeenLastCalledWith({ kind: 'attribute', field: 'tier', operator: 'eq', operand: 'new' })
  rerender(<StateRuleEditor {...catalogue} value={{ kind: 'attribute', field: 'tier', operator: 'eq', operand: 'another-state' }} onChange={change} />)
  expect(screen.getByRole('textbox', { name: 'Comparison value' })).toHaveValue('another-state')
 })
 it('keeps numeric comparisons numeric and hides the value for existence checks', () => {
  const change = vi.fn()
  const { rerender } = render(<StateRuleEditor {...catalogue} value={{ kind: 'attribute', field: 'score', operator: 'gte', operand: 10 }} onChange={change} attributes={[{ key: 'score', name: 'Score', type: 'number' }]} />)
  fireEvent.change(screen.getByRole('spinbutton', { name: 'Comparison value' }), { target: { value: '25' } })
  expect(change).toHaveBeenLastCalledWith(expect.objectContaining({ operand: 25 }))
  rerender(<StateRuleEditor {...catalogue} value={{ kind: 'attribute', field: 'score', operator: 'exists' }} onChange={change} />)
  expect(screen.queryByLabelText('Comparison value')).not.toBeInTheDocument()
 })
 it('changes AND to OR without discarding the condition tree and supports subgroups', () => {
  const children = [{ kind: 'event' as const, event: 'login' }]
  const change = vi.fn()
  render(<StateRuleEditor {...catalogue} value={{ kind: 'all', children }} onChange={change} />)
  fireEvent.click(screen.getByRole('button', { name: 'OR' }))
  expect(change).toHaveBeenLastCalledWith({ kind: 'any', children })
  fireEvent.click(screen.getByRole('button', { name: /Add condition group/ }))
  expect(change.mock.calls[change.mock.calls.length - 1]?.[0].children).toEqual([children[0], expect.objectContaining({ kind: 'all', children: [expect.objectContaining({ kind: 'event' })] })])
 })
 it('normalizes legacy nested rules without changing their operands', () => {
  expect(normalizeStateRule({ kind: 'all', children: [{ kind: 'attribute', field: 'flag', operand: false }, { kind: 'state', state_key: 'known' }] })).toEqual({ kind: 'all', children: [{ kind: 'attribute', field: 'flag', operator: 'eq', operand: false }, { kind: 'state', state_key: 'known', value: 'true' }] })
 })
 it('retains JSON number entry for callers without an attribute catalogue', () => {
  const change = vi.fn()
  render(<StateRuleEditor {...catalogue} value={{ kind: 'attribute', field: 'amount', operator: 'gte', operand: 10 }} onChange={change} />)
  fireEvent.change(screen.getByRole('textbox', { name: 'Comparison value' }), { target: { value: '25' } })
  expect(change).toHaveBeenLastCalledWith(expect.objectContaining({ operand: 25 }))
 })
 it('uses automatic customer ownership instead of offering unrelated object relations', () => {
  const change = vi.fn()
  render(<StateRuleEditor {...catalogue} subjectType="customer" value={{ kind: 'object_any', subject_type: 'contract', relation_key: 'old-invalid-relation', children: [{ kind: 'event', event: 'created' }] }} onChange={change} relations={[{ key: 'contract_plans', name: 'Contract plans', settings: { from_type: 'contract', to_type: 'repayment_plan' } }]} />)
  expect(screen.queryByRole('combobox', { name: 'Relation key' })).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Use customer ownership' }))
  expect(change).toHaveBeenLastCalledWith(expect.objectContaining({ relation_key: undefined }))
 })
})
