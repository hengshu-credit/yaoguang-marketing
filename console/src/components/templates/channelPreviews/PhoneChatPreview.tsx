import type { ChannelTemplateContent } from '../../../services/api/template'
import { DeviceFrame, type PreviewDevice } from '../../preview'
import MessageContent from './MessageContent'

export interface ChatPreviewProps {
  brand: string
  content: ChannelTemplateContent
  direction: 'ltr' | 'rtl'
  device?: PreviewDevice
}

const PhoneChatPreview: React.FC<ChatPreviewProps> = ({ brand, content, direction, device = 'ios' }) => (
  <DeviceFrame device={device} title={brand}>
    <div className="material-chat">
      <div className="material-chat__time" aria-hidden="true">09:41</div>
      <div className="material-chat__bubble"><MessageContent content={content} direction={direction} /></div>
    </div>
  </DeviceFrame>
)

export default PhoneChatPreview
