import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import EmailClientFrame from './EmailClientFrame'

describe('EmailClientFrame', () => {
  it('switches the same compiled email between mobile and desktop surfaces', async () => {
    const user = userEvent.setup()
    render(<EmailClientFrame html="<p>Hello</p>" title="Preview" />)
    const frame = screen.getByTitle('Preview')
    expect(frame).toHaveAttribute('data-client-profile', 'email_mobile')
    await user.selectOptions(screen.getByLabelText(/Email client preview/i), 'email_desktop')
    expect(frame).toHaveAttribute('data-client-profile', 'email_desktop')
    expect(frame).toHaveAttribute('srcdoc', '<p>Hello</p>')
  })
})

