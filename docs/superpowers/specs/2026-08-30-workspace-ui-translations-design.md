# Workspace UI Translations Design

**Status:** approved for implementation  
**Date:** 2026-08-30  
**Target branch:** `main`

## Goal

Keep every existing console UI locale, substantially complete the Simplified Chinese catalog across the whole console, and let each workspace owner override static interface copy from a hierarchical settings screen. The screenshots and the routes `/automations`, `/web-analytics/explore`, `/transactional-notifications`, and `/templates` are priority acceptance surfaces, not the scope boundary.

## Product behavior

- Add a **Languages** section to workspace settings.
- Present translations as a tree: menu area, page, then concrete static UI item.
- Pin the active UI locale as the first editable language column and place the other supported UI locales after it.
- Allow editing every safe static message in every existing UI locale.
- Allow restoring one cell, one item, one page, one menu area, or all overrides to platform defaults.
- Show which cells are workspace overrides and which still inherit platform defaults.
- Only workspace owners may edit. Every workspace member consumes the effective translations.
- Pages without a workspace context use the bundled platform catalogs only.
- Dynamic ICU messages with variables, plural rules, or select rules remain bundle-managed and are excluded from the workspace editor.

## Catalog model

Lingui remains the translation engine and the repository's PO files remain the platform defaults. The console currently supports `en`, `fr`, `es`, `de`, `ca`, `pt-BR`, `ja`, `it`, and `zh-CN`; workspace content-language settings do not change this UI-locale set.

The compiled Lingui message ID is the persisted override key. This lets an override merge directly into a loaded catalog without replacing thousands of existing `t` calls. The settings screen lazily loads the English PO source and the nine compiled catalogs, keeps only single-literal messages, and maps the English literal to its compiled ID. Source references from the PO file provide hierarchy metadata.

The effective value for a workspace page is:

1. workspace override for active locale and compiled message ID;
2. bundled catalog value for active locale;
3. bundled English value.

Changing an English source message changes its generated ID, so an old override becomes unreachable instead of being incorrectly applied to a different message. A later save naturally drops unreachable overrides because the editor only submits known IDs.

## Hierarchy

The catalog loader classifies PO source references with deterministic path rules. Menu shells (`WorkspaceLayout`, `SettingsSidebar`) form menu groups. Feature directories and route page files form pages such as Automations, Web Analytics, Transactional Notifications, Templates, Lists, Contacts, Segments, Broadcasts, Blog, Settings, and shared components. Unmatched strings are retained under **Shared / Other**, so classification never hides a safe static message.

The first language column renders the tree labels for menu and page rows. Leaf rows render an input in every locale column. Search matches the effective values in all locales and expands matching ancestors.

## Persistence and API

Sparse overrides live in the existing workspace settings JSON as:

```json
{
  "ui_translations": {
    "zh-CN": { "compiledMessageId": "中文覆盖值" },
    "en": { "compiledMessageId": "Custom English value" }
  }
}
```

This needs no database migration because workspace settings are already JSON-backed. A dedicated `POST /api/workspaces.setUITranslations` endpoint replaces only this field and preserves every other workspace setting. Reusing `workspaces.update` is intentionally avoided because a stale general-settings form could overwrite translation changes.

The domain validates workspace IDs, UI locale codes, message IDs, per-value length, nonblank values, locale entry counts, and total serialized size. The service authenticates the caller, requires the owner role, loads the current workspace, replaces only `UITranslations`, validates it, and updates the workspace. The existing workspace list/get responses distribute overrides to the console.

## Runtime application

The i18n module caches immutable base catalogs separately from the active merged catalog. `WorkspaceLayout` registers the current workspace's overrides; locale changes always rebuild from the base catalog plus the currently registered workspace overrides. Leaving a workspace clears the scope and restores the base catalog. Generation guards cover both locale switches and workspace switches so a slow catalog import cannot reactivate stale workspace copy.

## Restore and save behavior

Editor state is a sparse copy of the saved override object. Editing a value adds or changes one sparse entry. Restoring removes entries at the selected scope. Save sends the complete sparse map through the dedicated endpoint, refreshes the authentication workspace list, and immediately reapplies the saved overrides. Discard restores the last fetched map.

## Chinese baseline

The Simplified Chinese PO catalog is audited across the entire console. Product vocabulary uses a consistent glossary: workspace, automation/journey, broadcast/campaign, transactional notification, template, list, segment, web analytics, tracking, bounce, complaint, and delivery. Brand names, API identifiers, SQL, SMTP, UTM parameters, provider names, code snippets, and protocol tokens remain unchanged where translation would reduce clarity.

At minimum, all navigation, headings, actions, form labels, placeholders, help copy, empty states, status text, validation messages, and editor chrome on the priority routes must resolve to Chinese. A catalog test pins these priority strings and verifies that every supported UI locale still compiles.

## Error handling and limits

- Invalid locale, blank value, malformed ID, oversized value, excessive entry count, or oversized payload returns `400`.
- Non-owner writes return `403`.
- Runtime loading failures log the error and keep the bundled catalog active.
- Unknown or obsolete persisted IDs are ignored at runtime.
- The editor rejects empty/whitespace-only overrides and explains that Restore should be used to inherit the default.

## Testing

- Domain tests cover validation, normalization, clearing, limits, and JSON round trips.
- Service tests cover owner authorization, non-owner rejection, field-only replacement, clearing, and repository failures.
- HTTP tests cover route registration, methods, malformed input, validation, permissions, success, and demo restriction.
- i18n tests cover base/override merge, locale changes, workspace changes, stale async loads, and clearing scope.
- Catalog tests cover PO parsing, static-message filtering, hierarchy fallback, active-locale-first ordering, and priority Chinese strings.
- Component tests cover menu visibility, owner editing, restore scopes, save/discard, and read-only consumption.
- Final verification runs focused Go tests in the Go Docker image, frontend tests, Lingui extraction/compile, lint, and production build.

