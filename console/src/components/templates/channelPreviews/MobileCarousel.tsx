import { useCallback, useEffect, useRef, useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import type { ChannelTemplateContent } from '../../../services/api/template'
import MessageContent from './MessageContent'

export default function MobileCarousel({ cards, direction }: { cards: NonNullable<ChannelTemplateContent['cards']>; direction: 'ltr' | 'rtl' }) {
  const { t } = useLingui()
  const rail = useRef<HTMLDivElement>(null)
  const [index, setIndex] = useState(0)
  const activeIndex = Math.min(index, cards.length - 1)
  const syncIndex = useCallback(() => {
    const element = rail.current
    if (!element || !element.clientWidth) return
    const frame = element.getBoundingClientRect()
    const distances = Array.from(element.children).map(card => {
      const bounds = card.getBoundingClientRect()
      return Math.abs(direction === 'rtl' ? frame.right - 16 - bounds.right : bounds.left - frame.left - 16)
    })
    setIndex(distances.indexOf(Math.min(...distances)))
  }, [direction])
  useEffect(() => {
    const observer = new ResizeObserver(syncIndex)
    if (rail.current) observer.observe(rail.current)
    syncIndex()
    return () => observer.disconnect()
  }, [cards, syncIndex])

  const select = (next: number) => {
    const element = rail.current
    const target = element?.children[next]
    if (!element || !target) return
    const frame = element.getBoundingClientRect()
    const bounds = target.getBoundingClientRect()
    element.scrollBy({
      left: direction === 'rtl' ? bounds.right - frame.right + 16 : bounds.left - frame.left - 16,
      behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth'
    })
    setIndex(next)
  }

  return <div className="material-mobile-carousel">
    <div ref={rail} className="material-mobile-carousel__rail" role="region" aria-label={t`Message cards`} tabIndex={0} onScroll={syncIndex} onKeyDown={event => {
      if (event.target !== event.currentTarget) return
      const step = direction === 'rtl' ? -1 : 1
      const next = event.key === 'ArrowRight' ? activeIndex + step : event.key === 'ArrowLeft' ? activeIndex - step : event.key === 'Home' ? 0 : event.key === 'End' ? cards.length - 1 : undefined
      if (next === undefined) return
      event.preventDefault()
      select(Math.max(0, Math.min(cards.length - 1, next)))
    }}>
      {cards.map((card, cardIndex) => <article className="material-mobile-carousel__card" key={cardIndex} aria-label={`${cardIndex + 1} / ${cards.length}`}>
        <MessageContent content={{ ...card, family: 'rich_card' }} direction={direction} mobileMarketing />
      </article>)}
    </div>
    {cards.length > 1 && <div className="material-mobile-carousel__navigation">
      <span className="material-mobile-carousel__hint">{t`Swipe to explore`}</span>
      <div className="material-mobile-carousel__dots">
        {cards.map((_, cardIndex) => {
          const number = cardIndex + 1
          return <button key={cardIndex} type="button" aria-label={t`Show card ${number}`} aria-current={activeIndex === cardIndex ? 'true' : undefined} onClick={() => select(cardIndex)}><span /></button>
        })}
      </div>
      <span className="material-mobile-carousel__count" aria-live="polite" aria-atomic="true">{activeIndex + 1} / {cards.length}</span>
    </div>}
  </div>
}
