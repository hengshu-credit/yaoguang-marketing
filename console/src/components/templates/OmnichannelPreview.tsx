import { Alert, Tag } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { ChannelDefinition } from '../../services/api/channels'
import type { GenericChannelPreview } from '../../services/api/template'
import DesktopChatPreview from './channelPreviews/DesktopChatPreview'
import EnterpriseMessagePreview from './channelPreviews/EnterpriseMessagePreview'
import NotificationPreview from './channelPreviews/NotificationPreview'
import PhoneChatPreview from './channelPreviews/PhoneChatPreview'
import WebhookRequestPreview from './channelPreviews/WebhookRequestPreview'
import { previewRendererKind } from './channelPreviews/clientProfiles'

interface OmnichannelPreviewProps {
  definition: ChannelDefinition
  preview: GenericChannelPreview
  onProfileChange: (profile: string) => void
}

const OmnichannelPreview: React.FC<OmnichannelPreviewProps> = ({ definition, preview, onProfileChange }) => {
  const { t } = useLingui()
  const kind = previewRendererKind(preview.profile)
  const common = { brand: definition.label_key, content: preview.message, direction: preview.direction }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <label className="flex items-center gap-2">
          <span>{t`Client preview`}</span>
          <select
            aria-label={t`Client preview`}
            value={preview.profile}
            onChange={(event) => onProfileChange(event.target.value)}
            className="rounded-md border border-gray-300 bg-white px-3 py-2"
          >
            {definition.preview_profiles.map((profile) => <option key={profile.id} value={profile.id}>{profile.label_key}</option>)}
          </select>
        </label>
        <span className="flex items-center gap-2"><Tag color="gold">{t`Simulated preview`}</Tag>{preview.payload_bytes} bytes</span>
      </div>

      {kind === 'phone' && <PhoneChatPreview {...common} />}
      {kind === 'desktop' && <DesktopChatPreview {...common} />}
      {kind === 'notification' && <NotificationPreview {...common} />}
      {kind === 'enterprise' && <EnterpriseMessagePreview {...common} />}
      {kind === 'http' && <WebhookRequestPreview content={preview.message} />}
      {kind === 'unknown' && <Alert type="error" showIcon title={t`This preview profile is not supported.`} />}

      {preview.warnings.map((warning) => <Alert key={warning.code} type="warning" showIcon title={warning.message} />)}
    </div>
  )
}

export default OmnichannelPreview

