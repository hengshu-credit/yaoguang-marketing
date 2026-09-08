import { useState } from 'react'
import { App, Button, Space, Typography } from 'antd'
import { DownloadOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { createContactImportTemplate } from '../../utils/contactImportFile'
import { contactFileSchema } from '../../utils/contactFileSchema'

export function ImportTemplateButtons({ workspaceId, filename = 'customers-import-template' }: { workspaceId: string; filename?: string }) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [downloading, setDownloading] = useState<'csv' | 'xlsx'>()
  const download = async (format: 'csv' | 'xlsx') => {
    setDownloading(format)
    try {
      const fields = await contactFileSchema.load(workspaceId)
      const blob = await createContactImportTemplate(fields.map(field => field.header), format)
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `${filename}.${format}`
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      window.setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch {
      message.error(t`Unable to download the import template. Please try again.`)
    } finally {
      setDownloading(undefined)
    }
  }
  return <div className="mb-4"><Space wrap>
    <Button icon={<DownloadOutlined aria-hidden />} loading={downloading === 'csv'} disabled={Boolean(downloading)} onClick={() => void download('csv')}>{t`Download CSV template`}</Button>
    <Button icon={<DownloadOutlined aria-hidden />} loading={downloading === 'xlsx'} disabled={Boolean(downloading)} onClick={() => void download('xlsx')}>{t`Download Excel template`}</Button>
  </Space><Typography.Paragraph type="secondary" className="mt-2 mb-0">{t`Templates match customer exports and include the latest fields and attributes. System-managed columns are ignored during import.`}</Typography.Paragraph></div>
}
