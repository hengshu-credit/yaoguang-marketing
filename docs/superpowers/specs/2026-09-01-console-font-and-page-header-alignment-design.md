# Console Font Configuration and Page Header Alignment Design

## Goal

Align the `/audiences`, `/broadcasts`, `/templates`, and `/file-manager` page headers with `/customers`, and let workspace owners configure the marketing console font by selecting or entering a font family name or by uploading a font file.

The configured font applies only to the authenticated marketing console. It does not change email templates, MJML output, blog pages, notification-center content, or other customer-facing surfaces.

## Existing Behavior

- `WorkspaceLayout` already adds 24 px of desktop padding and 16 px of narrow-screen padding to normal workspace pages.
- The four affected pages add their own `p-6` wrapper, shifting their headers another 24 px down and right. `/customers` does not add this second wrapper.
- `WorkspacePageTitle` already standardizes the title at 24 px, weight 500, and line height 1.3.
- `App.tsx` and `index.css` currently hard-code the bundled `AlimamaFangYuanTiVF` font. The font does not cover every supported UI language.
- Workspace settings are stored in the existing system-database JSON settings object. No schema migration is required for another optional setting.
- `FileManagerProvider` already supports browser uploads to the configured S3-compatible store and returns a browser-readable file URL.

## Page Header Alignment

Remove the page-local `p-6` wrapper from:

- `AudiencesPage`
- `BroadcastsPage`
- `TemplatesPage`
- `FileManagerPage`

The pages continue to use `WorkspacePageTitle`, preserve their current subtitles, action buttons, tabs, filters, and vertical spacing, and rely on `WorkspaceLayout` as the single owner of page-edge padding. On desktop their title origin therefore matches `/customers`; on narrow screens all five pages follow the same 16 px layout padding.

No new subtitle is invented for a page that does not already have one.

## Workspace Font Data Model

Add an optional `console_font` object to `WorkspaceSettings` in Go and TypeScript:

```text
console_font:
  family: string
  url: string, optional
  file_name: string, optional
```

Semantics:

- `family` is a single font-family name. It can come from a provided option or from owner-entered text.
- An empty `family` means the recommended system multilingual stack.
- When `url` is empty, the console attempts to use `family` as a locally available font and then falls back to the system multilingual stack.
- When `url` is present, the console loads that resource under a fixed internal family alias. `family` remains the operator-facing name; the file's internal family metadata is not trusted or required.
- `file_name` records the selected file for display and extension validation.

The backend accepts an omitted `console_font` for backward compatibility and preserves the stored value when a partial workspace update omits it. After trimming, `family` may be empty (system default) or contain at most 128 Unicode code points; its non-space characters must be Unicode letters or numbers, `-`, `_`, or `.` so the value always names one family rather than a CSS expression. `url` may contain at most 2,048 characters and must use HTTP or HTTPS. `file_name` may contain at most 255 Unicode code points and, whenever `url` is present, must end case-insensitively in `.ttf`, `.otf`, `.woff`, or `.woff2`. Invalid settings fail the workspace update with a field-specific error.

## Configuration Center UI

Add a focused `ConsoleFontSettings` form section under Workspace Settings > General. The existing owner-only editing rule applies.

The section offers two modes:

1. **Font name** — an editable, searchable dropdown. Options include System default (recommended), Alimama FangYuan, PingFang SC, Microsoft YaHei, Noto Sans, Noto Sans SC, Arial, and Helvetica. An owner may type another locally installed family name.
2. **Uploaded font** — a button opens the existing file manager, where the owner can upload or select one `.ttf`, `.otf`, `.woff`, or `.woff2` file. The selected URL, filename, and display family are written into the same General Settings form and are not persisted until the existing Save control is used.

Changing back to Font name clears the pending uploaded URL and filename. Discard restores the last saved font configuration together with the other General Settings fields. The read-only General Settings view shows the configured family and whether it comes from an uploaded file.

All new copy uses Lingui. Catalog extraction runs after implementation, with Chinese translations supplied for the new configuration labels and messages.

The file manager's MIME map gains `.woff` and `.woff2`. The selector filters to supported font extensions. A configured S3/CDN endpoint must allow the console origin to read the font; otherwise runtime loading follows the failure behavior below.

