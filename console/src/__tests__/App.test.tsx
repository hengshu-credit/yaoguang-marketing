import { i18n } from '@lingui/core'
import { act, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, it, expect, vi } from 'vitest'
import { App } from '../App'

const localeState = vi.hoisted(() => ({ current: 'en' }))

vi.mock('../router', () => ({ router: {} }))
vi.mock('@tanstack/react-router', async () => {
  const React = await import('react')
  const { theme } = await import('antd')
  return {
    RouterProvider: () => {
      const { token } = theme.useToken()
      return React.createElement('output', { 'data-testid': 'antd-font-family' }, token.fontFamily)
    }
  }
})

vi.mock('../contexts/LocaleContext', async () => {
  const { i18n } = await import('@lingui/core')
  return {
    i18n,
    LocaleProvider: ({ children }: { children: ReactNode }) => children,
    useLocale: () => ({ locale: localeState.current }),
  }
})

describe('App', () => {
  beforeEach(() => {
    localeState.current = 'en'
    i18n.load('en', { 'Alkaid Marketing Platform': 'Alkaid Marketing Platform' })
    i18n.load('zh-CN', { 'Alkaid Marketing Platform': '瑶光营销平台' })
    i18n.activate('en')
    document.title = ''
  })

  it('renders without crashing', () => {
    render(<App />)
    expect(document.body).toBeDefined()
  })

  it('uses the Tianshu font in the shared Ant Design theme', () => {
    render(<App />)

    expect(screen.getByTestId('antd-font-family')).toHaveTextContent(
      'AlimamaFangYuanTiVF, "PingFang SC", "Microsoft YaHei", sans-serif'
    )
  })

  it('updates the browser title when the configured locale changes', async () => {
    const view = render(<App />)
    await waitFor(() => expect(document.title).toBe('Alkaid Marketing Platform'))

    act(() => {
      localeState.current = 'zh-CN'
      i18n.activate('zh-CN')
    })
    view.rerender(<App />)

    await waitFor(() => expect(document.title).toBe('瑶光营销平台'))
  })
})
