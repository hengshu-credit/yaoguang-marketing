import { Alert, Card, Space, Tag, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { PreviewTemplateResponse } from '../../services/api/template'
import { DeviceFrame, PreviewToolbar } from '../preview'
import NotificationPreview from './channelPreviews/NotificationPreview'

const { Text } = Typography

interface ChannelMessagePreviewProps {
  preview: PreviewTemplateResponse
  platform: PushClientProfile
  onPlatformChange: (platform: PushClientProfile) => void
  showDiagnostics?: boolean
}

export type PushClientProfile = 'android' | 'ios' | 'web' | 'huawei' | 'honor' | 'xiaomi' | 'oppo' | 'vivo'

const ChannelMessagePreview: React.FC<ChannelMessagePreviewProps> = ({
  preview, platform, onPlatformChange, showDiagnostics = true
}) => {
  const { t } = useLingui()
  if (!preview.sms && !preview.push) return null
  const device = platform === 'ios' ? 'ios' : platform === 'web' && !preview.sms ? 'desktop' : 'android'
  const platforms: { label: string; value: PushClientProfile }[] = [
    { label: 'iOS', value: 'ios' }, { label: 'Android', value: 'android' },
    ...(!preview.sms ? [
      { label: 'PC / Web', value: 'web' as const },
      { label: 'Huawei', value: 'huawei' as const }, { label: 'Honor', value: 'honor' as const },
      { label: 'Xiaomi', value: 'xiaomi' as const }, { label: 'OPPO', value: 'oppo' as const }, { label: 'vivo', value: 'vivo' as const }
    ] : [])
  ]
  return <div data-console-preview className="min-w-0 space-y-4">
    <PreviewToolbar>
      <div className="material-preview-platforms" role="group" aria-label={t`Client preview`}>
        {platforms.map((option) => <button key={option.value} type="button" aria-pressed={platform === option.value} onClick={() => onPlatformChange(option.value)}>{option.label}</button>)}
      </div>
    </PreviewToolbar>
    {preview.sms && <>
      <DeviceFrame device={device} title={preview.sms.sender_id || t`SMS`}>
        <div className="material-chat">
          <div className="material-chat__time" aria-hidden="true">09:41</div>
          <div className="material-chat__bubble material-chat__bubble--sms" dir="auto">{preview.sms.body}</div>
        </div>
      </DeviceFrame>
      {showDiagnostics && <Card size="small"><Space wrap>
        <Tag color="blue">{preview.sms.encoding.toUpperCase()}</Tag>
        <Text>{preview.sms.segment_count} segment(s)</Text>
        <Text type="secondary">{preview.sms.character_count} {t`characters`}</Text>
        <Text type="secondary">{preview.sms.remaining} {t`units remaining`}</Text>
      </Space></Card>}
    </>}
    {preview.push && <>
      <NotificationPreview
        device={device}
        brand={t`Push`}
        direction={preview.resolved_language?.startsWith('ur') || preview.resolved_language?.startsWith('ar') ? 'rtl' : 'ltr'}
        content={{ family: 'notification', title: preview.push.title, body: preview.push.body, media: preview.push.image_url ? { type: 'image', url: preview.push.image_url } : undefined }}
      />
      {showDiagnostics && <Card size="small"><Space wrap>
        <Tag color="purple">{platform}</Tag><Text>{preview.push.payload_bytes} bytes</Text>
        {preview.push.deep_link && <Text type="secondary" className="break-all">{preview.push.deep_link}</Text>}
      </Space></Card>}
      {showDiagnostics && preview.push.warnings.map((warning) => <Alert key={warning.code} type="warning" showIcon title={warning.message} />)}
    </>}
  </div>
}

export default ChannelMessagePreview
