import { useState } from 'react'
import { useLingui } from '@lingui/react/macro'

interface EmailClientFrameProps {
  html: string
  title: string
}

const EmailClientFrame: React.FC<EmailClientFrameProps> = ({ html, title }) => {
  const { t } = useLingui()
  const [profile, setProfile] = useState<'email_mobile' | 'email_desktop'>('email_mobile')
  return <div className="space-y-4">
    <label className="flex items-center gap-2">
      <span>{t`Email client preview`}</span>
      <select aria-label={t`Email client preview`} value={profile} onChange={(event) => setProfile(event.target.value as 'email_mobile' | 'email_desktop')} className="rounded-md border border-gray-300 bg-white px-3 py-2">
        <option value="email_mobile">{t`Mobile email`}</option>
        <option value="email_desktop">{t`Desktop email`}</option>
      </select>
    </label>
    <div className={profile === 'email_mobile' ? 'mx-auto max-w-[430px] rounded-[28px] border-[10px] border-neutral-900 p-2 shadow-xl' : 'w-full rounded-xl border border-gray-200 p-3 shadow'}>
      <iframe
        srcDoc={html}
        title={title}
        sandbox=""
        data-client-profile={profile}
        className="h-[600px] w-full border-0 bg-white"
      />
    </div>
  </div>
}

export default EmailClientFrame

