import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { InputDimensionFilters } from './input_dimension_filters'
import { TableSchemas } from './table_schemas'

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

vi.stubGlobal('ResizeObserver', ResizeObserverStub)

describe('InputDimensionFilters field catalogue', () => {
  beforeEach(() => {
    i18n.load('zh-CN', {
      Email: '邮件',
      equals: '等于',
      'Select a field': '选择字段',
      'select a value': '选择运算符'
    })
    i18n.activate('zh-CN')
  })

  afterEach(() => {
    act(() => i18n.activate('en'))
  })

  it('hides schema fields marked unavailable and localizes fields and operators', async () => {
    render(
      <I18nProvider i18n={i18n}>
        <InputDimensionFilters schema={TableSchemas.contacts} value={[]} onChange={() => {}} />
      </I18nProvider>
    )

    fireEvent.click(screen.getByRole('button', { name: '+ Add filter' }))
    fireEvent.mouseDown(screen.getByRole('combobox'))

    expect(await screen.findByText('邮件')).toBeInTheDocument()
    expect(screen.queryByText('Address Line 2')).not.toBeInTheDocument()
    expect(screen.queryByText('Updated At')).not.toBeInTheDocument()

    fireEvent.click(screen.getByText('邮件'))
    fireEvent.mouseDown(screen.getAllByRole('combobox')[1])
    expect(await screen.findByText('等于')).toBeInTheDocument()
  })
})
