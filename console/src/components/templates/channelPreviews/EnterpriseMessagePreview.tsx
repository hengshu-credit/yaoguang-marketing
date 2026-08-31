import type { ChannelTemplateContent } from '../../../services/api/template'

const EnterpriseMessagePreview: React.FC<{ brand: string; content: ChannelTemplateContent; direction: 'ltr' | 'rtl' }> = ({ brand, content, direction }) => (
  <div className="rounded-xl border border-slate-200 bg-white shadow-sm">
    <div className="border-b border-slate-200 px-4 py-3 font-semibold">{brand}</div>
    <div className="m-4 rounded-lg border-l-4 border-blue-500 bg-slate-50 p-4">
      {content.title && <strong className="block">{content.title}</strong>}
      {content.body && <p dir={direction} className="mt-2 whitespace-pre-wrap break-words">{content.body}</p>}
    </div>
  </div>
)

export default EnterpriseMessagePreview

