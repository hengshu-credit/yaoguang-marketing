import type { CSSProperties } from 'react'
import { BRAND } from '../../constants/brand'

export interface YaoguangBrandProps {
  compact?: boolean
  layout?: 'horizontal' | 'vertical'
  showDescription?: boolean
  className?: string
  style?: CSSProperties
}

export function YaoguangBrand({
  compact = false,
  layout = 'horizontal',
  showDescription = layout === 'vertical',
  className,
  style
}: YaoguangBrandProps) {
  const vertical = layout === 'vertical'

  return (
    <div
      className={className}
      aria-label={`${BRAND.productName}，${BRAND.tagline}`}
      style={{
        display: 'flex',
        flexDirection: vertical ? 'column' : 'row',
        alignItems: 'center',
        justifyContent: vertical ? 'center' : 'flex-start',
        gap: compact ? 0 : vertical ? 10 : 9,
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
        <div style={{ minWidth: 0, textAlign: vertical ? 'center' : 'left' }}>
          <div
            style={{
              color: '#182230',
              fontSize: vertical ? 22 : 17,
              fontWeight: 700,
              lineHeight: 1.25,
              letterSpacing: '0.02em',
              whiteSpace: 'nowrap'
            }}
          >
            {BRAND.productName}
          </div>
          <div
            style={{
              color: '#667085',
              fontSize: vertical ? 13 : 11,
              lineHeight: 1.45,
              marginTop: 2,
              whiteSpace: 'nowrap'
            }}
          >
            {BRAND.tagline}
          </div>
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
