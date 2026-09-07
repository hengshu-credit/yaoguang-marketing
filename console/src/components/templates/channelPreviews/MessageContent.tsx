import { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { ImageOff } from 'lucide-react'
import type { ChannelAction, ChannelMedia, ChannelTemplateContent } from '../../../services/api/template'

function Media({ media }: { media: ChannelMedia }) {
  const { t } = useLingui()
  const [failedUrl, setFailedUrl] = useState<string | null>(null)
  if (failedUrl === media.url) return <div className="material-message__media-error" role="status"><ImageOff size={18} />{t`Image unavailable`}</div>
  if (media.type === 'video') return <video className="material-message__media" src={media.url} controls preload="none" onError={() => setFailedUrl(media.url)} />
  if (media.type === 'audio') return <audio className="material-message__media" src={media.url} controls preload="none" onError={() => setFailedUrl(media.url)} />
  if (media.type === 'file') return <a className="material-message__text" href={media.url} target="_blank" rel="noopener noreferrer">{media.alt_text || t`Attachments`}</a>
  return <img className="material-message__media" src={media.url} alt={media.alt_text || ''} onError={() => setFailedUrl(media.url)} />
}

function Actions({ actions }: { actions?: ChannelAction[] }) {
  if (!actions?.length) return null
  return <div className="material-message__actions">{actions.map((action, index) =>
    action.type === 'url' || action.type === 'deep_link' ? (
      <a key={index} href={action.value} target="_blank" rel="noopener noreferrer">{action.label}</a>
    ) : <button key={index} type="button">{action.label}</button>
  )}</div>
}

export default function MessageContent({ content, direction }: { content: ChannelTemplateContent; direction: 'ltr' | 'rtl' }) {
  return <div className="material-message" dir={direction}>
    {content.media?.url && <Media media={content.media} />}
    {(content.title || content.body || content.footer || content.external_template) && <div className="material-message__text">
      {content.title && <strong>{content.title}</strong>}
      {content.body && <p dir={direction}>{content.body}</p>}
      {content.footer && <small>{content.footer}</small>}
      {content.external_template && <><strong>{content.external_template.id}</strong><small>{content.external_template.language}</small>{content.external_template.parameters?.map((parameter, index) => <p key={index}>{parameter.name}: {parameter.value}</p>)}</>}
    </div>}
    {content.cards?.length ? <div className="material-message__cards" tabIndex={0}>{content.cards.map((card, index) => <div className="material-message__card" key={index}>
      <MessageContent content={{ ...card, family: 'rich_card' }} direction={direction} />
    </div>)}</div> : null}
    <Actions actions={content.actions} />
  </div>
}
