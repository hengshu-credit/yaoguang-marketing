# SMS and Push Template Preview Design

## Goal

Make SMS and push first-class, versioned, multilingual Notifuse template channels. Authors must be able to preview unsaved content with the same Liquid renderer that delivery will use, while seeing channel-specific constraints before a campaign is activated.

## Data model

- Extend `templates` with nullable `sms JSONB` and `push JSONB` columns.
- Keep the existing append-only `(id, version)` model and optimistic concurrency checks.
- A template carries exactly one content object matching its channel.
- Translations use the same shape as default content and remain constrained to workspace languages.
- SMS stores `body` and optional `sender_id`.
- Push stores `title`, `body`, optional image/deep-link URLs, and custom data.

## Preview contract

`POST /api/templates.preview` accepts draft SMS or push content, optional translations, a requested language, test data, and a push platform. It requires template read permission.

The service resolves the requested language against the workspace default, injects workspace template variables, and renders every user-visible string through the bounded Liquid engine.

SMS preview returns GSM-7/UCS-2 encoding, visible character count, encoded unit count, segment count, per-segment capacity, and remaining capacity. Push preview returns the normalized payload, byte size, platform, and non-destructive client-layout/provider-limit warnings. Preview never silently truncates content.

## Safety and compatibility

- Existing email/web payloads and rows remain valid.
- SMS/push content is bounded before persistence and preview.
- Push custom data is recursively rendered with a depth bound and is included in payload-size calculation.
- Preview failures are client errors and do not persist partial templates.
- v43 is additive and safe for rolling application upgrades because new columns are nullable.
