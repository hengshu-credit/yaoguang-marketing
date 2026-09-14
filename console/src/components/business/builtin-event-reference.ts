import { msg } from '@lingui/core/macro'
import type { I18n, MessageDescriptor } from '@lingui/core'

export type BuiltinEventOrigin = 'platform' | 'provider' | 'interaction' | 'receipt' | 'state'
export interface EventReferenceField { path: string; type: string; description: string; children?: EventReferenceField[] }
export interface EventReferenceVariant { label: string; fields: EventReferenceField[]; example: Record<string, unknown> }
export interface BuiltinEventReference {
  origin: BuiltinEventOrigin
  trigger: string
  setup: string
  endpoints: string[]
  notes: string[]
  variants: EventReferenceVariant[]
  receiptExample?: Record<string, unknown>
}

const originLabels: Record<BuiltinEventOrigin, MessageDescriptor> = {
  platform: msg`Platform operation`, provider: msg`Provider feedback`, interaction: msg`Recipient interaction`,
  receipt: msg`Channel or client receipt`, state: msg`State calculation`
}
export const builtinEventOriginLabel = (origin: BuiltinEventOrigin, i18n: I18n) => i18n._(originLabels[origin])

const at = '2026-09-10T08:00:00Z'
const messageId = '11111111-1111-4111-8111-111111111111'
const customerId = '22222222-2222-4222-8222-222222222222'
const eventId = '33333333-3333-4333-8333-333333333333'

function envelope(type: string, source: string, subject: Record<string, unknown>, data: Record<string, unknown>) {
  return { id: '44444444-4444-4444-8444-444444444444', event_id: eventId, type, schema_version: 1,
    workspace_id: 'your-workspace', subject, source, occurred_at: at, received_at: at, correlation_id: eventId, data }
}

export function builtinEnvelopeFields(i18n: I18n): EventReferenceField[] {
  return [
    { path: 'id', type: 'UUID', description: i18n._(msg`Internal message ID`) },
    { path: 'event_id', type: 'UUID', description: i18n._(msg`Stable event occurrence ID retained on retries`) },
    { path: 'type', type: 'string', description: i18n._(msg`Event code`) },
    { path: 'schema_version', type: 'integer', description: i18n._(msg`Internal envelope version`) },
    { path: 'workspace_id', type: 'string', description: i18n._(msg`Workspace identifier used by the producer; timeline messages use the workspace database name.`) },
    { path: 'subject', type: 'object', description: i18n._(msg`Event subject`), children: [
      { path: 'subject.type', type: 'string', description: i18n._(msg`Source record type and identifier. A timeline event can identify a message or contact record, not only a customer.`) },
      { path: 'subject.id', type: 'string', description: i18n._(msg`Source record identifier`) },
      { path: 'subject.customer_id', type: 'UUID | null', description: i18n._(msg`Associated customer identifier, when available`) },
      { path: 'subject.contact_email', type: 'string | null', description: i18n._(msg`Associated contact email, when available`) }
    ] },
    { path: 'source', type: 'string', description: i18n._(msg`Internal producer, such as contact_timeline or platform.state_projector.`) },
    { path: 'occurred_at', type: 'date-time', description: i18n._(msg`Business occurrence time in ISO 8601 format`) },
    { path: 'received_at', type: 'date-time', description: i18n._(msg`Platform receipt time in ISO 8601 format`) },
    { path: 'correlation_id', type: 'UUID', description: i18n._(msg`Correlation ID connecting related events`) },
    { path: 'causation_id', type: 'UUID | null', description: i18n._(msg`Optional cause event ID`) },
    { path: 'data', type: 'object', description: i18n._(msg`Producer-specific payload shown below. This is not the public events.track request envelope.`) }
  ]
}

