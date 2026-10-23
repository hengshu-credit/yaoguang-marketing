import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import EmailClientFrame from '../templates/EmailClientFrame'

describe('shared HTML device preview', () => {
  it('preserves sandboxed HTML when switching between iOS, Android, desktop and Pad', async () => {
    const { container } = render(<EmailClientFrame html="<p>Member offer</p>" title="Member offer" />)
    for (const [profile, device] of [['iOS', 'ios'], ['Android', 'android'], ['Computer', 'desktop'], ['Pad', 'tablet']]) {
      await userEvent.click(screen.getByRole('button', { name: profile }))
      expect(container.querySelector('[data-preview-device]')).toHaveAttribute('data-preview-device', device)
      expect(container.querySelectorAll('[data-preview-device]')).toHaveLength(1)
      expect(screen.getByTitle('Member offer')).toHaveAttribute('sandbox', '')
      expect(screen.getByTitle('Member offer')).toHaveAttribute('srcdoc', '<p>Member offer</p>')
      expect(screen.getByTitle('Member offer')).toHaveAttribute('data-preview-viewport-width', device === 'desktop' ? '1024' : device === 'tablet' ? '768' : device === 'android' ? '393' : '375')
    }
  })

  it('opens Web material in the H5 browser surface', () => {
    const { container } = render(<EmailClientFrame html="<p>Landing page</p>" title="Landing page" web />)
    expect(container.querySelector('[data-preview-device]')).toHaveAttribute('data-preview-device', 'h5')
    expect(screen.getByTitle('Landing page')).toHaveAttribute('sandbox', '')
  })
})
