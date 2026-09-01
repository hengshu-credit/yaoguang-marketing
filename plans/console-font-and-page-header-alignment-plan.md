# Console Font Configuration and Page Header Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task in the current session. Do not dispatch subagents for this plan.

**Goal:** Align four workspace page headers with `/customers` and add a persistent workspace-level marketing-console font selector with uploaded-font support and safe multilingual fallback.

**Architecture:** Extend the existing JSON-backed `WorkspaceSettings` contract with an optional validated `console_font` object, reuse `FileManagerProvider` for font selection/upload, and apply the active workspace font through one CSS custom property plus a lifecycle-safe runtime controller. Keep workspace layout padding as the sole page-edge padding owner.

**Tech Stack:** Go, React 18, TypeScript, Ant Design 6, LinguiJS, Vitest/Testing Library, TanStack Router, browser `FontFace` API, existing S3-compatible file manager.

**Spec:** `docs/superpowers/specs/2026-09-01-console-font-and-page-header-alignment-design.md`

## Global Constraints

- The font applies only to the authenticated marketing console; email templates, MJML, blog output, notification-center content, and other customer-facing surfaces remain unchanged.
- Existing workspaces and invalid or unavailable named fonts use the multilingual system fallback.
- Uploaded fonts are limited to `.ttf`, `.otf`, `.woff`, and `.woff2` and HTTP(S) URLs.
- Reuse the existing file manager; add no database table, migration, upload endpoint, deployment volume, or Base64 font payload.
- All new user-facing strings use Lingui, and new Chinese strings receive `zh-CN` translations.
- Preserve unrelated worktree changes. Do not create a branch, commit, push, or publish.

---

### Task 1: Persist and Validate the Workspace Font Contract

**Files:**

- Modify: `internal/domain/workspace.go`
- Modify: `internal/domain/workspace_test.go`
- Modify: `internal/http/workspace_handler_test.go`

**Interfaces:**

- Produces: `domain.ConsoleFontSettings{Family, URL, FileName string}`
- Produces: `WorkspaceSettings.ConsoleFont *ConsoleFontSettings` serialized as `console_font`
- Preserves: omitted `console_font` values during `UpdateWorkspaceRequest` partial updates

- [ ] **Step 1: Add failing round-trip, validation, and omission tests**

Add focused tests that pin the wire contract:

```go
func TestWorkspaceSettings_ConsoleFontValueAndScan(t *testing.T) {
    settings := WorkspaceSettings{ConsoleFont: &ConsoleFontSettings{
        Family: "Noto Sans SC",
        URL: "https://cdn.example.com/fonts/noto.woff2",
        FileName: "noto.woff2",
    }}
    value, err := settings.Value()
    require.NoError(t, err)
    var decoded WorkspaceSettings
    require.NoError(t, decoded.Scan(value))
    assert.Equal(t, settings.ConsoleFont, decoded.ConsoleFont)
}
```

Add a table test around a valid base `WorkspaceSettings{Timezone: "UTC", DefaultLanguage: "en", Languages: []string{"en"}}` covering:

- empty/nil font setting;
- family `Noto Sans SC`;
- each supported extension with an HTTPS URL;
- invalid `data:` and `javascript:` URLs;
- unsupported `.eot` filename;
- missing filename with a URL;
- family over 128 runes;
- family containing `,`, quotes, braces, newline, or another CSS delimiter;
- URL over 2,048 characters;
- filename over 255 runes.

Extend `TestUpdateWorkspaceRequest_PreserveOmitted` so the stored settings contain a font, an omitted key restores it, and an explicit `"console_font": {}` wins and clears it.

In `workspace_handler_test.go`, add one successful `POST /api/workspaces.update` test whose mock uses `DoAndReturn` to assert that `settings.ConsoleFont` contains the posted family, URL, and filename before returning a workspace. Add one invalid-upload test that posts a `data:` URL and asserts HTTP 400 without a service call.

- [ ] **Step 2: Run the tests and verify RED**

Run:

```powershell
go test ./internal/domain -run 'TestWorkspaceSettings_ConsoleFont|TestUpdateWorkspaceRequest_PreserveOmitted' -count=1
go test ./internal/http -run 'TestWorkspaceHandler_Update_ConsoleFont' -count=1
```

Expected: compilation/test failure because `ConsoleFontSettings` and `WorkspaceSettings.ConsoleFont` do not exist and omission preservation is absent.

- [ ] **Step 3: Add the minimal domain implementation**

Add the model and validation helper near `WorkspaceSettings`:

