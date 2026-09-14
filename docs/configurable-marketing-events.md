# Business event occurrence protocol

The event code describes the behavior. The report ID identifies one occurrence. Three genuine logins by one customer produce three report IDs and three ledger facts; retrying any of those requests reuses its original report ID. The same rule applies to repeated borrowing on one customer or multiple occurrences on one configured object.

## Authenticated backend ingestion

POST /api/events.track with the normal server-side bearer credential:

```json
{"workspace_id":"workspace-id","source":"erp","event":{"report_id":"contract-sign-1","event_code":"contract.signed","customer_attributes":{"external_user_id":"customer-1"},"data":{"contract_id":"C-1"},"triggered_at":"2026-09-07T10:00:00+08:00","expires_at":"2026-09-08T10:00:00+08:00"}}
```

The configured event controls data schema, expiry mode, unmatched customer behavior and optional object association. Platform-generated reported_at is immutable on retry. Event mappings use settings.fields paths such as $.request.id and settings.attribute_mappings actions lookup, bind, update and record. No plaintext email is required: the resolved Customer UUID remains authoritative.

## Built-in event producers

In Settings → Customer and event configuration → Event management, built-in event details open **Trigger and payload**. The page shows the producer, triggering operation, related entry points, actual internal fields and copyable examples. Object fields use expandable child rows, such as `data → changes → bounced_at → old / new`, rather than flattened paths. The list also distinguishes the definition origin from its trigger source.

Built-in events are recorded by platform producers. Do not submit their internal envelopes to `events.track` or send them as provider webhook bodies. The main paths are:

| Event family | Trigger and prerequisites |
| --- | --- |
| `contact.created`, `contact.updated` | Contact-record inserts or business-field changes from console, import or API operations. Creation has empty `changes`, not a customer snapshot. Profile and tag activity has separate timeline kinds. |
| `email.sent` | Message-history creation in the send flow; some paths insert before calling the provider, so this is not delivery confirmation. |
| `email.bounced`, `email.complained`, `email.delivered` | Provider callbacks through `/webhooks/email`, or authorized normalized receipts through `/api/deliveryReceipts.ingest`, update matched message history. The email callback classifies hard bounces; soft bounces do not automatically generate the same event. |
| `email.opened`, `email.clicked` | Tracking image/link requests or authorized normalized receipts record the first timestamp. Tracking depends on client behavior and reachable URLs. |
| `email.unsubscribed` | A subscription operation linked to a message updates its unsubscribe timestamp; removing list membership alone is insufficient. |
| Channel events (`sms`, `push`, `whatsapp`, `webhook`) | Sending integrations and supported provider/client receipts. A default catalogue entry does not guarantee every integration emits that event. |
| `state.entered`, `state.exited` | State projection after eligible changes; read-only inspection does not emit transitions. Emission has a five-minute deadline and recursion protection. |

The email-associated timeline currently records delivery/failure changes as `channel.updated`; the no-email customer-message path uses `channel.delivered` / `channel.failed`. Detail examples show this naming difference explicitly. No-email message payloads currently contain only `data.changes.message_id`; the email-associated path contains changed fields with `old`/`new` values. Bounce diagnostics remain in the inbound callback record, rather than the `email.bounced` timeline payload.

Timeline event-bus envelopes wrap the source operation, entity and changes under `data`. The event ledger exposes those changes as `properties`. State transitions instead have a flat `data` payload containing the state key, current/previous value, revision and trigger deadline. These formats must not be interchanged with public ingestion requests.

**Declared extension fields** displays the stored schema separately. Adding an optional field such as a device fingerprint declares it, but does not collect its value or modify a built-in producer. The relevant SDK, provider adapter or internal producer must supply the value.

Producer references: `internal/database/init.go` (`track_message_history_changes`), `internal/database/schema/customer_activity.go`, `customer_channels.go`, `realtime.go` (`TimelineEventBridgeFunction`), and `internal/repository/subject_state_transitions.go`. Keep `console/src/components/business/builtin-event-reference.ts` aligned when these contracts change.

## Browser SDK

Publish the event with settings.browser_enabled=true, settings.authoritative=false and an explicit settings.allowed_browser_origins array. Builtin events always reject public browser reporting.

```javascript
await NotifuseAnalytics.trackEvent({event_code:'login',data:{method:'password'}});
await NotifuseAnalytics.trackEvent({event_code:'login',data:{method:'password'}});
```

These calls create distinct report IDs before queueing. The queue freezes data and triggered_at; network retries preserve both. A supplied report_id must be unique per real occurrence. Call flushEvents() when explicitly retrying a retained queue. No private API key belongs in the SDK configuration.

To identify the visitor, first obtain getSessionId(). Your authenticated application backend verifies its login session, resolves the Customer UUID, and calls POST /api/browserEvents.issueCredential with workspace_id, customer_id, session_id, exact origin and permitted event_codes. Return only the resulting short-lived credential to this browser session:

```javascript
await NotifuseAnalytics.identifyCustomer(credentialFromYourBackend);
```

The collector verifies origin/session/event scope and expiry, and links eligible anonymous historical occurrences for that session. Historical linkage does not replay marketing triggers. The browser credential lives only in memory; refresh it after 15 minutes and call identifyCustomer again. clearIdentity() stops using it. Existing identify(email,hmac) and trackGoal remain available with their original protocol.

Outbound v2 nf_id touchpoint tokens are forwarded separately. They prove a delivery touchpoint, never the identity of whoever opened or forwarded the link. A server-issued browser credential provides customer identity independently.

## Read model

GET /api/events.list?workspace_id=...&customer_id=... returns the latest occurrences, including repeated codes. GET /api/businessObjects.list returns arbitrary configured object types. CustomerBusinessActivity shows occurrence times, expiry/status, report ID, definition/mapping revisions and business data. Configured sensitive attribute keys are redacted for readers without workspace-write permission.

## Pending identity and erasure

POST /api/events.resolvePending requires contacts-write and customers-write. Supply workspace_id, source, report_id, a reason, and optionally a verified customer_id. Resolution uses the original event, mapping, attribute and object-definition snapshots. It retains the internal event UUID, report ID and both timestamps, checks expiry at resolution time, and releases an eligible occurrence downstream exactly once. Expired and backfill occurrences remain historical. A retry cannot silently apply a newer mapping.

The transaction checks expiry before customer mapping actions. Historical reports may locate an existing Customer, but do not create customers, bind identities/aliases, or update current profiles and object attributes. An unseen historical object key receives a stable identity anchor with empty current attributes, preserving its object-level history. A pending report that expires before resolution follows the same rule. The authenticated resolvePending endpoint also links an unassociated expired/backfill receipt to a verified existing Customer; it preserves original payload and times and never releases historical marketing.

GET /api/events.diagnostics?workspace_id=...&source=... returns recent ingestion errors without raw request bodies. Deleting a Customer authority row clears business receipts, ledger/outbox facts, state projections and linked anonymous sessions transactionally. Hashed erasure markers reject stale report retries and erased-session reidentification; contact endpoint deletion alone is not customer erasure.

The SDK persists a separate bounded occurrence queue per browser tab and shared session. A queued occurrence associated with a customer waits for that same customer's refreshed credential; logging into a different account does not reassign it.
