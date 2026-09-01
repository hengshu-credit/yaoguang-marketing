import { AutoComplete, Form, Input, Radio, Space, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useFileManager } from '../file_manager/context'
import type { StorageObject } from '../file_manager/interfaces'
import {
  FONT_FAMILY_PRESETS,
  SUPPORTED_FONT_EXTENSION,
  isSupportedConsoleFontFile
} from './consoleFontOptions'

export type ConsoleFontMode = 'name' | 'upload'

const FAMILY_PATTERN = /^[\p{L}\p{N} ._-]+$/u

interface ConsoleFontSettingsProps {
  onChange?: () => void
}

function familyFromFileName(fileName: string): string {
  const withoutExtension = fileName.replace(SUPPORTED_FONT_EXTENSION, '')
  const normalized = withoutExtension
    .replace(/[^\p{L}\p{N} ._-]+/gu, ' ')
    .replace(/\s+/g, ' ')
    .trim()
  return Array.from(normalized).slice(0, 128).join('')
}

export function ConsoleFontSettings({ onChange }: ConsoleFontSettingsProps) {
  const { t } = useLingui()
  const form = Form.useFormInstance()
  const mode = (Form.useWatch('console_font_mode', form) ?? 'name') as ConsoleFontMode
  const { SelectFileButton } = useFileManager()
  const options = [
    { value: FONT_FAMILY_PRESETS[0], label: t`System default (recommended)` },
    { value: FONT_FAMILY_PRESETS[1], label: t`Alimama FangYuan` },
    { value: FONT_FAMILY_PRESETS[2], label: t`PingFang SC` },
    { value: FONT_FAMILY_PRESETS[3], label: t`Microsoft YaHei` },
    { value: FONT_FAMILY_PRESETS[4], label: t`Noto Sans` },
    { value: FONT_FAMILY_PRESETS[5], label: t`Noto Sans SC` },
    { value: FONT_FAMILY_PRESETS[6], label: t`Arial` },
    { value: FONT_FAMILY_PRESETS[7], label: t`Helvetica` }
  ]

  const handleModeChange = (nextMode: ConsoleFontMode) => {
    if (nextMode === 'name') {
      const current = form.getFieldValue('console_font') ?? {}
      form.setFieldValue('console_font', {
        family: current.family ?? '',
        url: undefined,
        file_name: undefined
      })
      onChange?.()
    }
  }

  const handleFontSelected = (url: string, item: StorageObject) => {
    form.setFieldValue('console_font', {
      family: familyFromFileName(item.name),
      url,
      file_name: item.name
    })
    onChange?.()
  }

  return (
    <div>
      <Typography.Title level={5} className="!mb-1">
        {t`Marketing console font`}
      </Typography.Title>
      <Typography.Paragraph type="secondary">
        {t`Applies only to the marketing console. Email templates and customer-facing pages are unchanged.`}
      </Typography.Paragraph>

      <Form.Item name="console_font_mode" label={t`Font source`}>
        <Radio.Group
          options={[
            { label: t`Font name`, value: 'name' },
            { label: t`Uploaded font`, value: 'upload' }
          ]}
          onChange={(event) => handleModeChange(event.target.value as ConsoleFontMode)}
        />
      </Form.Item>

      {mode === 'name' ? (
        <Form.Item
          name={['console_font', 'family']}
          label={t`Font family`}
          rules={[
            {
              validator: async (_, value?: string) => {
                const family = value?.trim() ?? ''
                if (
                  Array.from(family).length <= 128 &&
                  (family === '' || FAMILY_PATTERN.test(family))
                ) {
                  return
                }
                throw new Error(t`Enter one font family name using letters, numbers, spaces, hyphens, underscores, or periods.`)
              }
            }
          ]}
          extra={t`Choose a suggested font or enter the name of a font installed on users' devices.`}
        >
          <AutoComplete
            options={options}
            placeholder={t`System default`}
            filterOption={(input, option) =>
              String(option?.label ?? option?.value ?? '')
                .toLowerCase()
                .includes(input.toLowerCase())
            }
          />
        </Form.Item>
      ) : (
        <Form.Item label={t`Font file`}>
          <Space orientation="vertical" style={{ width: '100%' }}>
            <SelectFileButton
              onSelect={handleFontSelected}
              acceptFileType=".ttf,.otf,.woff,.woff2"
              acceptItem={isSupportedConsoleFontFile}
              buttonText={t`Upload or select font`}
              type="default"
              size="middle"
            />
            <Form.Item noStyle shouldUpdate>
              {() => {
                const fileName = form.getFieldValue(['console_font', 'file_name']) as
                  | string
                  | undefined
                return (
                  <Typography.Text type="secondary">
                    {fileName ? t`Selected file: ${fileName}` : t`No font file selected`}
                  </Typography.Text>
                )
              }}
            </Form.Item>
          </Space>
        </Form.Item>
      )}

      <Form.Item name={['console_font', 'url']} hidden>
        <Input />
      </Form.Item>
      <Form.Item name={['console_font', 'file_name']} hidden>
        <Input />
      </Form.Item>
      {mode === 'upload' && (
        <Form.Item name={['console_font', 'family']} hidden>
          <Input />
        </Form.Item>
      )}
    </div>
  )
}
