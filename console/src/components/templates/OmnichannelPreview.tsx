import { Alert } from 'antd'
import { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import type { ChannelDefinition } from '../../services/api/channels'
import type { GenericChannelPreview } from '../../services/api/template'
import DesktopChatPreview from './channelPreviews/DesktopChatPreview'
import EnterpriseMessagePreview from './channelPreviews/EnterpriseMessagePreview'
import NotificationPreview from './channelPreviews/NotificationPreview'
import PhoneChatPreview from './channelPreviews/PhoneChatPreview'
import WebhookRequestPreview from './channelPreviews/WebhookRequestPreview'
import { previewDevice, previewRendererKind } from './channelPreviews/clientProfiles'
import { PreviewToolbar } from '../preview'
import type { PreviewDevice } from '../preview'
import PreviewDeviceButtons from '../preview/PreviewDeviceButtons'

interface OmnichannelPreviewProps {
  definition: ChannelDefinition
  preview: GenericChannelPreview
  onProfileChange: (profile: string) => void
}

const OmnichannelPreview: React.FC<OmnichannelPreviewProps> = ({ definition, preview, onProfileChange }) => {
  const { t } = useLingui()
  const kind = previewRendererKind(preview.profile)
  const [selectedDevice, setDevice] = useState<PreviewDevice>()
  const device = selectedDevice || previewDevice(preview.profile)
  const common = { brand: definition.label_key, content: preview.message, direction: preview.direction, device }

  return (
    <div data-console-preview className="space-y-4">
      <PreviewToolbar>
        {kind !== 'http' && <PreviewDeviceButtons value={device} onChange={next => {
          setDevice(next)
          const profiles = definition.preview_profiles
          const matched = profiles.find(item => previewDevice(item.id) === next)
            || profiles.find(item => item.surface === (next === 'desktop' ? 'desktop' : 'mobile'))
          if (matched && matched.id !== preview.profile) onProfileChange(matched.id)
        }} />}
      </PreviewToolbar>

      {kind === 'phone' && <PhoneChatPreview {...common} />}
      {kind === 'desktop' && <DesktopChatPreview {...common} />}
      {kind === 'notification' && <NotificationPreview {...common} inApp={definition.id === 'in_app'} />}
      {kind === 'enterprise' && <EnterpriseMessagePreview {...common} />}
      {kind === 'http' && <WebhookRequestPreview content={preview.message} />}
      {kind === 'unknown' && <Alert type="error" showIcon title={t`This preview profile is not supported.`} />}

      <div className="text-xs text-slate-500">{preview.payload_bytes} bytes</div>
      {preview.warnings.map((warning) => <Alert key={warning.code} type="warning" showIcon title={warning.message} />)}
    </div>
  )
}

export default OmnichannelPreview
