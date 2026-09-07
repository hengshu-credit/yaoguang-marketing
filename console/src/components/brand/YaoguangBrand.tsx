import { useLayoutEffect, useRef } from 'react'
import type { CSSProperties } from 'react'
import { useLingui } from '@lingui/react/macro'
import { BRAND } from '../../constants/brand'
import { fitTextToWidth } from './brandTextFit'

export interface YaoguangBrandProps {
  compact?: boolean
  layout?: 'horizontal' | 'vertical'
  showDescription?: boolean
  className?: string
  style?: CSSProperties
}

interface FittedBrandLineProps {
  text: string
  vertical: boolean
  color: string
  fontSize: number
  fontWeight?: number
  lineHeight: number
  marginTop?: number
  verticalLetterSpacing?: string
}

function FittedBrandLine({
  text,
  vertical,
  color,
  fontSize,
  fontWeight,
  lineHeight,
  marginTop,
  verticalLetterSpacing
}: FittedBrandLineProps) {
  const lineRef = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    if (vertical) return
    const line = lineRef.current
    if (!line) return

    let active = true
    const fit = () => {
      if (!active || !line.clientWidth) return
      const context = document.createElement('canvas').getContext('2d')
      if (!context) return
      context.font = window.getComputedStyle(line).font
      const result = fitTextToWidth({
        availableWidth: line.clientWidth,
        naturalWidth: context.measureText(text).width,
        characterCount: Array.from(text).length,
        fontSize
      })
      line.style.letterSpacing = `${result.letterSpacingPx}px`
      line.style.whiteSpace = result.wrap ? 'normal' : 'nowrap'
      line.style.overflowWrap = result.wrap ? 'anywhere' : 'normal'
    }

    fit()
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(fit)
    observer?.observe(line)
    void document.fonts?.ready.then(fit)
    window.addEventListener('resize', fit)
    return () => {
      active = false
      observer?.disconnect()
      window.removeEventListener('resize', fit)
    }
  }, [fontSize, text, vertical])

  return (
    <div
      ref={lineRef}
      style={{
        width: vertical ? undefined : '100%',
        color,
        fontSize,
        fontWeight,
        lineHeight,
        marginTop,
        letterSpacing: vertical ? verticalLetterSpacing : 0,
        whiteSpace: 'nowrap',
        overflowWrap: 'normal'
      }}
    >
      {text}
    </div>
  )
}

export function YaoguangBrand({
  compact = false,
  layout = 'horizontal',
  showDescription = layout === 'vertical',
  className,
  style
}: YaoguangBrandProps) {
  const { t } = useLingui()
  const vertical = layout === 'vertical'
  const productName = t`Alkaid Marketing Platform`
  const tagline = t`Understanding Intent. Reaching Customers`

  return (
    <div
      className={className}
      aria-label={`${productName}, ${tagline}`}
      style={{
        display: 'flex',
        flexDirection: vertical ? 'column' : 'row',
        alignItems: 'center',
        justifyContent: vertical ? 'center' : 'flex-start',
        gap: compact ? 0 : vertical ? 10 : 7,
        minWidth: 0,
        ...style
      }}
    >
      <img
        src={BRAND.logoPath}
        alt={BRAND.companyName}
        style={{
          display: 'block',
          width: compact ? 34 : vertical ? 58 : 42,
          height: compact ? 34 : vertical ? 58 : 42,
          objectFit: 'contain',
          flex: '0 0 auto'
        }}
      />
      {!compact && (
        <div
          style={{
            minWidth: 0,
            flex: vertical ? '0 1 auto' : '1 1 auto',
            maxWidth: vertical ? undefined : 'calc(100% - 49px)',
            textAlign: vertical ? 'center' : 'left',
            overflow: vertical ? undefined : 'hidden'
          }}
        >
          <FittedBrandLine
            text={productName}
            vertical={vertical}
            color="#182230"
            fontSize={vertical ? 22 : 13}
            fontWeight={700}
            lineHeight={1.25}
            verticalLetterSpacing="0.02em"
          />
          <FittedBrandLine
            text={tagline}
            vertical={vertical}
            color="#667085"
            fontSize={vertical ? 13 : 9}
            lineHeight={vertical ? 1.45 : 1.25}
            marginTop={2}
          />
          {showDescription && (
            <div
              style={{
                color: '#667085',
                fontSize: 12,
                lineHeight: 1.6,
                marginTop: 10,
                maxWidth: 360
              }}
            >
              {BRAND.description}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
