import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HeadersEditor } from './HeadersEditor'

describe('HeadersEditor', () => {
  it('offers common HTTP header names in an editable selector', async () => {
    const user = userEvent.setup()
    render(<HeadersEditor />)

    await user.click(screen.getByRole('button', { name: /Add.*header/i }))

    const headerInput = screen.getByRole('combobox')
    await user.click(headerInput)
    await user.type(headerInput, 'API')

    await screen.findByRole('option', { name: 'X-API-Key' })
    await user.click(screen.getByTitle('X-API-Key'))

    expect(headerInput).toHaveValue('X-API-Key')
  })

  it('preserves a custom header name and template placeholders in its content', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<HeadersEditor onChange={onChange} />)

    await user.click(screen.getByRole('button', { name: /Add.*header/i }))
    await user.type(screen.getByRole('combobox'), 'X-Customer-Key')
    fireEvent.change(screen.getByPlaceholderText('Bearer {{ contact.custom_string_1 }}'), {
      target: { value: 'Bearer {{ contact.custom_string_1 }}' }
    })
    await user.click(screen.getByRole('button', { name: 'Add' }))

    await waitFor(() => {
      expect(onChange).toHaveBeenCalledWith([
        {
          name: 'X-Customer-Key',
          value: 'Bearer {{ contact.custom_string_1 }}'
        }
      ])
    })
  })
})
