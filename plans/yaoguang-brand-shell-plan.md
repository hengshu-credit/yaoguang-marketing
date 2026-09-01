# Yaoguang Brand Shell Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan.

**Goal:** Deliver the first independently releasable Yaoguang batch: product branding in the console, Yaoguang engineering identity, runtime configuration compatibility, and the Simplified Chinese locale entry point.

**Architecture:** Centralize visible product identity in a small React brand component and constants module, keep Lingui around surrounding user-facing copy, expose a local SVG asset, and change only non-persistent engineering identifiers. Resolve runtime roles through a dedicated config function so `YAOGUANG_ROLE` wins and legacy `NOTIFUSE_ROLE` still works. Rename only first-party Go module/import paths; preserve database, queue, bucket, migration, and upstream-license identifiers.

**Tech Stack:** React, TypeScript, Ant Design, Lingui, Vitest, Go, Viper, Docker Compose.

**Spec:** `docs/superpowers/specs/2026-08-29-yaoguang-marketing-platform-design.md`

**Status:** Implemented on `main` on 2026-08-30. Frontend tests/build, changed-file lint, backend offline tests, Compose validation, and Go vet pass; repository-wide ESLint still reports five pre-existing Web Analytics warnings outside this batch.

## Global Constraints

- Display exact approved copy: `瑶光营销平台` and `观心知意，循光达客`.
- Use the Hengshu Credit animated SVG locally; do not hotlink the production website.
- Keep the product description in Chinese and English documentation.
- Preserve `demo@notifuse.com` only as existing demo data, not as product branding.
- Preserve persistent defaults such as `notifuse_system`, ClickHouse database/user names, S3 bucket names, queue routing keys, migration names, and table names in this batch.
- Preserve AGPL and upstream copyright notices.
- Do not edit the two pre-existing dirty channel-provider test files.

### Task 1: Add the Reusable Brand Lockup

**Files:**
- Create: `console/src/components/BrandLockup.tsx`
- Create: `console/src/components/BrandLockup.test.tsx`
- Create: `console/src/constants/brand.ts`
- Modify: `console/src/layouts/WorkspaceLayout.tsx`
- Modify: `console/src/pages/SignInPage.tsx`
- Modify: `console/src/pages/SetupWizard.tsx`
- Test: `console/src/__tests__/WorkspaceLayout.test.tsx`
- Test: `console/src/__tests__/SignInPage.test.tsx`
- Test: `console/src/pages/SetupWizard.test.tsx`

**Step 1: Write failing component and integration tests**

Assert that the real layouts render an image with accessible name `衡枢真信`, the visible product name `瑶光营销平台`, and the tagline `观心知意，循光达客`; assert collapsed navigation retains an accessible logo without forcing the text into view.

**Step 2: Verify red**

Run `cd console; npm test -- BrandLockup.test.tsx WorkspaceLayout.test.tsx SignInPage.test.tsx SetupWizard.test.tsx` and confirm failures are caused by the missing lockup/approved copy.

**Step 3: Implement the minimal lockup**

Export typed brand constants and a `BrandLockup` supporting full and compact variants. Replace direct PNG branding in the three target surfaces with the component while preserving existing interaction, routing, and responsive behavior.

**Step 4: Verify green**

Re-run the command from Step 2 and require zero failures.

### Task 2: Add Local Logo Asset and Browser Metadata

**Files:**
- Create: `console/public/images/hengshucredit_animated.svg`
- Modify: `console/index.html`
- Modify: `console/public/site.webmanifest`

**Step 1: Add the approved SVG as a local static asset**

Copy the exact Hengshu Credit SVG source into `console/public/images/hengshucredit_animated.svg`, retaining its original metadata and animation.

**Step 2: Update browser metadata**

Set the document title, manifest name/short name, theme metadata, preload, and SVG icon references to Yaoguang. Retain old PNG assets on disk for upgrade compatibility but stop referencing them from new metadata.

**Step 3: Verify assets through a production build**

Run `cd console; npm run build` and verify the built output contains the SVG asset and Yaoguang title/manifest values.

### Task 3: Add Yaoguang Runtime Role Compatibility

**Files:**
- Modify: `config/config.go`
- Modify: `config/realtime.go`
- Modify: `config/realtime_test.go`
- Modify: `env.example`
- Modify: `compose.yaml`

**Step 1: Write failing configuration tests**

Add table/isolated environment tests proving: `YAOGUANG_ROLE` is accepted, it wins when both role variables exist, legacy `NOTIFUSE_ROLE` remains accepted, and an invalid Yaoguang value names `YAOGUANG_ROLE` in the error.

**Step 2: Verify red**

Run `go test ./config -run 'Test.*Role' -count=1` and confirm the new cases fail because the Yaoguang variable is not read.

**Step 3: Implement minimal compatibility resolution**

Add one role-resolution function that reads `YAOGUANG_ROLE`, falls back to the already-bound `NOTIFUSE_ROLE`, and passes the selected variable name into validation. Update `env.example` and Compose role declarations to primary Yaoguang naming without changing persistence identifiers.

**Step 4: Verify green**