## Runtime Font Application

Define one CSS custom property, `--console-font-family`, on the document root. Both base console CSS and the Ant Design `ConfigProvider` font token use this property.

The fallback value is a multilingual system stack:

```text
system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans",
"Noto Sans CJK SC", "PingFang SC", "Microsoft YaHei", Arial, sans-serif
```

`WorkspaceLayout` resolves the active workspace's saved `console_font` setting and applies it through a focused runtime font helper:

- For a named font, quote and escape the single family name, place it before the fallback stack, and update the root CSS variable immediately.
- For an uploaded font, reset to the fallback while loading, create a `FontFace` with the fixed internal alias and saved URL, wait for it to load, register it in `document.fonts`, and then place the alias before the fallback stack.
- On workspace changes, remove the previously registered uploaded `FontFace`, ignore a stale asynchronous load from the previous workspace, and resolve the new workspace's setting.
- When leaving the workspace shell, restore the system fallback.

The bundled Alimama file remains available as a named preset, but it is no longer the default.

## Failure Behavior

- A missing or legacy `console_font` setting uses the system multilingual stack.
- A named font that is not installed naturally falls through to the system stack without blocking the page.
- An uploaded font that cannot be fetched, is rejected by the browser, or fails to parse leaves the fallback active and shows one localized warning for that load attempt.
- A stale font load that completes after a workspace switch is discarded and never overwrites the new workspace's font.
- File-manager configuration and upload errors continue to use the existing file-manager error UI.

## Security and Scope Boundaries

- Font family text is never concatenated into a stylesheet. It is normalized as one family name and assigned through the DOM style API.
- Uploaded font URLs must use HTTP or HTTPS; executable/data URLs are rejected by backend validation.
- Only the existing workspace owner update flow can persist General Settings changes.
- No font binary is stored in the workspace JSON, system database, or console bundle.
- No new backend upload endpoint, database table, or deployment volume is introduced.
- Font file lifecycle and deletion remain owned by the existing file manager.

## Test Strategy

### Frontend unit tests

- Each affected page has no page-local padding around its primary heading and continues to render the shared `WorkspacePageTitle` semantics.
- General Settings maps saved font data into the form, saves both named and uploaded configurations, clears uploaded fields when switching modes, and restores saved data on Discard.
- The font selector lists the required presets, accepts an entered family name, and filters uploaded selections to supported font extensions.
- The runtime helper applies named fonts, loads uploaded fonts, removes an old uploaded face on workspace switch, ignores stale loads, restores the fallback on cleanup, and warns while retaining the fallback after load failure.
- `App.tsx` and base CSS consume `--console-font-family` rather than a hard-coded default.
- `.woff` and `.woff2` resolve to the correct MIME types.

### Backend unit and HTTP tests

- `WorkspaceSettings` JSON value/scan round-trips `console_font`.
- validation accepts a named font and each supported upload extension.
- validation rejects invalid schemes, unsupported extensions, and excessive field lengths.
- an update that omits `console_font` preserves the stored setting, while an explicit empty object resets it.
- the workspace update endpoint passes valid font settings to the service and returns validation failures.

### Integrated verification

- Run focused Vitest suites, Lingui extraction/compilation, TypeScript build, ESLint for touched console code, focused Go domain/service/HTTP tests, and `git diff --check`.
- Start the real console and API, then verify `/customers`, `/audiences`, `/broadcasts`, `/templates`, and `/file-manager` at desktop and narrow widths.
- Confirm English and Chinese titles share the same origin, named-font changes apply after save, an uploaded font applies after save and reload, and a deliberately unreachable font URL visibly falls back without breaking navigation.

## Acceptance Criteria

- The four requested page headers align with `/customers` on desktop and narrow layouts without double padding.
- A workspace owner can select or enter a console font family from General Settings.
- A workspace owner can upload or select a supported font file through the existing file manager and save it as the console font.
- The selected font applies across the marketing console for that workspace and survives reload and sign-in from another browser.
- Existing workspaces default to the multilingual system stack.
- Font load failure is recoverable and leaves all console content readable.
- Email/template/customer-facing font behavior is unchanged.
