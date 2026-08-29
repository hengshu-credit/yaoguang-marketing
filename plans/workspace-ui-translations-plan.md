# Workspace UI Translations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add workspace-scoped editable UI translations, a hierarchical owner settings screen, runtime catalog overlays, and broad Simplified Chinese console coverage without removing any existing UI locale.

**Architecture:** Store sparse locale-to-message overrides in the existing workspace settings JSON and update them through an owner-only dedicated RPC endpoint. Keep Lingui PO catalogs as platform defaults, merge the active workspace's sparse overrides into the active compiled catalog, and derive the editor's safe static-message hierarchy from the English PO source references.

**Tech Stack:** Go 1.25, PostgreSQL-backed JSON workspace settings, stdlib HTTP RPC handlers, React 18, TypeScript, LinguiJS 5, Ant Design 6, Vitest.

**Spec:** `docs/superpowers/specs/2026-08-30-workspace-ui-translations-design.md`

## Global Constraints

- Work directly on the current `main` branch as explicitly requested.
- Preserve `internal/domain/customer.go` and `internal/domain/customer_test.go`; they are pre-existing untracked user work.
- Preserve the UI locales `en`, `fr`, `es`, `de`, `ca`, `pt-BR`, `ja`, `it`, and `zh-CN`.
- Runtime editing includes only static compiled messages; ICU variables, plurals, and selects remain bundle-managed.
- Only a workspace owner may persist UI translation overrides.
- Restore removes sparse overrides and falls back to the bundled catalog.
- Follow `console/CLAUDE.md`: every new user-facing string uses `useLingui()` and tagged template literals.

---

### Task 1: Workspace translation domain and owner-only API

**Files:**
- Modify: `internal/domain/workspace.go`
- Modify: `internal/domain/workspace_test.go`
- Modify: `internal/domain/mocks/mock_workspace_service.go`
- Modify: `internal/service/workspace_service.go`
- Modify: `internal/service/workspace_service_test.go`
- Modify: `internal/http/workspace_handler.go`
- Modify: `internal/http/workspace_handler_test.go`
- Modify: `console/src/services/api/workspace.ts`

**Interfaces:**
- Produces: `WorkspaceSettings.UITranslations map[string]map[string]string`
- Produces: `SetUITranslationsRequest.Validate() (string, map[string]map[string]string, error)`
- Produces: `WorkspaceServiceInterface.SetUITranslations(context.Context, string, map[string]map[string]string) error`
- Produces: `POST /api/workspaces.setUITranslations`
- Produces: `workspaceService.setUITranslations(data)` in the console client

- [ ] **Step 1: Write failing domain tests**

Add table-driven tests that assert the request accepts a known compiled ID such as `zNkWa6` for `zh-CN`, accepts an empty map as clear-all, and rejects: missing/non-alphanumeric/oversized workspace ID, unknown locale, blank or whitespace value, ID containing whitespace, a value over 2,000 UTF-8 bytes, more than 5,000 overrides, and a payload over 1 MiB. Add a JSON round-trip assertion that `ui_translations` survives `WorkspaceSettings.Value()` and `Scan()`.

- [ ] **Step 2: Run the domain tests and verify RED**

Run:

```powershell
docker run --rm -v "E:\workspace\notifuse:/src" -w /src golang:1.25-alpine go test ./internal/domain -run "Test(SetUITranslationsRequest|WorkspaceSettings_UITranslations)" -count=1
```

Expected: compile failure because `SetUITranslationsRequest` and `UITranslations` do not exist.

- [ ] **Step 3: Implement domain validation and wire shape**

Add the JSON field and request. Validation must use `IsSupportedUILanguage`, permit compiled IDs containing letters, numbers, `+`, `/`, and `=`, reject Unicode control/space characters in IDs, reject blank values without trimming stored content, enforce 2,000 bytes per value, 5,000 total entries, and 1 MiB encoded override data. Add `ValidateUITranslations()` to `WorkspaceSettings` and call it from `Validate()`.

- [ ] **Step 4: Run domain tests and verify GREEN**