Run `go test ./config -count=1` and require zero failures.

### Task 4: Update Product Defaults and Package Metadata

**Files:**
- Modify: `config/config.go`
- Modify: `config/config_test.go` or the nearest existing default-config test
- Modify: `console/src/pages/SetupWizard.test.tsx`
- Modify: `console/src/pages/SetupWizard.tsx`
- Modify: `console/package.json`
- Modify: `console/package-lock.json`

**Step 1: Write failing default-behavior tests**

Assert the configured default SMTP sender and setup-wizard SMTP name use `瑶光营销平台`, and the tracing service default uses `yaoguang-marketing-api`.

**Step 2: Verify red**

Run the focused Go and console tests and confirm they fail on existing Notifuse defaults.

**Step 3: Implement the defaults and package metadata**

Change only user-visible/non-persistent defaults and the console package name. Do not alter system database, ClickHouse, S3, or migration defaults.

**Step 4: Verify green**

Re-run the focused Go and console tests and require zero failures.

### Task 5: Add the Simplified Chinese Locale Entry Point

**Files:**
- Modify: `console/lingui.config.ts`
- Modify: `console/src/i18n/index.ts`
- Modify: `console/src/__tests__/i18n.test.tsx`
- Create: `console/src/i18n/locales/zh-CN/messages.po`
- Modify: `internal/domain/languages.go`
- Modify: the backend mailer locale registry identified by `rg 'pt-BR|SupportedLanguages' pkg internal`
- Modify: corresponding frontend and backend locale tests

**Step 1: Write failing locale-list tests**

Assert `zh-CN` is accepted and exposed by frontend and backend locale registries and that selecting it loads a non-empty catalogue.

**Step 2: Verify red**

Run the focused frontend locale tests and focused backend language tests; confirm failure is the absent locale.

**Step 3: Add locale plumbing and a complete initial catalogue**

Generate the Lingui catalogue for `zh-CN`, fill every extracted entry with either reviewed Simplified Chinese or the English source text as an explicit fallback, and use reviewed Chinese for all brand, navigation, authentication, setup, customer, event, campaign, journey, import, frequency-control, and channel-facing terms present in this batch. Do not permit empty `msgstr` entries.

**Step 4: Verify green and catalogue completeness**

Run `cd console; npm test -- i18n.test.tsx catalogues.test.ts` and the focused backend language tests. Require zero missing/empty catalogue entries.

### Task 6: Rename First-Party Go Module and Imports

**Files:**
- Modify: `go.mod`
- Modify: every tracked Go source importing exact prefix `github.com/Notifuse/notifuse`
- Modify: first-party module references in build/test scripts and active documentation

**Step 1: Prove the current module contract**

Run `go list -m` and record that it returns the old first-party module. Run `rg -l 'github.com/Notifuse/notifuse' --glob '*.go' --glob 'go.mod' --glob 'go.sum'` to define the exact mechanical replacement set.

**Step 2: Apply the exact mechanical rename**

Use `go mod edit -module github.com/hengshu-credit/yaoguang-marketing` and replace only the exact first-party prefix. Do not replace `github.com/Notifuse/liquidgo` or copyright text.

**Step 3: Verify the module graph**

Run `go list -m`, `go mod tidy`, `go test ./config ./internal/domain/... ./internal/service/... -count=1`, and confirm no exact old first-party import remains in Go files.

### Task 7: Rebrand Active Documentation and Packaging

**Files:**
- Modify: `README.md`
- Modify: `Dockerfile` if active labels or image metadata contain product branding
- Modify: `compose.yaml`
- Modify: `env.example`
- Modify: active developer scripts/configuration identified by `rg -l 'Notifuse|notifuse'` after excluding persistence identifiers, generated outputs, vendored files, historical plans/specs, licenses, and demo addresses

**Step 1: Update active product documentation**

Lead README with the Chinese product name, tagline, approved product description, architecture summary, quick start, configuration compatibility, and explicit upstream Notifuse/AGPL attribution.

**Step 2: Update non-persistent packaging names**

Use Yaoguang names for Compose anchors, service image labels, and the network. Preserve volume/data/database/queue identifiers needed for upgrades.

**Step 3: Audit protected names**

Review every remaining active occurrence and classify it as persistence compatibility, upstream attribution, legacy environment compatibility, demo data, historical documentation, or a missed product-brand occurrence. Fix only the final category.

### Task 8: Final Verification and Commit Hygiene

**Files:** all files changed by Tasks 1-7.

**Step 1: Run frontend verification**

Run `cd console; npm test`, `cd console; npm run lint`, and `cd console; npm run build`.

**Step 2: Run backend verification**

Run `go test ./... -count=1`, `go vet ./...`, and any repository Makefile validation target that is documented as required and available locally.

**Step 3: Inspect the result**

Run `git diff --check`, `git status --short`, and a scoped diff review. Confirm the two pre-existing channel-provider test modifications are unchanged and excluded from Yaoguang commits.

**Step 4: Commit coherent slices**

Commit plan, console brand/localization, runtime config/package identity, Go module rename, and documentation/packaging as separate coherent commits. Do not stage the pre-existing dirty test files.
