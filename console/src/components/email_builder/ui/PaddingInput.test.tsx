import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import PaddingInput from './PaddingInput'

/**
 * Regression tests for issue #369.
 *
 * mj-button / mj-social use PaddingInput in "shorthand string" mode for their
 * `inner-padding` attribute. Before a value is set, `value` is `undefined`. The
 * component must still treat this as shorthand mode and emit a STRING — emitting
 * an object {top, right, bottom, left} is what got serialized by the backend as
 * the Go map literal `map[bottom:0px top:0px]`, which Gmail rejects.
 */
describe('PaddingInput', () => {
  const getSides = () => screen.getAllByRole('spinbutton')

  it('emits a string in shorthand mode when value is undefined (issue #369)', () => {
    const onChange = vi.fn()
    // Shorthand mode is signalled by a string defaultValue (the mj-button case).
    render(<PaddingInput value={undefined} defaultValue="10px 25px" onChange={onChange} />)

    // Sides render top, right, bottom, left — change "top".
    fireEvent.change(getSides()[0], { target: { value: '5' } })

    expect(onChange).toHaveBeenCalled()
    const arg = onChange.mock.calls.at(-1)![0]
    expect(typeof arg).toBe('string')
    expect(arg).not.toContain('map[')
  })

  it('emits a string in shorthand mode when value is a string', () => {
    const onChange = vi.fn()
    render(<PaddingInput value="10px 25px" defaultValue="10px 25px" onChange={onChange} />)

    fireEvent.change(getSides()[0], { target: { value: '8' } })

    expect(onChange).toHaveBeenCalled()
    const arg = onChange.mock.calls.at(-1)![0]
    expect(typeof arg).toBe('string')
  })

  it('emits an object in individual mode (container padding case)', () => {
    const onChange = vi.fn()
    render(
      <PaddingInput
        value={{ top: '10px', right: '25px', bottom: '10px', left: '25px' }}
        onChange={onChange}
      />
    )

    fireEvent.change(getSides()[0], { target: { value: '12' } })

    expect(onChange).toHaveBeenCalled()
    const arg = onChange.mock.calls.at(-1)![0]
    expect(typeof arg).toBe('object')
    expect(arg).toHaveProperty('top', '12px')
  })
})
