# Native channel providers and delivery receipts (v44)

## Goal

Introduce a provider-neutral delivery boundary with production adapters for Twilio SMS and Firebase Cloud Messaging HTTP v1, workspace-scoped encrypted credentials, and an idempotent delivery-receipt ledger. This phase deliberately keeps provider HTTP and callback semantics out of automation and template code so the next phase can connect journeys, broadcasts and transactional sends to one stable interface.

## Decisions

- Provider configuration remains part of a workspace integration. `sms` integrations contain a Twilio provider; `push` integrations contain an FCM provider. Integration validation, encryption, redaction, credential hints and preserve-on-update behavior follow the existing email/LLM patterns.
- Twilio supports an Account SID plus Auth Token and optionally an API Key SID/secret. Sending prefers the restricted API key pair when configured. The Auth Token remains required because Twilio status callbacks are signed with it. A sender must configure exactly one of a From number or Messaging Service SID.
- FCM uses HTTP v1 with a service-account JSON credential. The JSON is encrypted as one secret, is never returned by workspace APIs, and obtains short-lived OAuth tokens for the `firebase.messaging` scope.
- Provider adapters implement a small `ChannelProvider` contract. Inputs contain already-rendered channel content and a stable idempotency/effect key; outputs contain the provider message id and accepted status. Adapters use bounded HTTP clients, cap response bodies, classify permanent 4xx failures separately from retryable 429/5xx/network failures, and never log credentials or endpoint addresses.
- Provider acceptance is not handset delivery. Every normalized callback or trusted-system receipt is appended to `delivery_receipts`. `(provider, receipt_id)` is the duplicate boundary and `payload_hash` detects an id reused with a different meaning.
- Twilio callbacks are form-encoded, signature-verified against the exact configured public callback URL, and normalized from `MessageSid`, `MessageStatus`, `ErrorCode` and `RawDlrDoneDate`. Unknown fields are tolerated because Twilio may add fields.
- FCM HTTP v1 has no per-device delivered callback. This phase records FCM acceptance from the send response and exposes authenticated receipt ingestion for trusted mobile/backend SDKs. A later client SDK will use per-message signed receipt tokens rather than embedding an API key.
- Receipt state maps onto message history without overwriting earlier timestamps. The immutable receipt ledger remains the audit source when providers send duplicate or out-of-order callbacks.

## Storage

Migration v44 creates workspace-local `delivery_receipts`:

- `provider`, `receipt_id` primary key
- provider message id and optional Notifuse message/effect ids
- normalized event (`accepted`, `sent`, `delivered`, `opened`, `failed`)
- occurrence/receipt times, error code, metadata, payload hash
- index on provider message id and on Notifuse message id

Raw callback bodies, phone numbers, device tokens and credentials are not stored in this ledger.

## HTTP surface

- `POST /api/deliveryReceipts.ingest`: authenticated trusted-system ingestion; requires Message History write and accepts at most 500 receipts.
- `POST /webhooks/delivery/twilio`: public Twilio callback; requires workspace/integration query parameters and a valid `X-Twilio-Signature`.

Both paths return duplicate/conflict information explicitly. Provider callbacks acknowledge valid duplicates with 200 and reject invalid signatures with 401.

## Failure and concurrency behavior

- Receipt insertion is one transaction using `INSERT ... ON CONFLICT DO NOTHING`; conflict hashes are compared before a duplicate is accepted.
- A bounded batch prevents request memory amplification. Each receipt reports its own result.
- Retryable provider errors carry a typed retry hint; permanent validation/provider errors do not enter retry loops.
- No provider call is automatically repeated after an ambiguous network outcome unless a higher layer can reconcile by provider/effect key.

## Deferred to v45

- SMS phone endpoints and internal decryption for dispatch.
- Journey SMS/push nodes, transactional omnichannel send API and fan-out to active device endpoints.
- Signed client receipt tokens and endpoint invalidation after FCM `UNREGISTERED` responses.
- APNs and Web Push native adapters.