```go
type ConsoleFontSettings struct {
    Family   string `json:"family,omitempty"`
    URL      string `json:"url,omitempty"`
    FileName string `json:"file_name,omitempty"`
}

func (font *ConsoleFontSettings) Validate() error
```

Implementation rules:

- trim all three fields before validation/storage;
- count lengths with `utf8.RuneCountInString`;
- allow family runes only when `unicode.IsLetter`, `unicode.IsNumber`, or the rune is space, `-`, `_`, or `.`;
- parse the URL and require scheme `http` or `https`;
- when URL is set, require a filename with a case-insensitive supported extension;
- return field-specific errors prefixed with `console font`.

Add `ConsoleFont *ConsoleFontSettings `json:"console_font,omitempty"`` to `WorkspaceSettings`, invoke `Validate` when non-nil, add `console_font` to `preservableWorkspaceSettingKeys`, and restore it in `PreserveOmitted`.

- [ ] **Step 4: Run domain tests and verify GREEN**

Run:

```powershell
go test ./internal/domain -run 'TestWorkspaceSettings_ConsoleFont|TestUpdateWorkspaceRequest_PreserveOmitted|TestWorkspaceSettings_ValueAndScan' -count=1
go test ./internal/http -run 'TestWorkspaceHandler_Update_ConsoleFont' -count=1
```

Expected: PASS.

---

### Task 2: Implement the Console Font Runtime

**Files:**

- Create: `console/src/lib/consoleFont.ts`
- Create: `console/src/lib/consoleFont.test.ts`
- Modify: `console/src/App.tsx`
- Modify: `console/src/index.css`
- Modify: `console/src/layouts/WorkspaceLayout.tsx`
- Modify: `console/src/services/api/workspace.ts`
- Modify: `console/src/__tests__/WorkspaceLayout.test.tsx`

**Interfaces:**

- Consumes: `WorkspaceSettings.console_font?: ConsoleFontSettings`
- Produces: `CONSOLE_FONT_FALLBACK`, `consoleFontStack(family?: string)`, and `applyConsoleFont(settings, runtime)` returning cleanup
- Produces: root CSS property `--console-font-family`

- [ ] **Step 1: Add TypeScript API types and failing pure runtime tests**

Add:

```ts
export interface ConsoleFontSettings {
  family: string
  url?: string
  file_name?: string
}
```

and `console_font?: ConsoleFontSettings` to `WorkspaceSettings`.

In `consoleFont.test.ts`, provide fake root style, font set, and font-face factory objects and assert:

- missing settings set exactly `CONSOLE_FONT_FALLBACK`;
- named `Noto Sans SC` becomes `"Noto Sans SC", ${CONSOLE_FONT_FALLBACK}`;
- a family containing a quote is rejected to fallback by the defensive runtime normalizer;
- uploaded font starts on fallback, registers the loaded face, then applies the fixed alias;
- cleanup deletes a registered uploaded face and restores fallback;
- cleanup before load resolution prevents stale registration/application;
- load rejection retains fallback and calls `onError` once.

- [ ] **Step 2: Run runtime tests and verify RED**

Run:

```powershell
npm test -- --run src/lib/consoleFont.test.ts
```

from `console/`.

Expected: FAIL because the runtime module does not exist.

- [ ] **Step 3: Implement the pure runtime controller**

Create a dependency-injected controller with browser defaults:

```ts
export interface ConsoleFontRuntime {
  root: HTMLElement
  fonts: Pick<FontFaceSet, 'add' | 'delete'>
  createFontFace: (family: string, source: string) => FontFace
  onError?: () => void
}

export function applyConsoleFont(
  settings: ConsoleFontSettings | undefined,
  runtime?: Partial<ConsoleFontRuntime>
): () => void
```

Use the fixed alias `YaoguangWorkspaceUploadedFont`, set fallback before asynchronous loading, guard completion with a disposed flag, and never build a `<style>` string.

- [ ] **Step 4: Verify runtime tests GREEN**

Run the same focused Vitest command. Expected: PASS.

- [ ] **Step 5: Add failing workspace lifecycle tests**

In `WorkspaceLayout.test.tsx`, mock `applyConsoleFont` and assert that rendering with `settings.console_font` passes the current workspace setting and unmount invokes the returned cleanup. Rerender with another workspace setting and assert old cleanup runs before the new application.

- [ ] **Step 6: Run the workspace lifecycle test and verify RED**

Run:

