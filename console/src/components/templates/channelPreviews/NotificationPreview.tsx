import type { ChannelTemplateContent } from '../../../services/api/template'

const NotificationPreview: React.FC<{ brand: string; content: ChannelTemplateContent; direction: 'ltr' | 'rtl' }> = ({ brand, content, direction }) => (
  <div className="mx-auto min-h-72 w-full max-w-[390px] rounded-[32px] border-[10px] border-neutral-900 bg-gradient-to-b from-sky-200 to-slate-100 p-4 shadow-xl">
    <div className="mt-10 rounded-2xl bg-white/95 p-4 shadow">
      <small className="text-slate-500">{brand}</small>
      {content.title && <strong className="mt-1 block break-words">{content.title}</strong>}
      {content.body && <p dir={direction} className="mt-1 whitespace-pre-wrap break-words">{content.body}</p>}
    </div>
  </div>
)

export default NotificationPreview

