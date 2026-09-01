import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { YaoguangBrand } from './YaoguangBrand'

describe('YaoguangBrand', () => {
  it('shows the product promise in the full vertical identity', () => {
    render(<YaoguangBrand layout="vertical" />)

    expect(screen.getByText('瑶光营销平台')).toBeInTheDocument()
    expect(screen.getByText('观心知意，循光达客')).toBeInTheDocument()
    expect(
      screen.getByText('面向金融科技及互联网业务的开源用户营销与客户触达平台。')
    ).toBeInTheDocument()
  })

  it('retains an accessible product identity in compact mode', () => {
    render(<YaoguangBrand compact />)

    expect(screen.getByLabelText('瑶光营销平台，观心知意，循光达客')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: '衡枢真信' })).toBeInTheDocument()
  })
})
