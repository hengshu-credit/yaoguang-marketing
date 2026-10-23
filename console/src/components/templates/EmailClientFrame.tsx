import { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { EmailPreview, HtmlPreview, PreviewToolbar, type PreviewDevice, type EmailPreviewMetadata } from '../preview'
import { webPreviewDocument } from './webTemplateContent'
import PreviewDeviceButtons from '../preview/PreviewDeviceButtons'

interface EmailClientFrameProps {
  html: string
  title: string
  web?: boolean
  email?: EmailPreviewMetadata
}

const devices: Record<string, PreviewDevice> = { email_mobile: 'ios', email_android: 'android', email_desktop: 'desktop', email_tablet: 'tablet', h5: 'h5' }

const EmailClientFrame: React.FC<EmailClientFrameProps> = ({ html, title, web = false, email }) => {
  const { t } = useLingui()
  const [profile, setProfile] = useState(web ? 'h5' : 'email_mobile')
  return <div className="min-w-0">
    <PreviewToolbar>
      <PreviewDeviceButtons label={web ? t`Client preview` : t`Email client preview`} value={devices[profile]} onChange={device => setProfile(device === 'ios' ? 'email_mobile' : `email_${device}`)} />
    </PreviewToolbar>
    {web ? <HtmlPreview html={webPreviewDocument(html)} title={title} device={devices[profile]} profile={profile} browser />
      : <EmailPreview html={html} title={title} device={devices[profile]} profile={profile} email={email} />}
  </div>
}

export default EmailClientFrame
