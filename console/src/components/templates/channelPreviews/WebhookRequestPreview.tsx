import type { ChannelTemplateContent } from '../../../services/api/template'

const WebhookRequestPreview: React.FC<{ content: ChannelTemplateContent }> = ({ content }) => (
  <div className="overflow-hidden rounded-lg border border-slate-700 bg-slate-950 text-slate-100 shadow">
    <div className="border-b border-slate-700 px-4 py-3 font-mono text-sm">POST /channel-delivery</div>
    <div className="space-y-1 border-b border-slate-700 px-4 py-3 font-mono text-xs text-sky-300">
      <div>Content-Type: {content.webhook?.content_type || 'application/json'}</div>
      <div>X-Yaoguang-Timestamp: 1788163200</div>
      <div>X-Yaoguang-Nonce: preview-nonce</div>
      <div>X-Yaoguang-Signature: v1=••••••••</div>
    </div>
    <pre className="overflow-auto whitespace-pre-wrap break-all p-4 text-xs">{content.webhook?.body || '{}'}</pre>
  </div>
)

export default WebhookRequestPreview

