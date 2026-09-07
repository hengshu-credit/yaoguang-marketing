export interface TextFitInput {
  availableWidth: number
  naturalWidth: number
  characterCount: number
  fontSize: number
}

export interface TextFitResult {
  letterSpacingPx: number
  wrap: boolean
}

const MIN_LETTER_SPACING_EM = -0.06

/**
 * Fit one translated brand line to its current container.
 *
 * Small differences are distributed evenly between characters. When the text
 * would need unreadably tight spacing, the minimum is retained and wrapping is
 * enabled instead.
 */
export function fitTextToWidth({
  availableWidth,
  naturalWidth,
  characterCount,
  fontSize
}: TextFitInput): TextFitResult {
  if (
    !Number.isFinite(availableWidth) ||
    !Number.isFinite(naturalWidth) ||
    !Number.isFinite(fontSize) ||
    availableWidth <= 0 ||
    naturalWidth <= 0 ||
    fontSize <= 0 ||
    characterCount <= 1
  ) {
    return { letterSpacingPx: 0, wrap: false }
  }

  const requestedSpacing = (availableWidth - naturalWidth) / (characterCount - 1)
  const minimumSpacing = fontSize * MIN_LETTER_SPACING_EM

  return {
    letterSpacingPx: Math.max(minimumSpacing, requestedSpacing),
    wrap: requestedSpacing < minimumSpacing
  }
}