Run the Step 2 command. Expected: the new focused tests pass. The unrelated pre-existing `TestZapierKeyPermissions` baseline failure is excluded by the focused `-run` expression.

- [ ] **Step 5: Write failing service and handler tests**

Service tests must assert an owner replaces only `UITranslations`, a non-owner receives `ErrUnauthorized`, authentication/repository failures propagate, and clearing writes an empty map. Handler tests must assert route registration, POST-only behavior, malformed JSON and invalid locale return `400`, non-owner returns `403`, success returns `200`, and the demo wrapper includes the new mutating route.

- [ ] **Step 6: Run service and handler tests and verify RED**

Run:

```powershell
docker run --rm -v "E:\workspace\notifuse:/src" -w /src golang:1.25-alpine go test ./internal/service ./internal/http -run "TestWorkspace(Service_SetUITranslations|Handler_HandleSetUITranslations|Handler_RegisterRoutes)" -count=1
```

Expected: compile failure because the interface and endpoint do not exist.

- [ ] **Step 7: Implement service, mock, handler, route, and TypeScript API**

Follow the existing `SetCustomFieldLabels` structure, but authorize with `userWorkspace.Role == "owner"`. The handler decodes `SetUITranslationsRequest`, calls the service, maps validation to `400`, owner failure to `403`, and returns `{"status":"success","message":"UI translations updated successfully"}`. Add TypeScript request/response interfaces and `workspaceService.setUITranslations`.

- [ ] **Step 8: Run focused backend tests and commit**

Run the Step 6 command and the Step 2 command. Then commit only Task 1 files:

```powershell
git add internal/domain/workspace.go internal/domain/workspace_test.go internal/domain/mocks/mock_workspace_service.go internal/service/workspace_service.go internal/service/workspace_service_test.go internal/http/workspace_handler.go internal/http/workspace_handler_test.go console/src/services/api/workspace.ts
git commit -m "feat: persist workspace ui translations"
```

### Task 2: Runtime Lingui workspace overlays

**Files:**
- Modify: `console/src/i18n/index.ts`
- Create: `console/src/i18n/workspaceCatalog.ts`
- Create: `console/src/i18n/workspaceCatalog.test.ts`
- Modify: `console/src/layouts/WorkspaceLayout.tsx`
- Modify: `console/src/__tests__/WorkspaceLayout.test.tsx`

**Interfaces:**
- Consumes: `WorkspaceSettings.ui_translations?: Record<string, Record<string, string>>`
- Produces: `setWorkspaceCatalog(workspaceId: string, overrides: UITranslations): Promise<void>`
- Produces: `clearWorkspaceCatalog(workspaceId: string): Promise<void>`
- Produces: `getBaseCatalog(locale: Locale): Promise<Messages>`

- [ ] **Step 1: Write failing catalog merge tests**

Test with literal base catalogs that a matching locale override wins, another locale remains unchanged, unknown IDs are ignored, clearing restores the base value, and switching from workspace A to B never carries A's value. Use a deferred promise to prove a stale A load cannot activate after B.

- [ ] **Step 2: Run the new test and verify RED**

Run:

```powershell
Set-Location console
npx vitest run src/i18n/workspaceCatalog.test.ts
```

Expected: failure because `workspaceCatalog.ts` does not exist.

- [ ] **Step 3: Implement immutable base catalogs and scoped merge**

Move catalog caching/merge mechanics to `workspaceCatalog.ts`. `loadLocale()` must always load/copy the base catalog, merge only known message IDs for the active locale, preserve its monotonic generation behavior, and keep localStorage persistence semantics unchanged. Workspace scope changes increment the same generation before rebuilding the current locale.

- [ ] **Step 4: Run the catalog tests and existing locale-race tests**

Run:

```powershell
Set-Location console
npx vitest run src/i18n/workspaceCatalog.test.ts src/__tests__/locale-race.test.tsx src/__tests__/i18n.test.tsx
```

Expected: all selected tests pass.

- [ ] **Step 5: Write a failing WorkspaceLayout integration test**

Provide a workspace with a `zh-CN` override for a sidebar message, render the layout, and assert the effective value appears after the workspace scope is registered. Unmount, render a workspace without overrides, and assert the bundled value returns.

