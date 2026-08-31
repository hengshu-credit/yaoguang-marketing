import type { DeliveryStatus } from '../../services/api/delivery'

export const deliveryStatusOptions: DeliveryStatus[] = [
  'unknown', 'terminal_failed', 'transient_failed', 'suppressed', 'deferred', 'submitting',
  'provider_accepted', 'confirmed', 'queued', 'planned', 'reserved', 'cancelled'
]

export interface DeliveryFilters {
  status?: DeliveryStatus
  channel?: string
  source_type?: string
  source_id?: string
  provider?: string
  customer_id?: string
  from?: string
  to?: string
}
