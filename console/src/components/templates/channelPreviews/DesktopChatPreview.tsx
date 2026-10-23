import PhoneChatPreview, { type ChatPreviewProps } from './PhoneChatPreview'

const DesktopChatPreview: React.FC<ChatPreviewProps> = (props) => <PhoneChatPreview {...props} device={props.device || 'desktop'} />

export default DesktopChatPreview
