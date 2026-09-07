import { Mail } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import HtmlPreview from './HtmlPreview'
import type { PreviewDevice } from './DeviceFrame'

export interface EmailPreviewMetadata {
  senderName?: string
  senderEmail?: string
  subject?: string
  previewText?: string
}

export default function EmailPreview({ html, title, device, profile, email }: {
  html: string
  title: string
  device: PreviewDevice
  profile?: string
  email?: EmailPreviewMetadata
}) {
  const { t } = useLingui()
  return <HtmlPreview html={html} title={title} frameTitle={t`Inbox`} device={device} profile={profile} header={
    <div className="material-email-header" data-testid="email-message-header">
      <h2 dir="auto">{email?.subject || t`Email Subject`}</h2>
      <div className="material-email-header__sender">
        <span className="material-email-header__avatar" aria-hidden="true"><Mail size={18} /></span>
        <div><strong dir="auto">{email?.senderName || email?.senderEmail || t`No default sender configured`}</strong>{email?.senderEmail && <small>{email.senderEmail}</small>}</div>
      </div>
      {email?.previewText && <p dir="auto">{email.previewText}</p>}
    </div>
  } />
}
