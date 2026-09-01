import type { CSSProperties, HTMLAttributes, ReactNode } from 'react'

interface WorkspacePageTitleProps extends Omit<HTMLAttributes<HTMLHeadingElement>, 'children'> {
  children: ReactNode
}

const baseStyle: CSSProperties = {
  margin: 0,
  fontSize: '24px',
  fontWeight: 500,
  lineHeight: 1.3
}

export function WorkspacePageTitle({
  children,
  style,
  ...props
}: WorkspacePageTitleProps) {
  return (
    <h1 {...props} style={{ ...baseStyle, ...style }}>
      {children}
    </h1>
  )
}
