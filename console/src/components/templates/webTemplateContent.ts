import type { WebTemplate } from '../../services/api/template'
import { jsonToHtml } from '../blog/utils'

export function webTemplateHtml(web?: WebTemplate): string {
  if (web?.html) return web.html
  const content = web?.content
  if (content && typeof content === 'object' && 'type' in content && content.type === 'doc') {
    return jsonToHtml(content as Parameters<typeof jsonToHtml>[0])
  }
  return ''
}

/** A saved web fragment needs its own document styles inside the sandbox. */
export function webPreviewDocument(html: string): string {
  if (/<(?:!doctype|html)[\s>]/i.test(html)) return html
  return `<!doctype html><html><head><meta name="viewport" content="width=device-width, initial-scale=1" /><style>
    body { margin:0; padding:24px; font:14px/1.7 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; color:#243247; overflow-wrap:anywhere; }
    * { box-sizing:border-box; } img,video,iframe { max-width:100%; height:auto; } img { display:block; }
    pre { white-space:pre-wrap; } table { max-width:100%; } a { color:#2464bd; } blockquote { margin-inline:0; padding-inline:16px; border-inline-start:3px solid #dce3ed; }
  </style></head><body>${html}</body></html>`
}
