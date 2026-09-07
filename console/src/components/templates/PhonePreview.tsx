import { useLingui } from '@lingui/react/macro'
import { DeviceFrame } from '../preview'

const IphoneEmailPreview = ({ sender, subject, previewText, timestamp, currentTime = '9:41' }: {
  sender: string
  subject: string
  previewText: string
  timestamp: string
  currentTime: string
}) => {
  const { t } = useLingui()
  return <DeviceFrame device="ios" compact currentTime={currentTime}>
    <div className="material-inbox">
      <h3>{t`Inbox`}</h3>
      <div className="material-inbox__row">
        <span className="material-inbox__dot" aria-hidden="true" />
        <div className="material-inbox__content">
          <div className="material-inbox__sender"><strong>{sender}</strong><time>{timestamp}</time></div>
          <div className="material-inbox__subject">{subject}</div>
          <div className="material-inbox__excerpt">{previewText}</div>
        </div>
      </div>
    </div>
  </DeviceFrame>
}

export default IphoneEmailPreview
