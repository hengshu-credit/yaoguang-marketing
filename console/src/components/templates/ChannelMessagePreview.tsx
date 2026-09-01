import { Alert, Card, Segmented, Space, Tag, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { PreviewTemplateResponse } from '../../services/api/template'

const { Text, Title } = Typography

interface ChannelMessagePreviewProps {
  preview: PreviewTemplateResponse
  platform: PushClientProfile
  onPlatformChange: (platform: PushClientProfile) => void
  showDiagnostics?: boolean
}

export type PushClientProfile = 'android' | 'ios' | 'web' | 'huawei' | 'honor' | 'xiaomi' | 'oppo' | 'vivo'

const deviceStyle: React.CSSProperties = {
  width: 'min(390px, 100%)',
  minHeight: 260,
  margin: '0 auto',
  padding: 18,
  borderRadius: 34,
  border: '10px solid #171717',
  background: 'linear-gradient(180deg, #e7eef8 0%, #f7f8fa 100%)',
  boxShadow: '0 12px 32px rgba(0,0,0,.16)'
}

const ChannelMessagePreview: React.FC<ChannelMessagePreviewProps> = ({
  preview,
  platform,
  onPlatformChange,
  showDiagnostics = true
}) => {
  const { t } = useLingui()

  if (preview.sms) {
    return (
      <Space orientation="vertical" size="large" className="w-full">
        <div style={deviceStyle}>
          <Text type="secondary">{preview.sms.sender_id || t`SMS`}</Text>
          <div className="mt-4 flex justify-end">
            <div className="max-w-[85%] rounded-2xl rounded-br-sm bg-blue-500 px-4 py-3 text-white whitespace-pre-wrap break-words">
              {preview.sms.body}
            </div>
          </div>
        </div>
        {showDiagnostics && <Card size="small">
          <Space wrap>
            <Tag color="blue">{preview.sms.encoding.toUpperCase()}</Tag>
            <Text>{preview.sms.segment_count} segment(s)</Text>
            <Text type="secondary">
              {preview.sms.character_count} {t`characters`}
            </Text>
            <Text type="secondary">
              {preview.sms.remaining} {t`units remaining`}
            </Text>
          </Space>
        </Card>}
      </Space>
    )
  }

  if (!preview.push) return null

  return (
    <Space orientation="vertical" size="large" className="w-full">
      <Segmented
        block
        value={platform}
        options={[
          { label: 'Android', value: 'android' },
          { label: 'iOS', value: 'ios' },
          { label: t`Web`, value: 'web' },
          { label: 'Huawei', value: 'huawei' },
          { label: 'Honor', value: 'honor' },
          { label: 'Xiaomi', value: 'xiaomi' },
          { label: 'OPPO', value: 'oppo' },
          { label: 'vivo', value: 'vivo' }
        ]}
        onChange={(value) => onPlatformChange(value as PushClientProfile)}
      />
      <div style={deviceStyle}>
        <Card size="small" styles={{ body: { padding: 14 } }}>
          <Space align="start" className="w-full">
            <div className="h-10 w-10 shrink-0 rounded-xl bg-blue-600 text-center text-lg leading-10 text-white">
              N
            </div>
            <div className="min-w-0 flex-1">
              <Title level={5} className="!mb-1 !text-sm break-words">
                {preview.push.title}
              </Title>
              <Text className="whitespace-pre-wrap break-words">{preview.push.body}</Text>
              {preview.push.image_url && (
                <img
                  src={preview.push.image_url}
                  alt=""
                  className="mt-3 max-h-40 w-full rounded-lg object-cover"
                />
              )}
            </div>
          </Space>
        </Card>
      </div>
      {showDiagnostics && <Card size="small">
        <Space wrap>
          <Tag color="purple">{platform}</Tag>
          <Text>{preview.push.payload_bytes} bytes</Text>
          {preview.push.deep_link && <Text type="secondary">{preview.push.deep_link}</Text>}
        </Space>
      </Card>}
      {showDiagnostics && preview.push.warnings.map((warning) => (
        <Alert key={warning.code} type="warning" showIcon title={warning.message} />
      ))}
    </Space>
  )
}

export default ChannelMessagePreview
