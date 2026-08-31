# Omnichannel material templates and client preview design

## 1. Goal

Expand Yaoguang Marketing from Email, SMS, Push and Webhook-oriented delivery into a maintainable omnichannel material system. Authors must be able to create multilingual material for common Chinese and overseas messaging channels, render unsaved drafts through the same Liquid kernel used by delivery, and switch among the relevant device or client previews when a channel has multiple presentation surfaces.

The first delivery phase uses one signed generic channel Webhook for all newly added channels. Existing native Email providers, Twilio SMS and FCM Push remain unchanged. Native adapters for the newly added third-party platforms are separate future work.

## 2. Scope

### 2.1 Channel catalogue

The built-in catalogue contains these material channels:

- existing: `email`, `web`, `sms`, `push`;
- general: `in_app`, `rcs`, `webhook`;
- China: `wechat_official_account`, `wechat_mini_program`, `wecom`, `dingtalk`, `feishu`;
- global and regional messaging: `whatsapp`, `telegram`, `line`, `zalo`, `viber`, `messenger`, `instagram`, `kakao`.

`push` remains one template channel. Android, iOS, Web Push and domestic OEM clients are preview profiles and endpoint platforms, not separate template channels.

Country packs are recommendation metadata rather than persistence or delivery restrictions:

| Country or market | Recommended messaging channels |
| --- | --- |
| China | WeChat Official Account, WeChat Mini Program, WeCom, DingTalk, Feishu, App Push, Web Push, RCS |
| Kazakhstan | WhatsApp, Telegram |
| Uzbekistan | Telegram, WhatsApp |
| Philippines | Messenger, Viber, WhatsApp |
| Thailand | LINE, Messenger, WhatsApp |
| Vietnam | Zalo OA or ZNS, Messenger, Viber, WhatsApp |
| Indonesia | WhatsApp, Telegram, Messenger |
| Mexico | WhatsApp, RCS, Messenger, Instagram Messaging |
| Peru | WhatsApp, Messenger, Instagram Messaging |
| Pakistan | WhatsApp, Telegram |

All general channels remain available regardless of the selected country pack. A workspace can therefore use one WhatsApp template across several countries without cloning it.

### 2.2 Explicit non-goals

This phase does not:

- call the native WhatsApp, Telegram, LINE, Zalo, Viber, Messenger, Instagram, Kakao, WeChat, WeCom, DingTalk or Feishu APIs directly;
- synchronize platform-side template approval state;
- claim pixel-identical rendering by a third-party proprietary client;
- add a visual drag-and-drop layout engine for arbitrary vendor schemas;
- replace existing Email, SMS or Push template storage or provider adapters;
- add a new Campaign or Journey execution engine.

## 3. Design principles

1. A channel is a stable business capability; a provider is a delivery implementation; a preview profile is a presentation surface. These concepts must not share one enum.
2. New channels use one versioned structured content model instead of one database column per channel.
3. The backend owns channel capabilities, validation, Liquid rendering and delivery compilation. The console never becomes the source of truth for platform limits.
4. Preview and delivery compile the same draft structure through the same renderer. Preview may add diagnostics, but it may not silently truncate or rewrite content.
5. Existing APIs and stored Email, Web, SMS and Push templates remain valid.
6. Country recommendations are configuration metadata and may evolve without a database migration.

## 4. Channel catalogue

### 4.1 Backend registry

The domain layer exposes an immutable registry of `ChannelDefinition` values. Every definition contains:

- stable channel ID and localized display key;
- regions and recommended ISO 3166-1 alpha-2 country codes;
- content families supported by the channel;
- media, action, card, carousel and external-template capabilities;
- channel-specific bounds;
- preview profile definitions;
- supported delivery modes: `native`, `signed_webhook`, or both.

The registry rejects duplicate IDs, duplicate preview profile IDs, unsupported content families and invalid country codes at test time. The public read-only `GET /api/channels.catalog` endpoint returns definitions in a deterministic display order. It requires authenticated Workspace access but no template write permission.

