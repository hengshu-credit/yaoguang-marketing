import type { ChannelTemplateContent } from '../../../services/api/template'
import PhoneChatPreview from './PhoneChatPreview'

const DesktopChatPreview: React.FC<{ brand: string; content: ChannelTemplateContent; direction: 'ltr' | 'rtl' }> = (props) => (
  <div className="rounded-xl border border-slate-200 bg-slate-50 p-4 shadow-sm">
    <div className="mb-3 flex gap-2"><span className="h-3 w-3 rounded-full bg-red-400" /><span className="h-3 w-3 rounded-full bg-amber-400" /><span className="h-3 w-3 rounded-full bg-green-400" /></div>
    <div className="mx-auto max-w-xl"><PhoneChatPreview {...props} /></div>
  </div>
)

export default DesktopChatPreview

