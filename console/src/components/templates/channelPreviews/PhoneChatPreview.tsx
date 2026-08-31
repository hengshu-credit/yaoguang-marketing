import type { ChannelTemplateContent } from '../../../services/api/template'

interface PhoneChatPreviewProps {
  brand: string
  content: ChannelTemplateContent
  direction: 'ltr' | 'rtl'
}

const PhoneChatPreview: React.FC<PhoneChatPreviewProps> = ({ brand, content, direction }) => (
  <div className="mx-auto min-h-96 w-full max-w-[390px] overflow-hidden rounded-[32px] border-[10px] border-neutral-900 bg-slate-100 shadow-xl">
    <div className="border-b border-slate-200 bg-white px-4 py-3 text-center font-semibold">{brand}</div>
    <div className="space-y-3 p-4">
      <div className="ml-auto max-w-[88%] overflow-hidden rounded-2xl rounded-br-sm bg-white shadow-sm">
        {content.media?.url && (
          <img src={content.media.url} alt={content.media.alt_text || ''} className="max-h-52 w-full object-cover" />
        )}
        <div className="space-y-2 p-3">
          {content.title && <strong className="block break-words">{content.title}</strong>}
          {content.body && <p dir={direction} className="whitespace-pre-wrap break-words">{content.body}</p>}
          {content.footer && <small className="block text-slate-500">{content.footer}</small>}
          {content.actions?.map((action, index) =>
            action.type === 'url' || action.type === 'deep_link' ? (
              <a key={`${action.label}-${index}`} href={action.value} className="block border-t border-slate-100 pt-2 text-center text-blue-600">
                {action.label}
              </a>
            ) : (
              <button key={`${action.label}-${index}`} type="button" className="block w-full border-t border-slate-100 pt-2 text-blue-600">
                {action.label}
              </button>
            )
          )}
        </div>
      </div>
    </div>
  </div>
)

export default PhoneChatPreview

