import React from 'react'
import { TiptapRichEditor } from './TiptapRichEditor'
import { TiptapInlineEditor } from './TiptapInlineEditor'
import type { BaseTiptapProps } from './shared/types'

// Extended props interface that includes the inline prop for backward compatibility
export interface TiptapComponentProps extends BaseTiptapProps {
  inline?: boolean // Determines which editor variant to use
}

/**
 * Backward-compatible TiptapComponent that automatically chooses between
 * TiptapRichEditor and TiptapInlineEditor based on the inline prop.
 *
 * @deprecated Consider using TiptapRichEditor or TiptapInlineEditor directly for better type safety and performance.
 */
export const TiptapComponent: React.FC<TiptapComponentProps> = ({ inline = false, ...props }) => {
  // Choose the appropriate editor based on the inline prop
  if (inline) {
    return <TiptapInlineEditor {...props} />
  }

  return <TiptapRichEditor {...props} />
}

export default TiptapComponent