```powershell
npm test -- --run src/__tests__/WorkspaceLayout.test.tsx
```

Expected: FAIL because `WorkspaceLayout` does not apply workspace font settings.

- [ ] **Step 7: Wire the runtime into the shell and theme**

- in `WorkspaceLayout`, derive the current workspace once and use an effect that calls `applyConsoleFont` with a localized warning callback;
- in `App.tsx`, replace the hard-coded font token with `var(--console-font-family)`;
- in `index.css`, define the fallback custom property on `:root` and make the body/base font use `var(--console-font-family)`;
- keep the bundled Alimama `@font-face`, but remove it from the default stack.

- [ ] **Step 8: Verify runtime and layout tests GREEN**

Run both focused test files. Expected: PASS.

---

### Task 3: Add the General Settings Font Editor and File Selection Support

**Files:**

- Create: `console/src/components/settings/ConsoleFontSettings.tsx`
- Create: `console/src/components/settings/ConsoleFontSettings.test.tsx`
- Modify: `console/src/components/settings/GeneralSettings.tsx`
- Modify: `console/src/components/settings/GeneralSettings.test.tsx`
- Modify: `console/src/components/file_manager/context.tsx`
- Modify: `console/src/components/file_manager/fileExtensions.ts`
- Create: `console/src/components/file_manager/fileExtensions.test.ts`

**Interfaces:**

- Extends: `SelectFileButtonProps.onSelect(url: string, item?: StorageObject): void` without breaking one-argument consumers
- Produces: `FONT_FAMILY_OPTIONS`, `isSupportedConsoleFontFile`, and the nested form value `console_font`

- [ ] **Step 1: Add failing MIME and settings-component tests**

Add MIME assertions:

```ts
expect(GetContentType('font.woff')).toBe('font/woff')
expect(GetContentType('font.woff2')).toBe('font/woff2')
```

Mock `useFileManager` with a `SelectFileButton` that invokes `onSelect` using a representative `StorageObject`. Test the new component for:

- visible Font name and Uploaded font modes;
- required dropdown options including System default, Alimama FangYuan, PingFang SC, Microsoft YaHei, Noto Sans SC, Arial, and Helvetica;
- accepting a typed `Noto Sans JP` family;
- selecting `brand.woff2` records URL, filename, and inferred display family;
- supported-file filtering accepts four approved extensions case-insensitively and rejects `.eot` and non-files;
- switching from upload to name clears URL and filename.

- [ ] **Step 2: Run focused tests and verify RED**

Run from `console/`:

```powershell
npm test -- --run src/components/settings/ConsoleFontSettings.test.tsx src/components/file_manager/fileExtensions.test.ts
```

Expected: FAIL because the component and MIME mappings are missing.

- [ ] **Step 3: Implement the focused editor and file-manager callback extension**

- add `.woff: 'font/woff'` and `.woff2: 'font/woff2'`;
- change the context callback to pass the selected `StorageObject` as a second argument;
- implement an Ant Design `Radio.Group` plus editable `AutoComplete` for named fonts;
- expose the file-manager button with `acceptFileType=".ttf,.otf,.woff,.woff2"` and an `acceptItem` extension predicate;
- infer the display family by removing the extension and replacing disallowed filename characters with spaces;
- use nested Ant Form fields so General Settings owns save/discard lifecycle.

- [ ] **Step 4: Verify component tests GREEN**

Run the same focused test command. Expected: PASS.

- [ ] **Step 5: Add failing General Settings integration tests**

Extend the workspace fixture with a saved font and assert:

- form initialization shows the saved named or uploaded configuration;
- Discard restores all saved font fields;
- Save sends `settings.console_font` while preserving other workspace settings;
- changing from upload to System default sends `{ family: '' }` without URL or filename;
- the non-owner description shows the configured family and uploaded-source indicator.

- [ ] **Step 6: Run General Settings tests and verify RED**

Run:

```powershell
npm test -- --run src/components/settings/GeneralSettings.test.tsx
```

Expected: FAIL because General Settings does not map or render console fonts.

- [ ] **Step 7: Integrate the editor into General Settings**

Extend `GeneralSettingsFormValues`, `toFormValues`, save mapping, read-only `Descriptions`, and the owner form. Place the font section after Logo and before Timezone so it remains part of general workspace appearance settings.

- [ ] **Step 8: Verify settings tests GREEN**

Run General Settings plus the new focused component tests. Expected: PASS.

---

### Task 4: Remove Double Page Padding and Pin Header Alignment

**Files:**

