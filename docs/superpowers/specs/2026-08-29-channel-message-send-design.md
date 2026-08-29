# Idempotent SMS and push send design

## Outcome

Notifuse has one delivery contract for direct API calls and realtime automation nodes. It uses the existing saved SMS/push templates, workspace translations, encrypted contact endpoints, encrypted provider credentials, message history and receipt projection instead of creating a parallel campaign subsystem.

## Request and authorization

`POST /api/channelMessages.send` requires Transactional write. A request identifies the workspace, `sms` or `push` channel, provider integration, contact email, optional endpoint, saved template/version, optional language/data/metadata and a caller-owned `effect_key` of at most 255 characters.

The caller must create the contact and register an endpoint first through `/api/ingest.batch`. SMS endpoints use `channel=sms`, `provider=twilio`, `platform=phone` and an E.164 address. Push currently delivers through active FCM endpoints. Addresses remain AES-GCM encrypted in `contact_endpoints` and are excluded from public JSON and timeline events.

## Rendering and endpoint resolution

The service loads the contact, saved template and workspace. It selects the named endpoint or the most recently active endpoint for the channel. Explicit language wins, then endpoint locale, then contact language, then workspace default. A BCP-47 endpoint locale such as `en-US` can fall back to its supported base language `en`.

`BuildTemplateData` supplies the complete contact, workspace URLs and caller data. `TemplateService.PreviewTemplate` is deliberately reused as the server rendering kernel, so draft preview and actual delivery use the same Liquid processing and SMS/push validation.

## Concurrency and external-effect safety

`channel_send_executions.effect_key` is the authority key. Before calling a provider, an insert reserves the key with a SHA-256 hash of the effective request, deterministic message ID, resolved endpoint, template version and language. PostgreSQL `ON CONFLICT` gives these outcomes:

- same key and same request: return the stored execution without another provider call;
- same key and different request: reject with a payload conflict;
- new key: transition `reserved -> submitted` and call the provider once.

Provider acceptance transitions `submitted -> confirmed` and inserts encrypted `message_history` in the same database transaction. A provider-declared rejection becomes `failed`. A connection failure after request dispatch, an unreadable/malformed HTTP success response, or a local confirmation failure after provider acceptance becomes `unknown`; unknown is not automatically retried because acceptance may already have happened.

This is effect-once behavior under retries, not impossible distributed exactly-once delivery. An operator reconciliation flow is still required before an `unknown` execution can be deliberately retried.

## Realtime journeys

The low-code canvas exposes `sms` and `push` nodes with template, integration, optional endpoint and language fields. Realtime planning treats them as external side effects. The journey worker passes the durable realtime effect key into the same channel service, so RabbitMQ redelivery and worker restarts converge on the existing send execution rather than issuing another provider request.

## Delivery status

Twilio and trusted provider callbacks continue through the v44 receipt ledger. `message_history.external_id` contains the provider message ID, allowing delivered/opened/failed receipts to find and project onto the confirmed send. Receipt idempotency remains independent from send idempotency.

## Current boundaries

- FCM is the implemented push sender; APNs and Web Push endpoint storage exists but native send adapters are future work.
- Direct send selects one endpoint. Multi-device fan-out should create one child effect key per endpoint rather than sharing a single key across multiple external effects.
- Frequency caps, quiet hours, consent policies and an operator UI for reconciling `unknown` sends should be added before large-scale promotional traffic.
