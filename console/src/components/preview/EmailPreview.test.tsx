import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import EmailClientFrame from '../templates/EmailClientFrame'

describe('email client header and body', () => {
  it('keeps the configured sender, subject, summary and body together across devices', async () => {
    const html = '<p>Personalized message body</p>'
    render(<EmailClientFrame title="Preview" html={html} email={{ senderName: 'Member Services', senderEmail: 'members@example.com', subject: 'Welcome Ada', previewText: 'Your member benefits are ready' }} />)
    for (const profile of ['email_mobile', 'email_android', 'email_desktop', 'h5']) {
      await userEvent.selectOptions(screen.getByLabelText('Email client preview'), profile)
      const header = within(screen.getByTestId('email-message-header'))
      expect(header.getByRole('heading', { name: 'Welcome Ada' })).toBeVisible()
      expect(header.getByText('Member Services')).toBeVisible()
      expect(header.getByText('members@example.com')).toBeVisible()
      expect(header.getByText('Your member benefits are ready')).toBeVisible()
      expect(screen.getByTitle('Preview')).toHaveAttribute('srcdoc', html)
      expect(screen.getByTitle('Preview')).toHaveAttribute('sandbox', '')
    }
  })

  it('updates settings as text without modifying compiled body HTML', () => {
    const { rerender } = render(<EmailClientFrame title="Preview" html="<p>Body</p>" email={{ subject: 'Old subject' }} />)
    rerender(<EmailClientFrame title="Preview" html="<p>Body</p>" email={{ senderName: '<b>Sender</b>', subject: 'New subject', previewText: 'New summary' }} />)
    expect(screen.queryByText('Old subject')).not.toBeInTheDocument()
    expect(screen.getByText('<b>Sender</b>')).toBeVisible()
    expect(screen.getByText('New summary')).toBeVisible()
    expect(screen.getByTitle('Preview')).toHaveAttribute('srcdoc', '<p>Body</p>')
  })

  it('keeps web pages free of mail-specific headers', () => {
    render(<EmailClientFrame title="Web page" html="<p>Landing page</p>" web email={{ subject: 'Email only' }} />)
    expect(screen.queryByTestId('email-message-header')).not.toBeInTheDocument()
  })
})
