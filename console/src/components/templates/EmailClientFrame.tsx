import { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { EmailPreview, HtmlPreview, PreviewToolbar, type PreviewDevice, type EmailPreviewMetadata } from '../preview'
import { webPreviewDocument } from './webTemplateContent'

interface EmailClientFrameProps {
  html: string
  title: string
  web?: boolean
  email?: EmailPreviewMetadata
}

const devices: Record<string, PreviewDevice> = { email_mobile: 'ios', email_android: 'android', email_desktop: 'desktop', h5: 'h5' }

const EmailClientFrame: React.FC<EmailClientFrameProps> = ({ html, title, web = false, email }) => {
  const { t } = useLingui()
  const [profile, setProfile] = useState(web ? 'h5' : 'email_mobile')
  return <div className="min-w-0">
    <PreviewToolbar>
      <label>
        <span>{t`Client preview`}</span>
        <select aria-label={web ? t`Client preview` : t`Email client preview`} value={profile} onChange={(event) => setProfile(event.target.value)}>
          <option value="email_mobile">{web ? 'iOS' : `iOS · ${t`Mobile email`}`}</option>
          <option value="email_android">{web ? 'Android' : `Android · ${t`Mobile email`}`}</option>
          <option value="email_desktop">{web ? 'PC' : `PC · ${t`Desktop email`}`}</option>
          <option value="h5">H5</option>
        </select>
      </label>
    </PreviewToolbar>
    {web ? <HtmlPreview html={webPreviewDocument(html)} title={title} device={devices[profile]} profile={profile} browser />
      : <EmailPreview html={html} title={title} device={devices[profile]} profile={profile} email={email} />}
  </div>
}

export default EmailClientFrame