const changeDescriptions: Record<string, MessageDescriptor> = {
  template_id: msg`Template identifier`, template_version: msg`Template version`, channel: msg`Delivery channel`,
  broadcast_id: msg`Associated broadcast, if any`, sent_at: msg`Recorded send time`,
  delivered_at: msg`Recorded delivery time`, failed_at: msg`Recorded failure time`,
  opened_at: msg`First recorded open time`, clicked_at: msg`First recorded click time`,
  bounced_at: msg`Recorded hard-bounce time`, complained_at: msg`Recorded complaint time`,
  unsubscribed_at: msg`Recorded unsubscribe time`, status_info: msg`Provider or platform status detail`,
  first_name: msg`Example changed contact field; actual changed fields vary by operation.`
}

// Keep these payloads aligned with database/init.go::track_message_history_changes,
// schema/realtime.go::TimelineEventBridgeFunction and schema/customer_channels.go.
function timelineVariant(i18n: I18n, key: string, changes: Record<string, unknown>, entityType = 'message_history', withoutEmail = false): EventReferenceVariant {
  const contact = entityType === 'contact'
  const operation = key.endsWith('.created') || key.endsWith('.sent') ? 'insert' : 'update'
  const entityId = contact ? null : messageId
  const data = { operation, entity_type: entityType, entity_id: entityId, changes }
  const changedFields: EventReferenceField[] = Object.entries(changes).map(([field, value]) => {
    const path = `data.changes.${field}`
    if (field === 'message_id') return { path, type: 'UUID', description: i18n._(msg`Message history identifier; the no-email producer records only this field.`) }
    const values = value as { old?: unknown; new: unknown }
    const type = field.endsWith('_at') ? 'date-time' : field === 'template_version' ? 'integer' : 'string'
    return { path, type: 'object', description: i18n._(changeDescriptions[field]), children: Object.keys(values).map(part => ({
      path: `${path}.${part}`, type: part === 'old' || values.new === null ? `${type} | null` : type,
      description: i18n._(part === 'old' ? msg`Previous value; null when no value was recorded` : msg`New value recorded by this operation`)
    })) }
  })
  const fields: EventReferenceField[] = [{ path: 'data', type: 'object', description: i18n._(msg`Event payload`), children: [
    { path: 'data.operation', type: 'string', description: i18n._(msg`The source write operation: insert or update.`) },
    { path: 'data.entity_type', type: 'string', description: i18n._(msg`Source record type`) },
    { path: 'data.entity_id', type: 'string | null', description: i18n._(msg`Source record identifier; null for a contact record`) },
    { path: 'data.changes', type: 'object', description: i18n._(msg`Changed values from the timeline. Updates use old/new; inserts generally use new only.`), ...(changedFields.length ? { children: changedFields } : {}) }
  ] }]
  return {
    label: contact ? i18n._(msg`Contact timeline payload`) : withoutEmail ? i18n._(msg`Customer message without email`) : i18n._(msg`Message associated with an email`),
    fields,
    example: envelope(key, 'contact_timeline', { type: entityType, id: entityId ?? 'customer@example.com', customer_id: customerId, contact_email: withoutEmail ? null : 'customer@example.com' }, data)
  }
}

