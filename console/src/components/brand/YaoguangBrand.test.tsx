import { render, screen } from '@testing-library/react'
import { describe, expect, it, afterEach } from 'vitest'
import { i18n } from '../../i18n'
import { YaoguangBrand } from './YaoguangBrand'

describe('YaoguangBrand', () => {
  afterEach(() => {
    i18n.activate('en')
  })

  it('shows the localized product name and tagline in English', () => {
    i18n.loadAndActivate({
      locale: 'en',
      messages: {
        'Alkaid Marketing Platform': 'Alkaid Marketing Platform',
        'Understanding Intent. Reaching Customers': 'Understanding Intent. Reaching Customers'
      }
    })

    render(<YaoguangBrand layout="vertical" />)

    expect(screen.getByText('Alkaid Marketing Platform')).toBeInTheDocument()
    expect(screen.getByText('Understanding Intent. Reaching Customers')).toBeInTheDocument()
    expect(
      screen.getByText('面向金融科技及互联网业务的开源用户营销与客户触达平台。')
    ).toBeInTheDocument()
  })

  it('uses localized text in the compact accessible label', () => {
    i18n.loadAndActivate({
      locale: 'zh-CN',
      messages: {
        'Alkaid Marketing Platform': '瑶光营销平台',
        'Understanding Intent. Reaching Customers': '观心知意，循光达客'
      }
    })

    render(<YaoguangBrand compact />)

    expect(screen.getByLabelText('瑶光营销平台, 观心知意，循光达客')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: '衡枢真信' })).toBeInTheDocument()
  })

})