The frontend uses this endpoint for channel selection, filters, country packs, capability-aware editors and preview profile selectors. User-facing names remain wrapped in Lingui; the API returns stable keys and metadata rather than translated prose.

### 4.2 Content families

The first schema version supports these content families:

- `text`: formatted or plain body plus optional link preview behavior;
- `notification`: title, body, icon or image, deep link and custom data;
- `rich_card`: optional header media, title, body, footer and actions;
- `carousel`: two or more rich cards where the channel supports it;
- `external_template`: platform template identifier, platform language, category and parameter bindings;
- `work_message`: text or markdown, title, image and actions for enterprise collaboration clients;
- `webhook_payload`: content type, JSON body template and bounded non-secret headers.

Not every channel exposes every family. The registry decides which families and fields are legal. Unsupported fields are validation errors rather than values that delivery silently drops.

## 5. Persistence and API compatibility

### 5.1 Migration v54

Workspace migration v54 adds nullable columns to `templates`:

```sql
content JSONB,
content_schema_version INTEGER
```

The same migration creates `channel_webhook_nonces` with `integration_id`, `nonce`, `expires_at` and a composite primary key. Receipt verification reserves the nonce transactionally before applying a callback; expired rows are removed opportunistically in bounded batches. This makes replay protection effective across horizontally scaled processes without depending on process memory.

The migration is additive, idempotent and does not backfill existing Email, Web, SMS or Push rows. Fresh-install schema receives the same columns and nonce table. `config.VERSION` advances from the concurrently developed Audience migration's `53.0` to `54.0`, and migration parity tests pin the version.

Existing `email`, `web`, `sms`, `push` and `translations` columns remain unchanged. New channels persist their default material in `content`; generic translations use `TemplateTranslation.Content`. A template carries exactly one default content representation matching its channel.

### 5.2 Domain model

`ChannelTemplateContent` is a typed Go structure with bounded nested structures for media, actions, cards, platform template references and arbitrary custom data. It is not an unvalidated `map[string]any`. Channel-specific validators receive the registry definition and reject illegal combinations.

Create, update, get and list APIs extend their existing payloads with:

```json
{
  "channel": "whatsapp",
  "content_schema_version": 1,
  "content": {
    "family": "external_template",
    "external_template": {
      "id": "order_update",
      "language": "es_MX",
      "parameters": []
    }
  }
}
```

Existing clients that only understand Email, Web, SMS and Push continue to receive their existing fields. List filtering accepts all registered channels and rejects unknown channels early.

### 5.3 Languages and direction

Workspace content languages add Kazakh (`kk`), Uzbek (`uz`), Filipino (`fil`) and Urdu (`ur`). Existing Indonesian, Thai, Vietnamese, Spanish, Russian, Korean, Chinese and English entries are reused. The preview response includes text direction; Urdu renders RTL while platform chrome remains appropriate to the selected locale and client.

Platform language identifiers such as WhatsApp `es_MX` remain platform-template metadata and are not confused with the Workspace content locale `es`.

## 6. Rendering and preview contract

### 6.1 Server rendering

`POST /api/templates.preview` expands from SMS and Push to all catalogue channels. A request contains Workspace, channel, schema version, unsaved content, translations, requested Workspace language, preview profile and test data.

The service performs, in order:

1. authentication and template read permission;
2. catalogue and preview profile resolution;
3. language selection and translation fallback;
4. bounded Liquid rendering for every user-visible string and recursively allowed data value;
5. channel validator and payload-size diagnostics;
6. compilation into a structured `RenderedChannelMessage`;
7. return of preview metadata, warnings and the selected profile.

The response never contains executable HTML for chat channels. Email continues to use its existing compiled HTML path and is rendered in a sandboxed iframe. Media URLs are HTTPS-only and preview failures do not persist a template.

### 6.2 Preview profiles

The initial profile set is:

