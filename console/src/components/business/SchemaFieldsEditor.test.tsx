import { useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SchemaFieldsEditor } from './SchemaFieldsEditor'
import type { BusinessSchema } from './business-schema'

function Example() {
  const [value, setValue] = useState<BusinessSchema>({ type: 'object', properties: {}, required: [] })
  return <><SchemaFieldsEditor value={value} onChange={setValue} /><output data-testid="schema">{JSON.stringify(value)}</output></>
}

describe('SchemaFieldsEditor', () => {
  it('adds, renames and marks a configurable field as required', () => {
    render(<Example />)
    fireEvent.click(screen.getByRole('button', { name: 'Add field' }))
    const key = screen.getByRole('textbox', { name: 'Field key field_1' })
    fireEvent.change(key, { target: { value: 'contract_no' } })
    fireEvent.blur(key)
    fireEvent.click(screen.getByRole('switch', { name: 'Required contract_no' }))
    const result = JSON.parse(screen.getByTestId('schema').textContent!)
    expect(result.properties.contract_no.type).toBe('string')
    expect(result.required).toEqual(['contract_no'])
  })

  it('does not expose write actions to a read-only viewer', () => {
    render(<SchemaFieldsEditor value={{ type: 'object', properties: { email: { type: 'string' } } }} onChange={() => undefined} disabled />)
    expect(screen.getByRole('button', { name: 'Add field' })).toBeDisabled()
    expect(screen.getByRole('textbox', { name: 'Field key email' })).toBeDisabled()
  })
})
