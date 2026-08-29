# Omnichannel endpoint and content design

Date: 2026-08-29
Status: approved for implementation on `dev`

## Outcome

Notifuse will model a marketing identity (contact), its reachable client installations
(endpoints), localized channel content, and delivery providers as separate layers. This
keeps external profile ingestion idempotent while allowing one contact to receive a push
on several devices and an email or SMS through a different address.

## Decisions

### Contact endpoints

Workspace databases gain a `contact_endpoints` table. A row is identified by the
external system's stable `endpoint_id` and belongs to a contact email. The first supported
channel is `push`, with providers `fcm`, `apns`, and `webpush` and platforms `android`,
`ios`, and `web`.

The endpoint stores provider address material encrypted with the application secret. A
SHA-256 fingerprint supports safe equality and uniqueness checks without making the
address readable in SQL. Public reads expose endpoint metadata but never the plaintext
address or ciphertext.

An endpoint is disabled instead of deleted when a client logs out, revokes permission, or
a provider reports an invalid token. Re-registering the same endpoint is an idempotent
upsert that increments its version only when values change.

### External ingest contract

`POST /api/ingest.batch` accepts an optional `endpoints` array on each contact item.
Each mutation is either:

- `upsert`: requires endpoint id, push provider, platform, and address; accepts locale,
  timezone, app id, device id, and JSON attributes.
- `disable`: requires only endpoint id; preserves the encrypted address and metadata for
  audit and later reactivation.

Endpoint validation is performed before the contact bulk upsert. A contact item is
reported as failed if any of its endpoint mutations fails. Retrying is safe because both
contact and endpoint writes are idempotent.

### Realtime propagation

Insert, meaningful update, and disable changes append semantic rows to
`contact_timeline`. The existing v40 timeline bridge writes those rows to the event
ledger and transactional outbox in the same database commit. The rule and journey
workers can therefore react to `contact.endpoint_registered`,
`contact.endpoint_updated`, and `contact.endpoint_disabled` without a second event path.

### Localized content and preview

The next slice extends templates with `sms` and `push` content variants while retaining
the existing email/web model. A push variant contains title, body, optional image URL,
deep link, and custom data. An SMS variant contains body and optional sender id.
Translations use the existing language-keyed map and fallback rules.

Preview is server-rendered from Liquid test data into a channel-neutral response. Email
returns HTML/text, SMS returns text plus encoding/segment estimates, and push returns a
normalized notification payload plus iOS, Android, and Web presentation hints. Provider
adapters consume the normalized payload and do not own template rendering.

## Concurrency and safety

- Endpoint writes use one `INSERT ... ON CONFLICT ... DO UPDATE` statement.
- A partial index serves active endpoint selection by email/channel/provider.
- Ciphertext and address fingerprints are never returned by public list APIs or timeline
  changes.
- Batch and in-flight limits remain enforced by the existing ingest service.
- Provider-specific retry, rate limiting, invalid-token disabling, and delivery receipts
  are implemented behind the delivery worker, not in the synchronous ingest request.

## Delivery order

1. v42 schema, domain validation, encrypted repository, ingest wiring, and tests.
2. Endpoint metadata list API and contact-console visibility.
3. SMS/push template types, translations, validation, and preview API.
4. FCM HTTP v1, APNs HTTP/2, and Web Push adapters with provider credentials.
5. Delivery receipts, token invalidation, channel preference/consent policy, and metrics.

## Explicit non-goals for v42

v42 does not send notifications and does not store provider credentials. It creates the
durable, secure address book and realtime events required by later provider adapters.