export function getBuiltinEventReference(key: string, i18n: I18n): BuiltinEventReference | undefined {
  const text = (message: MessageDescriptor) => i18n._(message)
  if (key === 'contact.created' || key === 'contact.updated') return {
    origin: 'platform',
    trigger: text(key === 'contact.created' ? msg`Created when a contact record is inserted, including console, import or API writes that create the contact projection.` : msg`Created when business fields on a contact record change; timestamp/version-only maintenance does not count.`),
    setup: text(msg`Perform the customer/contact operation through the console or authorized API. Do not separately report this built-in event.`),
    endpoints: [],
    notes: [text(msg`These events describe contact records. Customer profile and tag changes have their own timeline kinds; creating a customer without an email does not necessarily create contact.created.`), text(msg`A contact.created payload has empty changes rather than a full customer snapshot.`)],
    variants: [timelineVariant(i18n, key, key === 'contact.created' ? {} : { first_name: { old: 'Old name', new: 'New name' } }, 'contact')]
  }
  if (key === 'state.entered' || key === 'state.exited') {
    const data = { state_key: 'eligible', value: key === 'state.entered' ? 'true' : 'false', previous_value: key === 'state.entered' ? 'false' : 'true', rule_revision: 2, processing_status: 'accepted', expires_at: '2026-09-10T08:05:00Z', causation_depth: 1 }
    return {
      origin: 'state', trigger: text(key === 'state.entered' ? msg`The state projector detects a transition into a satisfied state.` : msg`The state projector detects a transition out of a satisfied state, to false or unknown.`),
      setup: text(msg`Activate a state rule and supply the customer attributes or business events it needs. The state projector produces transitions.`), endpoints: [],
      notes: [text(msg`Read-only state inspection does not emit transitions. Old changes outside the five-minute emission window are not re-emitted; replay and initial evaluation depend on the producer context.`)],
      variants: [{ label: text(msg`State transition payload`), example: envelope(key, 'platform.state_projector', { type: 'customer', id: customerId, customer_id: customerId }, data), fields: [{ path: 'data', type: 'object', description: text(msg`Event payload`), children: [
        { path: 'data.state_key', type: 'string', description: text(msg`Configured state key`) },
        { path: 'data.value', type: 'string', description: text(msg`Current result: true, false or unknown`) },
        { path: 'data.previous_value', type: 'string', description: text(msg`Previous result: true, false or unknown`) },
        { path: 'data.rule_revision', type: 'integer', description: text(msg`State rule revision used for the calculation.`) },
        { path: 'data.processing_status', type: 'string', description: text(msg`Processing status`) },
        { path: 'data.expires_at', type: 'date-time', description: text(msg`Five-minute transition trigger deadline`) },
        { path: 'data.causation_depth', type: 'integer', description: text(msg`Transition chain depth used to prevent recursive triggering.`) }
      ] }] }]
    }
  }
  const [channel, suffix, extra] = key.split('.')
  if (extra || !['email', 'sms', 'push', 'whatsapp', 'webhook'].includes(channel) || !['sent', 'delivered', 'opened', 'clicked', 'failed', 'bounced', 'complained', 'unsubscribed', 'updated'].includes(suffix)) return undefined
  const email = channel === 'email'
  const origin: BuiltinEventOrigin = suffix === 'sent' || suffix === 'updated' ? 'platform' : email ? ['opened', 'clicked', 'unsubscribed'].includes(suffix) ? 'interaction' : 'provider' : 'receipt'
  let trigger: string
  if (suffix === 'sent') trigger = text(msg`Created when the platform inserts message history for the send path.`)
  else if (email && suffix === 'bounced') trigger = text(msg`An email provider bounce is matched to a message and classified as a hard bounce; the platform sets bounced_at and records the event.`)
  else if (email && suffix === 'complained') trigger = text(msg`A provider complaint is matched to a message; the platform sets complained_at and records the event.`)
  else if (email && suffix === 'opened') trigger = text(msg`Loading the tracking image or ingesting an authorized open receipt sets the first open timestamp.`)
  else if (email && suffix === 'clicked') trigger = text(msg`Following a tracked link or ingesting an authorized click receipt sets the first click timestamp.`)
  else if (suffix === 'unsubscribed') trigger = text(msg`A subscription operation linked to a message sets unsubscribed_at. Removing a list membership alone is not the same event.`)
  else if (suffix === 'updated') trigger = text(msg`A message delivery time, failure time or status detail changes on the email-associated timeline path.`)
  else trigger = text(msg`A supported provider/client receipt or send result updates the corresponding message status. A catalogue entry alone does not produce a receipt.`)
  const endpoints = email && ['bounced', 'complained', 'delivered'].includes(suffix)
    ? ['POST /webhooks/email?provider=…&workspace_id=…&integration_id=…', 'POST /api/deliveryReceipts.ingest']
    : email && suffix === 'opened' ? ['GET /t/{encrypted-token}', 'GET /opens (legacy)', 'POST /api/deliveryReceipts.ingest']
      : email && suffix === 'clicked' ? ['GET /r/{encrypted-token}', 'GET /visit (legacy)', 'POST /api/deliveryReceipts.ingest']
        : origin === 'receipt' && suffix !== 'unsubscribed' ? [...(channel === 'sms' ? ['POST /webhooks/delivery/twilio'] : []), 'POST /webhooks/delivery/channel', 'POST /api/deliveryReceipts.ingest'] : []
  const notes: string[] = []
  if (suffix === 'sent') notes.push(text(msg`Sent is not delivery confirmation. Some email paths create message history before calling the provider, so later failure is possible.`))
  if (email && suffix === 'bounced') notes.push(text(msg`Bounce type/category/diagnostic details belong to the inbound callback record. This timeline payload contains the bounced_at change; soft bounces do not automatically produce this hard-bounce event.`))
  if (email && ['opened', 'clicked'].includes(suffix)) notes.push(text(msg`Tracking must be enabled and its URLs reachable. Image blocking, proxying or automated link checks can affect these signals; repeated activity may update counters without another timestamp event.`))
  if (!email) notes.push(text(msg`Supported feedback varies by channel and integration. Open, click, complaint or unsubscribe events must not be inferred from a successful send.`))
  const genericUpdate = suffix === 'delivered' || suffix === 'failed'
  if (genericUpdate) notes.push(text(msg`Current producer difference: messages associated with an email record delivered_at/failed_at as channel.updated. The no-email customer-message path emits channel.delivered/channel.failed. This default catalogue code is not emitted by every path.`))
  if (!email && ['bounced', 'complained', 'unsubscribed', 'updated'].includes(suffix)) notes.push(text(msg`The current no-email customer-message trigger does not emit this suffix. It requires an email-associated status-write path or an additional producer implementation.`))
  if (suffix === 'unsubscribed') notes.push(text(msg`The normalized delivery receipt API does not accept unsubscribed as a receipt event.`))
  const changes = suffix === 'sent'
    ? { template_id: { new: 'welcome' }, template_version: { new: 1 }, channel: { new: channel }, broadcast_id: { new: null }, sent_at: { new: at } }
    : suffix === 'updated' ? { delivered_at: { old: null, new: at }, status_info: { old: null, new: 'Provider status detail' } }
      : { [`${suffix}_at`]: { old: null, new: at } }
  const variants = [timelineVariant(i18n, genericUpdate ? `${channel}.updated` : key, changes)]
  if (!email && ['sent', 'delivered', 'failed', 'opened', 'clicked'].includes(suffix)) variants.push(timelineVariant(i18n, key, { message_id: messageId }, 'message_history', true))
  return {
    origin, trigger, endpoints, notes, variants,
    setup: text(email && ['bounced', 'complained', 'delivered'].includes(suffix) ? msg`Configure the email provider callback in Settings → Integrations. Provider-specific webhook bodies are normalized by the platform; use the callback URL supplied by the integration.` : origin === 'receipt' ? msg`Configure the sending integration and its receipt/feedback path. Normalized receipt ingestion requires authorization and a real message identifier.` : msg`Use the platform send, tracking or subscription flow. These internal events must not be posted to events.track.`),
    ...(endpoints.includes('POST /api/deliveryReceipts.ingest') && suffix !== 'unsubscribed' ? { receiptExample: { workspace_id: 'your-workspace', receipts: [{ provider: email ? 'ses' : channel === 'sms' ? 'twilio' : channel === 'push' ? 'fcm' : 'channel_webhook', receipt_id: 'provider-receipt-001', message_id: messageId, event: suffix, occurred_at: at }] } } : {})
  }
}
