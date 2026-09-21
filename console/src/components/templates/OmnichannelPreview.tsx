import { Alert } from 'antd'
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

interface OmnichannelPreviewProps {
  definition: ChannelDefinition
  preview: GenericChannelPreview
  onProfileChange: (profile: string) => void
}

const OmnichannelPreview: React.FC<OmnichannelPreviewProps> = ({ definition, preview, onProfileChange }) => {
  const { t } = useLingui()
  const kind = previewRendererKind(preview.profile)
  const common = { brand: definition.label_key, content: preview.message, direction: preview.direction, device: previewDevice(preview.profile) }

  return (
    <div data-console-preview className="space-y-4">
      <PreviewToolbar>
        <label className="flex items-center gap-2">
          <span>{t`Client preview`}</span>
          <select
            aria-label={t`Client preview`}
            value={preview.profile}
            onChange={(event) => onProfileChange(event.target.value)}
          >
            {definition.preview_profiles.map((profile) => <option key={profile.id} value={profile.id}>{profile.label_key}</option>)}
          </select>
        </label>
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
