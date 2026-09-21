import type { CSSProperties, HTMLAttributes, ReactNode } from 'react'
import { useLayoutEffect, useRef } from 'react'
import { useConsoleTheme } from '../../theme/ConsoleThemeProvider'

interface WorkspacePageTitleProps extends Omit<HTMLAttributes<HTMLHeadingElement>, 'children'> {
  children: ReactNode
}

const baseStyle: CSSProperties = {
  margin: 0,
  fontSize: 'var(--console-title-size, 22px)',
  fontWeight: 600,
  lineHeight: 1.4,
  color: 'var(--console-text, #172033)'
}

export function WorkspacePageTitle({
  children,
  style,
  className,
  ...props
}: WorkspacePageTitleProps) {
  const node = useRef<HTMLHeadingElement>(null)
  const lastTitle = useRef('')
  const release = useRef<(() => void)>()
  const register = useConsoleTheme()?.title
  useLayoutEffect(() => {
    const text = node.current?.textContent?.trim() ?? ''
    if (text !== lastTitle.current) { lastTitle.current = text; release.current?.(); release.current = register?.(text) }
  }, [children, register])
  useLayoutEffect(() => () => { release.current?.(); lastTitle.current = '' }, [])
  return (
    <h1 {...props} ref={node} className={`workspace-page-title${className ? ` ${className}` : ''}`} style={{ ...baseStyle, ...style }}>
      {children}
    </h1>
  )
}
