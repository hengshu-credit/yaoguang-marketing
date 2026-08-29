import type { WebAnalyticsTab } from './lib/types'

/**
 * The filters and annotations tabs configure rather than read analytics data,
 * so the assistant has no visible report context there. Both are independently
 * excluded from NAVIGABLE_TABS and the tests cross-check the two rules.
 */
export function shouldHideAssistant(tab: WebAnalyticsTab): boolean {
  return tab === 'filters' || tab === 'annotations'
}
