import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { Form } from 'antd'
import { describe, expect, it, vi } from 'vitest'
import type { StorageObject } from '../file_manager/interfaces'
import { ConsoleFontSettings } from './ConsoleFontSettings'
import { isSupportedConsoleFontFile } from './consoleFontOptions'

const selectedFont: StorageObject = {
  key: 'fonts/brand.woff2',
  name: 'brand.woff2',
  is_folder: false,
  path: 'fonts/brand.woff2',
  last_modified: new Date('2026-09-01T00:00:00Z'),
  file_info: {
    size: 12000,
    size_human: '12 KB',
    content_type: 'font/woff2',
    url: 'https://cdn.example.com/fonts/brand.woff2'
  }
}

vi.mock('../file_manager/context', () => ({
  useFileManager: () => ({
    SelectFileButton: ({
      onSelect,
      acceptItem,
      buttonText
    }: {
      onSelect: (url: string, item: StorageObject) => void
      acceptItem: (item: StorageObject) => boolean
      buttonText: string
    }) => (
      <button
        type="button"
        data-accepts-selected={String(acceptItem(selectedFont))}
        onClick={() => onSelect(selectedFont.file_info.url, selectedFont)}
      >
        {buttonText}
      </button>
    )
  })
}))

function Values() {
  const form = Form.useFormInstance()
  return (
    <Form.Item noStyle shouldUpdate>
      {() => <output data-testid="font-values">{JSON.stringify(form.getFieldsValue(true))}</output>}
    </Form.Item>
  )
}

function renderSettings(onChange = vi.fn()) {
  return {
    onChange,
    ...render(
      <Form
        initialValues={{
          console_font_mode: 'name',
          console_font: { family: '' }
        }}
      >
        <ConsoleFontSettings onChange={onChange} />
        <Values />
      </Form>
    )
  }
}

describe('ConsoleFontSettings', () => {
  it('offers editable named-font choices for multilingual consoles', async () => {
    renderSettings()

    expect(screen.getByRole('radio', { name: 'Font name' })).toBeChecked()
    expect(screen.getByRole('radio', { name: 'Uploaded font' })).toBeInTheDocument()
    const family = screen.getByRole('combobox', { name: 'Font family' })
    fireEvent.mouseDown(family)

    for (const option of [
      'System default (recommended)',
      'Alimama FangYuan',
      'PingFang SC',
      'Microsoft YaHei',
      'Noto Sans SC',
      'Arial',
      'Helvetica'
    ]) {
      expect(await screen.findByText(option)).toBeInTheDocument()
    }

    fireEvent.change(family, { target: { value: 'Noto Sans JP' } })
    await waitFor(() => {
      expect(screen.getByTestId('font-values')).toHaveTextContent('Noto Sans JP')
    })
  })

  it('records the selected font URL, filename, and inferred family', async () => {
    const { onChange } = renderSettings()

    fireEvent.click(screen.getByRole('radio', { name: 'Uploaded font' }))
    const selectButton = await screen.findByRole('button', { name: 'Upload or select font' })
    expect(selectButton).toHaveAttribute('data-accepts-selected', 'true')
    fireEvent.click(selectButton)

    await waitFor(() => {
      const values = screen.getByTestId('font-values')
      expect(values).toHaveTextContent('https://cdn.example.com/fonts/brand.woff2')
      expect(values).toHaveTextContent('brand.woff2')
      expect(values).toHaveTextContent('brand')
    })
    expect(onChange).toHaveBeenCalled()
  })

  it('clears uploaded fields when returning to the named-font mode', async () => {
    renderSettings()
    fireEvent.click(screen.getByRole('radio', { name: 'Uploaded font' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Upload or select font' }))
    fireEvent.click(screen.getByRole('radio', { name: 'Font name' }))

    await waitFor(() => {
      const values = screen.getByTestId('font-values').textContent ?? ''
      expect(values).not.toContain('https://cdn.example.com/fonts/brand.woff2')
      expect(values).not.toContain('brand.woff2')
    })
  })
})

describe('isSupportedConsoleFontFile', () => {
  it.each(['font.ttf', 'font.OTF', 'font.woff', 'font.WOFF2'])(
    'accepts supported file %s',
    (name) => {
      expect(isSupportedConsoleFontFile({ ...selectedFont, name })).toBe(true)
    }
  )

  it('rejects folders and unsupported extensions', () => {
    expect(isSupportedConsoleFontFile({ ...selectedFont, name: 'font.eot' })).toBe(false)
    expect(isSupportedConsoleFontFile({ ...selectedFont, is_folder: true })).toBe(false)
  })
})
