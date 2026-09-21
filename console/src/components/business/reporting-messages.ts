import type { I18n } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import type { ReportingIssue } from './event-reporting'

export function reportingIssueMessage(issue: ReportingIssue, i18n: I18n): string {
  switch (issue.code) {
    case 'ttl': return i18n._(msg`Enter a duration that is a whole number of seconds, between 1 second and 10 years.`)
    case 'mode': return i18n._(msg`Choose an expiration policy.`)
    case 'browser_conflict': return i18n._(msg`Choose one reporting mode. Browser reporting cannot require a trusted backend at the same time.`)
    case 'browser_object': return i18n._(msg`Browser reporting supports customer events only. Use backend reporting for business objects.`)
    case 'origins': return i18n._(msg`Add at least one exact HTTP origin, without a path, wildcard, credentials or query string.`)
    case 'sources': return i18n._(msg`Select at least one source or explicitly turn off the source restriction.`)
    case 'event_code': return i18n._(msg`Select a fixed event or map its stable event code.`)
    case 'required_mapping': return i18n._(msg`This mapping is required for activation and sample validation.`)
    case 'attribute_mapping': return i18n._(msg`Select a customer attribute and a path under the normalized $.data or $.customer_attributes object.`)
    default: return i18n._(msg`Use a literal object path such as $.data.order_id. Arrays and wildcards are not supported.`)
  }
}