- Email: desktop and mobile message or inbox surfaces;
- SMS: iOS Messages and Google Messages;
- Push: iOS lock screen and banner, Android notification, Chrome or Edge desktop, Chrome Android, Safari macOS or iOS, Huawei, Honor, Xiaomi, OPPO and vivo notification surfaces;
- WhatsApp: iOS, Android and Web;
- Telegram: mobile and desktop;
- LINE: mobile and desktop;
- Zalo: mobile;
- Viber: mobile and desktop;
- Messenger: mobile and Web;
- Instagram Messaging: mobile and Web;
- WeChat Official Account and Mini Program: WeChat mobile surfaces;
- WeCom, DingTalk and Feishu: mobile and desktop;
- RCS: Google Messages rich message;
- Kakao: mobile;
- In-App: generic iOS phone, Android phone and Web panel;
- Webhook: canonical HTTP request, signature headers and response contract.

Every preview is labeled as a simulation. Layout code renders structured data and applies documented field bounds, but does not claim to run proprietary third-party client engines.

### 6.3 Real-time behavior

The editor debounces preview requests by approximately 300 milliseconds. A monotonically increasing request generation prevents an older response from replacing a newer draft. Changing language, content family, channel or preview profile invalidates the current preview immediately.

Validation distinguishes blocking errors from non-blocking warnings. Examples include an unsupported action type, a platform approval reference that is missing, text that may wrap or truncate in a client, payload byte limits and media aspect-ratio guidance.

## 7. Console experience

Templates Page uses one create action instead of separate Email and SMS or Push buttons. The creation flow contains:

1. country or region recommendation filter;
2. channel selection with delivery-mode and availability badges;
3. content-family selection;
4. capability-aware editor;
5. language tabs and template data;
6. continuously updated preview and profile selector.

The templates table gains a channel filter and keeps category and name filtering. Each row shows the stable channel badge, content family, version and delivery mode. Existing Email editing continues to use the mature Email editor after channel selection; existing SMS and Push content is adapted to the shared message editor without changing stored payloads.

The generic editor is composed from focused typed sections: message text, media, actions, cards, external platform template, custom data and translations. Platform-specific help and limits come from the catalogue. JSON is only exposed for custom data and Webhook payloads; ordinary marketers do not edit the complete persistence document.

Responsive acceptance covers desktop, narrow laptop and mobile-width drawers. Empty search results, no configured channels, preview loading, rendering errors, unsupported content and media failures all have explicit states.

## 8. Signed generic channel Webhook

### 8.1 Integration

Add `channel_webhook` as a Workspace integration type with:

- HTTPS endpoint;
- encrypted HMAC secret;
- explicit list of allowed registered channels;
- request timeout within a bounded range;
- optional bounded non-secret headers.

Secrets follow existing integration encryption, redaction, credential hints and preserve-on-update contracts. The API never returns the plaintext secret.

### 8.2 Contact endpoints

`ContactEndpointMutation` accepts registered channels. Existing strict Twilio, FCM, APNs and Web Push validation remains. New signed-Webhook endpoints use provider `channel_webhook`, a registered channel, a registered preview or endpoint platform where applicable, and an opaque address no longer than the existing encrypted-address bound.

Endpoint addresses remain encrypted at rest and absent from public JSON, logs and timeline payloads.

### 8.3 Delivery envelope and signature

The provider sends a versioned JSON envelope containing:

- event and schema version;
- Workspace and stable `effect_key`;
- message and attempt identifiers;
- channel, platform, locale and message purpose;
- encrypted-at-rest endpoint address supplied only in the outbound request;
- template ID and version;
- structured rendered content and bounded metadata.

Headers include timestamp, nonce, effect key and an HMAC-SHA256 `v1` signature over timestamp, nonce and exact body bytes. The provider uses a bounded HTTP client, caps response bodies and never follows redirects to a different origin.

The receiver returns a bounded JSON result with `accepted`, `rejected` or `retryable`, optional provider message ID, error code and retry hint. Definite validation and permission errors are terminal; explicit rate limits and safe pre-acceptance failures may retry; ambiguous outcomes become `unknown` and are not automatically repeated.

