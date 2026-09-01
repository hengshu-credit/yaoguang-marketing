# Template Category Management Implementation Plan

> **For agentic workers:** Execute inline with test-driven development. Do not use subagents, create a branch, stage, commit or push. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Workspace-maintained material-template categories while preserving existing category IDs and marketing/transactional compliance behavior.

**Architecture:** Migration v55 seeds a Workspace category catalogue. A focused repository/service/handler owns category CRUD; TemplateService validates assignments, while repository reads enrich templates with category purpose. The console consumes the catalogue for filters and editor selects and exposes a management drawer.

**Tech Stack:** Go, PostgreSQL Workspace migrations, net/http, React 18, TypeScript, Ant Design, TanStack Query, Lingui, Vitest.

**Spec:** `docs/superpowers/specs/2026-09-01-template-category-management-design.md`

## Global Constraints

- Preserve unrelated dirty-worktree edits.
- Existing template request payloads and stored category IDs remain valid.
- Never infer marketing purpose from a custom category name.
- Never cascade category deletion to templates.
- Use Template read/write permissions for category routes.
- Every production change starts with a failing focused test.

---

### Task 1: Domain category contract

**Files:**
- Create: `internal/domain/template_category.go`
- Create: `internal/domain/template_category_test.go`
- Modify: `internal/domain/template.go`

- [ ] Add failing tests for ID pattern, name length, purpose enum, sort range, system mutation and delete conflict errors.
- [ ] Implement `TemplateCategoryDefinition`, request types, repository/service interfaces, built-in definitions and `EffectiveTemplateCategoryPurpose` fallback.
- [ ] Add `CategoryPurpose` to `Template` as a read-enrichment field.
- [ ] Run `go test ./internal/domain -run 'TestTemplateCategory' -count=1`.

### Task 2: Migration v55 and fresh Workspace schema

**Files:**
- Create: `internal/migrations/v55.go`
- Create: `internal/migrations/v55_test.go`
- Modify: `config/config.go`
- Modify: `internal/database/init.go`
- Modify: migration version assertions.

- [ ] Add failing migration tests for table creation, built-in seed rows, legacy distinct-category seed and registration.
- [ ] Implement additive v55 statements and fresh-schema table/default inserts.
- [ ] Set `config.VERSION` to `55.0` and update pinned version assertions without changing earlier migration bodies.
- [ ] Run `go test ./internal/migrations -count=1`.

### Task 3: Category repository and service

**Files:**
- Create: `internal/repository/template_category_postgres.go`
- Create: `internal/repository/template_category_postgres_test.go`
- Create: `internal/service/template_category_service.go`
- Create: `internal/service/template_category_service_test.go`

- [ ] Add failing repository tests for ordered list, create, update, usage count, system delete rejection and transactional used-category delete rejection.
- [ ] Implement Workspace-connection-backed repository CRUD.
- [ ] Add failing service tests for Template read/write permissions, immutable purpose and inactive assignment lookup.
- [ ] Implement authenticated category service.
- [ ] Run focused repository and service tests.

### Task 4: Category HTTP API and application wiring

**Files:**
- Create: `internal/http/template_category_handler.go`
- Create: `internal/http/template_category_handler_test.go`
- Modify: `internal/app/app.go`
- Modify: `console/src/services/api/permissions.ts`

- [ ] Add failing handler tests for all four routes, method validation, 400/403/409/500 mappings and redacted responses.
- [ ] Implement handler and register routes.
- [ ] Wire repository/service in the application and declare endpoint permissions.
- [ ] Run `go test ./internal/http ./internal/app -run 'TestTemplateCategory|^$' -count=1`.

### Task 5: Template assignment and runtime purpose

**Files:**
- Modify: `internal/service/template_service.go`
- Modify: `internal/service/template_service_test.go`
- Modify: `internal/repository/template_postgres.go`
- Modify: `internal/repository/template_postgres_test.go`
- Modify: `internal/service/automation_node_executor.go`
- Modify relevant sender/provider tests.

- [ ] Add failing tests proving create rejects missing/inactive categories, update preserves an assigned inactive category, and custom marketing categories enforce subscription-sensitive delivery.
- [ ] Validate category assignment in TemplateService through the category repository.
- [ ] Stamp `settings.category_purpose` during validated writes and restore `category_purpose` on every repository read.
- [ ] Replace runtime `category == marketing` decisions with `EffectiveTemplateCategoryPurpose`.
- [ ] Run template repository/service and automation executor regressions.

### Task 6: Console API, shared selector and category manager

**Files:**
- Create: `console/src/services/api/templateCategories.ts`
- Create: `console/src/services/api/templateCategories.test.ts`
- Create: `console/src/components/templates/TemplateCategorySelect.tsx`
- Create: `console/src/components/templates/TemplateCategorySelect.test.tsx`
- Create: `console/src/components/templates/TemplateCategoryManager.tsx`
- Create: `console/src/components/templates/TemplateCategoryManager.test.tsx`
- Modify: all three template editor drawers.

- [ ] Add failing API and UI tests for dynamic options, inactive current value, create/edit/deactivate and protected delete.
- [ ] Implement the API client and shared category query/label helpers.
- [ ] Implement the manager drawer and shared selector.
- [ ] Replace hard-coded editor category options.
- [ ] Run focused component tests.

### Task 7: Dynamic Template Management filters and final verification

**Files:**
- Modify: `console/src/pages/TemplatesPage.tsx`
- Create or modify: `console/src/pages/TemplatesPage.test.tsx`
- Modify semantic category-purpose checks in template/broadcast UI.
- Modify `openapi` source and bundle.

- [ ] Add failing page tests for dynamic ordered filters and manager visibility by permission.
- [ ] Build filters from the category API and render category names/usage safely.
- [ ] Replace UI marketing-name comparisons with category purpose plus legacy fallback.
- [ ] Extract/compile translations and update OpenAPI.
- [ ] Run focused backend tests, frontend tests, TypeScript production build, ESLint and `git diff --check`.
- [ ] Perform signed-in browser create/select/filter/deactivate acceptance if authentication is available; otherwise report the gate without fabricating login.
