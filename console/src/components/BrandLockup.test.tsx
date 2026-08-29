import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { BrandLockup } from './BrandLockup'

describe('BrandLockup', () => {
  it('renders the approved full product identity', () => {
    render(<BrandLockup />)

    expect(screen.getByRole('img', { name: '恒数科技' })).toHaveAttribute(
      'src',
      '/console/images/hengshucredit_animated.svg'
    )
    expect(screen.getByText('瑶光营销平台')).toBeInTheDocument()
    expect(screen.getByText('观心知意，循光达客')).toBeInTheDocument()
  })

  it('keeps an accessible logo but hides text in compact mode', () => {
    render(<BrandLockup compact />)

    expect(screen.getByRole('img', { name: '恒数科技' })).toBeInTheDocument()
    expect(screen.queryByText('瑶光营销平台')).not.toBeInTheDocument()
    expect(screen.queryByText('观心知意，循光达客')).not.toBeInTheDocument()
  })
})