- Modify: `console/src/pages/AudiencesPage.tsx`
- Modify: `console/src/pages/BroadcastsPage.tsx`
- Modify: `console/src/pages/TemplatesPage.tsx`
- Modify: `console/src/pages/FileManagerPage.tsx`
- Modify: `console/src/pages/AudiencesPage.test.tsx`
- Modify: `console/src/__tests__/pages.smoke.test.tsx`

**Interfaces:**

- Preserves: `WorkspacePageTitle` title semantics, current subtitles/actions/tabs, and `WorkspaceLayout` padding ownership

- [ ] **Step 1: Add failing DOM-structure assertions**

For Audiences and the three pages already rendered in `pages.smoke.test.tsx`, locate each level-one heading and assert:

```ts
expect(heading.closest('.p-6')).toBeNull()
```

Also keep the existing title-size assertions and tab/navigation assertions.

- [ ] **Step 2: Run focused page tests and verify RED**

Run from `console/`:

```powershell
npm test -- --run src/pages/AudiencesPage.test.tsx src/__tests__/pages.smoke.test.tsx
```

Expected: FAIL because all four headings are still inside a page-local `p-6` wrapper.

- [ ] **Step 3: Remove only the duplicate padding classes**

Replace each root `<div className="p-6">` with an unpadded root container. Do not change header margins, subtitles, action placement, tabs, filters, tables, or empty states.

- [ ] **Step 4: Verify page tests GREEN**

Run the same focused page tests. Expected: PASS.

---

### Task 5: Localize, Build, and Perform End-to-End Verification

**Files:**

- Modify: `console/src/i18n/locales/*.po`
- Regenerate: `console/src/i18n/locales/*.js`
- Modify if required by extraction: `console/src/i18n/zhCN.priority.test.ts`
- Update: `CHANGELOG.md`

**Interfaces:**

- Verifies: backend contract, console type/lint/build health, persisted font application, multilingual fallback, and page geometry

- [ ] **Step 1: Extract and compile Lingui catalogs**

Run from `console/`:

```powershell
npm run lingui:extract
npm run lingui:compile
```

Fill the new `zh-CN.po` entries with Chinese translations, then compile again. Do not replace or overwrite existing translations.

- [ ] **Step 2: Add the unreleased changelog entry**

Under the current unreleased version, describe the workspace-configurable console font, multilingual fallback, uploaded-font support through the existing file manager, and four-page header alignment as one Feature/Improvement bullet. Do not describe unshipped intermediate behavior as a separate fix.

- [ ] **Step 3: Run focused and layer-wide verification**

Run at repository root unless stated otherwise:

```powershell
go test ./internal/domain -count=1
go test ./internal/service -run 'Workspace' -count=1
go test ./internal/http -run 'Workspace' -count=1
```

Run from `console/`:

```powershell
npm test -- --run src/lib/consoleFont.test.ts src/components/settings/ConsoleFontSettings.test.tsx src/components/settings/GeneralSettings.test.tsx src/components/file_manager/fileExtensions.test.ts src/__tests__/WorkspaceLayout.test.tsx src/pages/AudiencesPage.test.tsx src/__tests__/pages.smoke.test.tsx
npm run lint
npm run build
```

Then run:

```powershell
git diff --check
git status --short
```

Expected: all tests/build/lint pass; diff check is clean; only intended files are modified.

- [ ] **Step 4: Verify real desktop and narrow-screen UI**

Start the existing local stack using the repository's documented development command, then automate a signed-in browser session:

- at desktop width, record the level-one heading bounding-box `x` and `y` for `/customers`, `/audiences`, `/broadcasts`, `/templates`, and `/file-manager`; assert each requested page matches `/customers` within one CSS pixel;
- repeat at 375 px width and verify no action/title overlap or horizontal clipping;
- save System default, switch English and Chinese, and confirm all navigation/title glyphs render without missing-glyph boxes;
- select a named font and confirm the computed `font-family` changes after save and survives reload;
- upload/select a `.woff2` font through the file manager, save, reload, and confirm `document.fonts.check` and computed font family use the uploaded alias;
- deliberately configure an unreachable HTTP font URL through the API fixture or controlled test setup, reload, and confirm the localized warning appears while computed style remains on the fallback;
- switch between two workspaces with different font settings and confirm the old uploaded face does not leak into the new workspace.

- [ ] **Step 5: Report evidence without committing**

Summarize changed files, RED/GREEN test evidence, full verification results, any environment-only gates, and browser observations. Leave all changes uncommitted for user review.