### 8.4 Receipts

A public signed channel receipt endpoint verifies Workspace integration, timestamp, nonce and HMAC using constant-time comparison. Valid callbacks normalize accepted, sent, delivered, opened, clicked and failed events into the existing receipt ledger. Receipt IDs remain the duplicate boundary and payload-hash conflicts are rejected.

## 9. Security and limits

- Template content, translations, test data and custom data have explicit byte and depth bounds.
- Webhook URLs require HTTPS outside explicit test configuration and reject embedded credentials.
- Signature secrets are encrypted with the configured master key and redacted at every API boundary.
- Nonces are replay-protected for the accepted clock-skew window.
- Preview images cannot execute script; chat content renders as React text and structured components.
- Webhook response bodies and error details are capped before logging or persistence.
- Sensitive endpoint addresses and fully rendered customer content are excluded from logs.

## 10. Error semantics

- Unknown channel, profile, family or unsupported field: validation error.
- Invalid Liquid or data shape: preview or save validation error with field path.
- Missing translation: explicit fallback metadata, not silent ambiguity.
- Preview media failure: visible warning while text preview remains usable.
- Missing Webhook integration or endpoint: delivery validation failure before effect reservation when possible.
- Provider rejection: terminal failure with normalized code.
- Provider throttling or explicit retryable response: bounded retry according to `Retry-After`.
- Network result after request bytes may have been transmitted: `unknown`, requiring reconciliation rather than automatic resend.

## 11. Testing and acceptance

Backend coverage includes:

- registry uniqueness, country packs, capabilities and deterministic API order;
- v54 fresh schema, existing-workspace migration, idempotence and code-version parity;
- domain round trips and validation for every content family;
- legacy Email, Web, SMS and Push compatibility;
- translation fallback, new languages, Urdu RTL and platform language metadata;
- Liquid rendering, depth and byte bounds, unsupported-field errors and preview warnings;
- integration secret encryption, redaction and update preservation;
- endpoint validation and public redaction;
- Webhook canonical signature, redirect policy, timeout, bounded response, accepted, rejected, retryable, duplicate and ambiguous outcomes;
- signed receipt verification, replay rejection, duplicate receipt and hash conflict;
- handler permission and body-size contracts;
- full-stack integration coverage for create, preview, update and signed delivery.

Frontend coverage includes:

- catalogue loading, country recommendations and channel filtering;
- one create entry point and legacy Email route preservation;
- capability-aware fields for each content family;
- translation and JSON error handling;
- debounced preview and stale-response suppression;
- every preview renderer and profile switch;
- long content, RTL, missing media, warnings, empty, loading and error states;
- responsive layout without overlap or clipping.

Verification runs the relevant Go domain, service, repository, HTTP, migration and integration targets, console unit tests, Lingui extraction and compilation, TypeScript build, and real browser click-through of template creation, unsaved live preview, client switching, save, reopen and filtered-empty behavior.

## 12. Delivery order

Implementation proceeds in independently testable vertical slices:

1. channel catalogue, languages and API;
2. v54 storage and generic content domain model;
3. generic preview rendering and channel validators;
4. unified console creation and editing workflow;
5. client preview renderers and responsive behavior;
6. signed channel Webhook integration, endpoints, provider and receipts;
7. full regression, browser acceptance and documentation alignment.

Every slice preserves unrelated dirty-worktree changes. No branch, commit, push or deployment is part of this work without explicit authorization.

## 13. Reference capability sources

- LINE Messaging API message types: <https://developers.line.biz/en/docs/messaging-api/message-types/>
- Telegram Bot API: <https://core.telegram.org/bots/api>
- Viber REST Bot API: <https://developers.viber.com/docs/api/rest-bot-api/>
- WhatsApp Cloud API collection: <https://www.postman.com/meta/whatsapp-business-platform/documentation/wlk6lh4/whatsapp-cloud-api>
- Zalo Open APIs: <https://docs.zaloplatforms.com/open-apis>
