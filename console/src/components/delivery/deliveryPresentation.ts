import type { DeliveryIntent, DeliveryStatus } from '../../services/api/delivery'

export const deliveryStatusLabels: Record<DeliveryStatus, string> = {
  planned: 'Planned',
  reserved: 'Reserved',
  queued: 'Queued',
  submitting: 'Submitting to provider',
  provider_accepted: 'Accepted by provider',
  confirmed: 'Confirmed',
  suppressed: 'Suppressed',
  deferred: 'Deferred',
  transient_failed: 'Will retry',
  terminal_failed: 'Failed permanently',
  unknown: 'Needs confirmation',
  cancelled: 'Cancelled'
}

export function deliveryExplanation(intent: DeliveryIntent): string {
  if (intent.status === 'suppressed') {
    switch (intent.suppression_reason) {
      case 'frequency_policy':
        return 'Blocked by message frequency policy'
      case 'consent':
      case 'consent_not_granted':
        return 'Blocked because marketing consent is unavailable'
      case 'identity_missing':
        return 'No usable identity exists for this channel'
      default:
        return intent.suppression_reason
          ? `Suppressed: ${intent.suppression_reason}`
          : 'Suppressed before provider submission'
    }
  }
  if (intent.status === 'unknown') {
    return 'The provider result is uncertain. Verify it before retrying or changing the outcome.'
  }
  if (intent.status === 'terminal_failed') {
    return 'Delivery failed permanently. Review the latest attempt for a fix.'
  }
  if (intent.status === 'deferred') return 'Delivery is waiting for the next allowed send time.'
  return deliveryStatusLabels[intent.status]
}