- [ ] **Step 6: Register and clean up workspace scope in WorkspaceLayout**

Resolve the current workspace from `useAuth().workspaces`. In an effect keyed by workspace ID and the override object, call `setWorkspaceCatalog`; return cleanup calling `clearWorkspaceCatalog` for that same ID. Runtime failures log and leave the base catalog active.

- [ ] **Step 7: Run focused tests and commit**

Run the Step 4 command plus `npx vitest run src/__tests__/WorkspaceLayout.test.tsx`, then commit Task 2 files.

### Task 3: Static catalog inventory and hierarchy

**Files:**
- Create: `console/src/i18n/catalogInventory.ts`
- Create: `console/src/i18n/catalogInventory.test.ts`
- Create: `console/src/i18n/po.ts`
- Create: `console/src/i18n/po.test.ts`
- Modify: `console/src/vite-env.d.ts`

**Interfaces:**
- Produces: `parsePOCatalog(source: string): POEntry[]`
- Produces: `loadStaticCatalogInventory(): Promise<TranslationItem[]>`
- Produces: `orderLocales(active: Locale): Locale[]`
- `TranslationItem` includes compiled ID, source text, source references, menu key, page key, and bundled values for all locales.

- [ ] **Step 1: Write failing PO parser tests**

Use an inline PO fixture with comments, multiple references, escaped quotes, and multiline `msgid`/`msgstr`. Assert exact decoded values and references.

- [ ] **Step 2: Run parser test and verify RED**

Run `npx vitest run src/i18n/po.test.ts`. Expected: missing module failure.

- [ ] **Step 3: Implement the minimal PO parser**

Parse entry blocks, `#:` references, quoted continuation lines, and JSON-style escape sequences. Skip the header entry.

- [ ] **Step 4: Write failing inventory tests**

Inject small compiled catalogs and PO entries. Assert only one-literal messages are included, every item has all nine bundled values with English fallback, menu/page classification is deterministic, unmatched references land in Shared/Other, and `orderLocales('zh-CN')` returns `zh-CN` first without losing a locale.

- [ ] **Step 5: Implement catalog inventory**

Lazily import `en.po?raw` and all nine compiled `.po` modules. Invert the English simple-literal catalog from text to compiled ID, join PO references, apply ordered path rules, and return stable menu/page/source ordering. Add the `*.po?raw` module declaration.

- [ ] **Step 6: Run inventory/parser tests and commit**

Run `npx vitest run src/i18n/po.test.ts src/i18n/catalogInventory.test.ts`, then commit Task 3 files.

### Task 4: Hierarchical Languages settings screen

**Files:**
- Create: `console/src/components/settings/UITranslationsSettings.tsx`
- Create: `console/src/components/settings/UITranslationsSettings.test.tsx`
- Modify: `console/src/components/settings/SettingsSidebar.tsx`
- Modify: `console/src/components/settings/SettingsSidebar.test.tsx`
- Modify: `console/src/pages/WorkspaceSettingsPage.tsx`
- Modify: `console/src/services/api/workspace.ts`

**Interfaces:**
- Consumes: `TranslationItem[]`, active `Locale`, saved `ui_translations`, owner flag.
- Produces: tree table with current-locale-first columns, search, override markers, scoped restore, save, and discard.

- [ ] **Step 1: Write failing settings screen tests**

Mock `loadStaticCatalogInventory` with two pages and three leaves. Assert: current locale column is first; all nine locales render; edit creates a sparse override; Restore cell removes one override; Restore page removes only descendant overrides; Restore all clears the map after confirmation; Save sends the exact sparse object; Discard restores saved values; non-owner inputs and save are unavailable.

- [ ] **Step 2: Run component test and verify RED**

Run `npx vitest run src/components/settings/UITranslationsSettings.test.tsx`. Expected: missing component failure.

- [ ] **Step 3: Implement the tree editor**

Use `SettingsSectionHeader`, Ant Design `Table`, `Input`, `Badge`, `Button`, `Popconfirm`, and `SettingsSaveBar`. Keep the active locale column fixed left, horizontally scroll other locale columns, default collapsed menu/page rows, and expand ancestors during search. Store only non-default values; entering the exact bundled default removes the override. Reject whitespace-only input with an inline error.

