import { Bell } from 'lucide-react'
import { DeviceFrame } from '../../preview'
import MessageContent from './MessageContent'
import type { ChatPreviewProps } from './PhoneChatPreview'

const NotificationPreview: React.FC<ChatPreviewProps & { inApp?: boolean }> = ({ brand, content, direction, device = 'ios', inApp = false }) => (
  <DeviceFrame device={device} title={brand} appearance={inApp ? 'app' : 'notification'}>
    <div className={inApp ? 'material-in-app' : undefined}>
    <div className="material-notification">
      <div className="material-notification__heading"><span className="material-notification__icon"><Bell size={14} aria-hidden="true" /></span><span className="material-notification__brand">{brand}</span><span>09:41</span></div>
      <MessageContent content={content} direction={direction} />
    </div>
    </div>
  </DeviceFrame>
)

export default NotificationPreview
