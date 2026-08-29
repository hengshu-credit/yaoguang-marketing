import { createContext, useContext } from 'react'
import type { WebAnalyticsContextValue } from './context'

export const WebAnalyticsContext = createContext<WebAnalyticsContextValue | null>(null)

export function useWebAnalytics(): WebAnalyticsContextValue {
  const context = useContext(WebAnalyticsContext)
  if (!context) {
    throw new Error('useWebAnalytics must be used inside a WebAnalyticsProvider')
  }
  return context
}