- [ ] **Step 4: Add the owner settings route section**

Add `'languages'` to `SETTINGS_SECTIONS`, a translated `Languages` item visible only to owners, and a `WorkspaceSettingsPage` switch case passing the workspace, owner flag, `refreshWorkspaces`, and current locale. Keep invalid-section redirect behavior.

- [ ] **Step 5: Run focused settings tests and commit**

Run:

```powershell
Set-Location console
npx vitest run src/components/settings/UITranslationsSettings.test.tsx src/components/settings/SettingsSidebar.test.tsx src/__tests__/pages.smoke.test.tsx
```

Expected: all selected tests pass. Commit Task 4 files.

### Task 5: Simplified Chinese catalog coverage

**Files:**
- Modify: `console/src/i18n/locales/zh-CN.po`
- Regenerate: `console/src/i18n/locales/zh-CN.js`
- Create: `console/src/i18n/zhCN.priority.test.ts`

**Interfaces:**
- Produces: Chinese defaults for the whole console, with explicit priority-route coverage.

- [ ] **Step 1: Write the failing priority catalog test**

Parse `zh-CN.po` and assert literal Chinese translations for navigation, Explore dimensions/metrics/filter controls, list creation, broadcast creation, automation builder nodes/configuration, transactional notification creation/tracking, template settings/editor chrome, workspace settings, restore/default actions, validation, empty states, and save/cancel actions. Assert each value differs from the English source unless it matches the technical-token allowlist (`API`, `SQL`, `SMTP`, `UTM`, provider/brand names, code, IDs, URLs, and units).

- [ ] **Step 2: Run the test and verify RED**

Run `npx vitest run src/i18n/zhCN.priority.test.ts`. Expected: failures list the still-English priority strings.

- [ ] **Step 3: Translate the Simplified Chinese catalog**

Apply the agreed glossary consistently. Preserve ICU placeholders, HTML-like tags, code fragments, provider names, and punctuation. Translate static UI copy throughout the catalog, not only strings named by the priority test; leave technical-token entries intact.

- [ ] **Step 4: Extract and compile catalogs**

Run:

```powershell
Set-Location console
npm run lingui:extract
npm run lingui:compile
```

Expected: all nine catalogs compile and no translated entry loses its placeholder.

- [ ] **Step 5: Run priority and i18n tests, then commit**

Run `npx vitest run src/i18n/zhCN.priority.test.ts src/__tests__/i18n.test.tsx src/__tests__/i18nMacro.test.ts`. Commit the PO, generated JS, and test.

### Task 6: Full verification and review

**Files:**
- Modify only files required by failures directly caused by Tasks 1-5.

**Interfaces:**
- Verifies the complete spec.

- [ ] **Step 1: Run focused backend verification**

Run the Task 1 domain/service/http commands. Also run `go test ./internal/service ./internal/http -count=1`. Record the known unrelated full-domain baseline failure separately.

- [ ] **Step 2: Run frontend test, lint, build, and catalog verification**

Run:

```powershell
Set-Location console
npm test -- --run
npm run lint
npm run build
```

Expected: exit code 0 for each command.

- [ ] **Step 3: Review the requirement checklist**

Confirm: whole-console Chinese base catalog; all nine UI locales preserved; owner-only Languages settings entry; active language first; menu/page/item hierarchy; every safe static cell editable; restore cell/item/page/menu/all; sparse persistence; runtime application and clearing; priority routes covered; no unrelated Customer changes.

- [ ] **Step 4: Request code review and resolve findings**

Use `superpowers:requesting-code-review` with the implementation base SHA, current HEAD, this plan, and the design spec. Fix every Critical and Important finding, then rerun affected verification.

- [ ] **Step 5: Inspect final diff and commit remaining fixes**

Run `git status --short`, `git diff --check`, and `git diff <base-sha>..HEAD --stat`. Commit only remaining task files with a focused message. Do not stage the pre-existing untracked Customer files.

