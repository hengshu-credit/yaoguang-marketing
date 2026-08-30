# Console Information Architecture and Customer Workflows Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Unify analytics page hierarchy, restore an editable two-column Customer 360, make lists the only audience workspace, and support list-native campaign snapshots plus durable imports bound to lists.

**Architecture:** React pages share focused layout and workflow components while the Go domain keeps Audience compatibility and adds first-class list sources where the console needs them. Database migration v52 evolves campaign versions and import jobs without editing prior migrations; Customer upsert gains an additive membership patch so imports never erase or reactivate existing memberships.

**Tech Stack:** React 18, TypeScript, Ant Design 6, TanStack Router/Query, LinguiJS, Vitest/Testing Library, Go, PostgreSQL 17, sqlmock.

**Spec:** `docs/superpowers/specs/2026-08-31-console-information-architecture-and-customer-workflows-design.md`

## Global Constraints

- Work directly on the current `main` branch; do not create a branch or worktree.
- Preserve all pre-existing uncommitted delivery-center, catalog, and v46 changes.
- Do not delete Audience tables or public Audience APIs.
- Do not change English menu names, permission resource names, or RPC endpoint names.
- Customer imports add list memberships and never overwrite existing unsubscribed, bounced, complained, or pending states.
- Every behavior change follows red-green-refactor and ends with fresh verification.
- Because the shared Chinese catalogs already contain user changes, never stage or commit mixed catalog files automatically.

---

### Task 1: Add the v52 persistence contract

**Files:**
- Create: `internal/migrations/v52.go`
- Create: `internal/migrations/v52_test.go`
- Modify: `internal/database/schema/marketing_tables.go`
- Modify: `internal/database/schema/marketing_tables_test.go`
- Modify: `config/config.go`

**Interfaces:**
- Produces: nullable `campaign_versions.audience_id`, nullable `campaign_versions.audience_version`, optional `campaign_versions.list_id`, and `import_jobs.list_ids TEXT[] NOT NULL`.
- Constraint: exactly one campaign source is valid: `(audience_id, audience_version)` or `list_id`.

- [ ] **Step 1: Write failing migration and final-schema tests**

```go
func TestV52MigrationAddsListCampaignSourceAndImportBindings(t *testing.T) {
    db, mock, err := sqlmock.New()
    require.NoError(t, err)
    defer db.Close()
    mock.ExpectExec("ALTER TABLE campaign_versions ALTER COLUMN audience_id DROP NOT NULL").WillReturnResult(sqlmock.NewResult(0, 0))
    mock.ExpectExec("ALTER TABLE campaign_versions ALTER COLUMN audience_version DROP NOT NULL").WillReturnResult(sqlmock.NewResult(0, 0))
    mock.ExpectExec("ALTER TABLE campaign_versions ADD COLUMN IF NOT EXISTS list_id").WillReturnResult(sqlmock.NewResult(0, 0))
    mock.ExpectExec("ALTER TABLE campaign_versions DROP CONSTRAINT IF EXISTS campaign_versions_source_check").WillReturnResult(sqlmock.NewResult(0, 0))
    mock.ExpectExec("ALTER TABLE campaign_versions ADD CONSTRAINT campaign_versions_source_check").WillReturnResult(sqlmock.NewResult(0, 0))
    mock.ExpectExec("ALTER TABLE import_jobs ADD COLUMN IF NOT EXISTS list_ids").WillReturnResult(sqlmock.NewResult(0, 0))
    require.NoError(t, (&V52Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
    require.NoError(t, mock.ExpectationsWereMet())
}
```

Extend the schema test to assert `list_id VARCHAR(32)`, `list_ids TEXT[]`, and the source check.

- [ ] **Step 2: Run the tests and verify RED**

Run: `go test ./internal/migrations ./internal/database/schema`

Expected: failure because V52 and the final schema columns do not exist.

- [ ] **Step 3: Implement the migration and final schema**

Use idempotent `ALTER TABLE` statements in `V52Migration.UpdateWorkspace`, register it with `init()`, and change `config.VERSION` from `51.0` to `52.0`. In fresh schema DDL, make the Audience pair nullable, add `list_id`, add the exclusive source constraint, and add `list_ids` to import jobs.

- [ ] **Step 4: Run migration/schema tests and verify GREEN**

Run: `go test ./internal/migrations ./internal/database/schema`

Expected: PASS.

- [ ] **Step 5: Checkpoint without staging user changes**

Run: `git diff --check -- internal/migrations/v52.go internal/migrations/v52_test.go internal/database/schema/marketing_tables.go internal/database/schema/marketing_tables_test.go config/config.go`

---

### Task 2: Add additive Customer list memberships and durable import bindings

**Files:**
- Modify: `internal/domain/customer.go`
- Modify: `internal/domain/customer_test.go`
- Modify: `internal/domain/import_job.go`
- Modify: `internal/repository/customer_postgres.go`
- Modify: `internal/repository/customer_postgres_test.go`
- Modify: `internal/repository/import_job_postgres.go`
- Modify: `internal/service/import_job_service.go`
- Modify: `internal/service/import_job_service_test.go`
- Modify: `internal/http/import_job_handler.go`
- Modify: `internal/http/import_job_handler_test.go`
- Modify: `openapi.json`

**Interfaces:**
- Produces: `CustomerUpsertInput.ListMembershipsAdd *[]CustomerListMembershipInput` serialized as `list_memberships_add`.
- Produces: `ImportJob.ListIDs []string` and `StageCSV(ctx, workspaceID, filename, listIDs, source)`.
- HTTP contract: repeated `list_id` query parameters on `/api/imports.upload`.

- [ ] **Step 1: Write failing domain tests for additive membership validation**

```go
func TestCustomerUpsertInputValidatesAdditiveMembershipsIndependently(t *testing.T) {
    externalID := "customer-1"
    memberships := []CustomerListMembershipInput{{ListID: "news"}, {ListID: "vip", Status: "active"}}
    input := CustomerUpsertInput{ExternalUserID: &externalID, ListMembershipsAdd: &memberships}
    require.NoError(t, input.Validate())
    assert.Equal(t, "active", (*input.ListMembershipsAdd)[0].Status)
}
```

Add rejection cases for duplicates inside the additive set and for the same list appearing in replace and add sets.

- [ ] **Step 2: Run the focused domain test and verify RED**

Run: `go test ./internal/domain -run 'TestCustomerUpsertInputValidatesAdditiveMemberships'`

Expected: compile failure because `ListMembershipsAdd` is absent.

- [ ] **Step 3: Implement the additive domain contract**

Reuse one membership-normalization helper for replace and add fields. Include the additive field in canonical payload hashing through normal JSON serialization.

- [ ] **Step 4: Write failing repository tests for suppression-safe additive writes**

Expect an insert shaped as:

```sql
INSERT INTO customer_list_memberships (customer_id, list_id, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $4)
ON CONFLICT (customer_id, list_id) DO NOTHING
```

Also expect the contact-list compatibility projection to use `ON CONFLICT DO NOTHING` so an existing suppression state is never reset.

- [ ] **Step 5: Run the focused repository tests and verify RED**

Run: `go test ./internal/repository -run 'Additive|Membership'`

Expected: failure because the repository ignores the additive field.

- [ ] **Step 6: Implement additive writes in the Customer repository**

Apply full replacement first when present, then additive inserts. Keep both customer and contact-list projections in the same transaction.

- [ ] **Step 7: Write failing import service/repository/handler tests**

Tests must prove:

```go
job := domain.ImportJob{ListIDs: []string{"news", "vip"}}
// Create/Get/List round-trip both IDs.
// A staged row produces ListMembershipsAdd with active news and vip.
// Repeated ?list_id=news&list_id=vip reaches StageCSV in order.
```

- [ ] **Step 8: Run focused import tests and verify RED**

Run: `go test ./internal/repository ./internal/service ./internal/http -run 'Import.*List|ImportJob'`

Expected: failures because jobs do not store target lists and StageCSV has no list argument.

- [ ] **Step 9: Implement durable import list bindings**

Validate each list ID with the existing alphanumeric/32-character rule, deduplicate while preserving selection order, store with `pq.Array`, read with `pq.Array`, and pass the job lists into `importRowToCustomer`. Update OpenAPI request parameters and `ImportJob` schema.

- [ ] **Step 10: Verify the complete Customer/import slice**

Run: `go test ./internal/domain ./internal/repository ./internal/service ./internal/http -run 'Customer|Import'`

Expected: PASS.

---

### Task 3: Make campaign snapshots list-native

**Files:**
- Modify: `internal/domain/campaign.go`
- Modify: `internal/domain/campaign_test.go`
- Modify: `internal/repository/campaign_postgres.go`
- Create: `internal/repository/campaign_postgres_test.go`
- Modify: `internal/service/campaign_service.go`
- Modify: `internal/service/campaign_snapshot_service.go`
- Modify: `internal/service/campaign_snapshot_service_test.go`
- Modify: `internal/service/campaign_snapshot_worker_test.go`
- Modify: `internal/service/broadcast_service.go`
- Modify: `internal/service/broadcast_service_test.go`

**Interfaces:**
- Produces: `CampaignVersion.ListID string`.
- Replaces repository member lookup with `ListCampaignMembers(ctx, workspaceID string, version CampaignVersion, after string, limit int)`.
- `PrepareBroadcast` accepts either a versioned Audience or `broadcast.Audience.List`.

- [ ] **Step 1: Write failing CampaignVersion source-validation tests**

```go
func TestCampaignVersionAcceptsExactlyOneRecipientSource(t *testing.T) {
    validList := validCampaignVersion()
    validList.AudienceID, validList.AudienceVersion, validList.ListID = "", 0, "news"
    require.NoError(t, validList.Validate())

    both := validList
    both.AudienceID, both.AudienceVersion = "audience-1", 1
    assert.ErrorContains(t, both.Validate(), "exactly one")
}
```

- [ ] **Step 2: Run domain tests and verify RED**

Run: `go test ./internal/domain -run 'CampaignVersion'`

Expected: compile failure because `ListID` is absent.

- [ ] **Step 3: Implement the source union and repository persistence**

Persist nullable Audience fields and `list_id`; scan nullable values back into the domain. Preserve all existing Audience-backed versions.

- [ ] **Step 4: Write failing snapshot tests for active list members**

The repository query must select only:

```sql
SELECT membership.customer_id, ''
FROM customer_list_memberships membership
WHERE membership.list_id = $1 AND membership.status = 'active'
  AND membership.customer_id > COALESCE(NULLIF($2, '')::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
ORDER BY membership.customer_id LIMIT $3
```

Service tests must prove list and Audience sources both page into the same snapshot table.

- [ ] **Step 5: Run snapshot tests and verify RED**

Run: `go test ./internal/repository ./internal/service -run 'Campaign|Snapshot|PrepareBroadcast'`

Expected: failures because only Audience members are supported.

- [ ] **Step 6: Implement list-native snapshot preparation**

Generalize member lookup by CampaignVersion, prepare a Campaign Run for either source, and change Broadcast scheduling to prepare whenever no run exists and either a list or Audience is configured.

- [ ] **Step 7: Verify campaign and broadcast behavior**

Run: `go test ./internal/domain ./internal/repository ./internal/service -run 'Campaign|Broadcast'`

Expected: PASS with both source types and immutable snapshots covered.

---

### Task 4: Consolidate navigation and the lists workspace

**Files:**
- Modify: `console/src/layouts/WorkspaceLayout.tsx`
- Modify: `console/src/__tests__/WorkspaceLayout.test.tsx`
- Modify: `console/src/router.tsx`
- Modify: `console/src/pages/ListsPage.tsx`
- Create: `console/src/pages/ListsPage.test.tsx`
- Modify: `console/src/components/lists/ListStats.tsx`
- Delete: `console/src/pages/AudiencesPage.tsx`
- Delete: `console/src/pages/AudiencesPage.test.tsx`

**Interfaces:**
- Sidebar Audiences item links to `/lists` and remains selected for `/audiences` compatibility paths.
- `/audiences` performs a replace redirect to `/lists`.
- ListsPage owns list management and live counts only.

- [ ] **Step 1: Write failing navigation, redirect, and list-page tests**

Assert that the first accessible menu link is Data Analytics, the Audiences link targets `/lists`, and the Lists page has create/refresh/count behavior but no “Audience definition” or “Batch import” tab.

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `cd console; npm test -- --run src/__tests__/WorkspaceLayout.test.tsx src/pages/ListsPage.test.tsx`

Expected: failures on menu order, route target, and missing list-page test behaviors.

- [ ] **Step 3: Implement navigation, redirect, and focused ListsPage**

Use TanStack Router `beforeLoad` plus `throw redirect({ replace: true, ... })` for `/audiences`. Remove the old import icon from list cards. Configure stats to refetch on focus, poll every 10 seconds, and expose an explicit refresh that invalidates both list and stat queries.

- [ ] **Step 4: Run the focused tests and verify GREEN**

Run: `cd console; npm test -- --run src/__tests__/WorkspaceLayout.test.tsx src/pages/ListsPage.test.tsx`

Expected: PASS.

---

### Task 5: Standardize all Data Analytics page hierarchy

**Files:**
- Create: `console/src/components/navigation/DataAnalyticsPageShell.tsx`
- Create: `console/src/components/navigation/DataAnalyticsPageShell.test.tsx`
- Modify: `console/src/components/navigation/WorkspaceSectionTabs.tsx`
- Modify: `console/src/pages/AnalyticsPage.tsx`
- Modify: `console/src/pages/WebAnalyticsPage.tsx`
- Modify: `console/src/pages/WebAnalyticsLivePage.tsx`
- Modify: `console/src/components/web_analytics/tabs/FiltersTab.tsx`
- Modify: `console/src/components/web_analytics/tabs/AnnotationsTab.tsx`
- Modify: `console/src/__tests__/pages.smoke.test.tsx`
- Modify: `console/src/components/navigation/WorkspaceSectionTabs.test.tsx`

**Interfaces:**
- `DataAnalyticsPageShell({ workspaceId, activeKey, actions?, toolbar?, children })` renders one h1, one description, tabs, optional toolbar, then content.
- The shell owns all section title and description translations.

- [ ] **Step 1: Write failing shell tests for all seven descriptors and DOM order**

```tsx
const order = within(container).getAllByTestId(/analytics-(header|tabs|toolbar|content)/)
expect(order.map((node) => node.dataset.testid)).toEqual([
  'analytics-header', 'analytics-tabs', 'analytics-toolbar', 'analytics-content'
])
expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1)
```

Render each key and assert its unique title and non-empty description.

- [ ] **Step 2: Run focused tests and verify RED**

Run: `cd console; npm test -- --run src/components/navigation/DataAnalyticsPageShell.test.tsx src/components/navigation/WorkspaceSectionTabs.test.tsx src/__tests__/pages.smoke.test.tsx`

Expected: compile failure because the shell is absent.

- [ ] **Step 3: Implement the shared shell and migrate pages**

Move Marketing timezone/range actions into shell actions; move Explore export into shell actions; pass report controls as toolbar; replace Live back-link with live status actions; remove duplicate inner h2 blocks from Filters and Annotations.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run: `cd console; npm test -- --run src/components/navigation/DataAnalyticsPageShell.test.tsx src/components/navigation/WorkspaceSectionTabs.test.tsx src/__tests__/pages.smoke.test.tsx`

Expected: PASS.

---

### Task 6: Add Customer update API and rebuild Customer 360

**Files:**
- Modify: `console/src/services/api/customer.ts`
- Modify: `console/src/services/api/customer.test.ts`
- Modify: `console/src/pages/CustomersPage.tsx`
- Modify: `console/src/pages/CustomersPage.test.tsx`
- Modify: `console/src/components/customers/CustomerDrawer.tsx`
- Modify: `console/src/components/customers/CustomerDrawer.test.tsx`
- Create: `console/src/components/customers/CustomerProfilePanel.tsx`
- Create: `console/src/components/customers/CustomerProfilePanel.test.tsx`

**Interfaces:**
- Produces: `customerApi.update(workspaceId, customerId, patch, idempotencyKey)`.
- CustomerProfilePanel consumes Customer, Workspace, write permission, and async field/tag/attribute handlers.

- [ ] **Step 1: Write failing API contract tests**

```ts
await customerApi.update('ws1', 'customer-1', { profile: { status: 'active' } }, 'edit-1')
expect(api.post).toHaveBeenCalledWith('/api/customers.upsert', {
  workspace_id: 'ws1', idempotency_key: 'edit-1',
  customer: { locator: { customer_id: 'customer-1' }, profile: { status: 'active' } }
})
```

- [ ] **Step 2: Run API tests and verify RED**

Run: `cd console; npm test -- --run src/services/api/customer.test.ts`

Expected: compile failure because `update` is absent.

- [ ] **Step 3: Implement the typed update API**

Define patch interfaces without `any`; return the mutation envelope used by the backend.

- [ ] **Step 4: Write failing profile panel and drawer tests**

Tests cover the two-column structure, business display name, masked identity, collapsed system IDs, standard edit, tag save, attribute add/edit/unset, write-permission gating, membership names, and lazy timeline/journey/delivery queries.

- [ ] **Step 5: Run Customer tests and verify RED**

Run: `cd console; npm test -- --run src/components/customers/CustomerProfilePanel.test.tsx src/components/customers/CustomerDrawer.test.tsx src/pages/CustomersPage.test.tsx`

Expected: failures against the old tab-only drawer.

- [ ] **Step 6: Implement the responsive two-column Customer 360**

Use the old Notifuse visual pattern: no-padding 1200px drawer, muted fixed profile rail, flexible activity pane, compact editable rows, and responsive stacking. Use `crypto.randomUUID()` per save, invalidate detail/list queries, and display actionable errors.

- [ ] **Step 7: Run Customer tests and verify GREEN**

Run: `cd console; npm test -- --run src/services/api/customer.test.ts src/components/customers/CustomerProfilePanel.test.tsx src/components/customers/CustomerDrawer.test.tsx src/pages/CustomersPage.test.tsx`

Expected: PASS.

---

### Task 7: Move durable imports into Customer Management and bind lists

**Files:**
- Create: `console/src/components/customers/CustomerImportPanel.tsx`
- Create: `console/src/components/customers/CustomerImportPanel.test.tsx`
- Modify: `console/src/pages/CustomersPage.tsx`
- Modify: `console/src/pages/CustomersPage.test.tsx`
- Modify: `console/src/services/api/marketing.ts`
- Modify: `console/src/services/api/marketing.test.ts`
- Modify: `console/src/components/broadcasts/UpsertBroadcastDrawer.tsx`
- Modify: `console/src/components/broadcasts/UpsertBroadcastDrawer.test.tsx`

**Interfaces:**
- `importJobApi.upload(workspaceId, file, listIds)` appends repeated `list_id` parameters.
- CustomerImportPanel owns list selection, upload, progress, cancellation, history, and error download.
- Broadcast form stores `audience.list` and clears Audience version fields.

- [ ] **Step 1: Write failing API and import-panel tests**

Assert upload URL parameters contain both selected list IDs, the upload button is disabled while lists are loading, zero lists remains valid, history displays bound list names, and upload progress remains close-safe.

- [ ] **Step 2: Run focused tests and verify RED**

Run: `cd console; npm test -- --run src/services/api/marketing.test.ts src/components/customers/CustomerImportPanel.test.tsx src/pages/CustomersPage.test.tsx`

Expected: failures because the panel and list-aware upload are absent.

- [ ] **Step 3: Extract and implement CustomerImportPanel**

Move the durable five-step workflow from the deleted Audience page, wrap all visible strings with Lingui, and show it under the Customers page “Batch Import” tab.

- [ ] **Step 4: Write failing Broadcast list-selector test**

Select `news` and assert submitted payload contains:

```ts
audience: {
  list: 'news',
  audience_id: undefined,
  audience_version: undefined,
  audience_build_id: undefined,
  exclude_unsubscribed: true
}
```

- [ ] **Step 5: Run Broadcast test and verify RED**

Run: `cd console; npm test -- --run src/components/broadcasts/UpsertBroadcastDrawer.test.tsx`

Expected: failure because the current form requires Audience IDs.

- [ ] **Step 6: Implement direct list selection**

Load lists with the existing Lists API, show current list names, require one list, and update validation/tab navigation without changing the English API field names.

- [ ] **Step 7: Verify customer import and broadcast UI**

Run: `cd console; npm test -- --run src/services/api/marketing.test.ts src/components/customers/CustomerImportPanel.test.tsx src/pages/CustomersPage.test.tsx src/components/broadcasts/UpsertBroadcastDrawer.test.tsx`

Expected: PASS.

---

### Task 8: Apply Chinese menu translations and complete verification

**Files:**
- Modify carefully: `console/src/i18n/locales/zh-CN.po`
- Regenerate carefully: `console/src/i18n/locales/zh-CN.js`
- Modify carefully: `console/src/i18n/zhCN.priority.test.ts`
- Generated catalogs may change only as required by newly introduced messages; preserve all pre-existing user hunks.

**Interfaces:**
- Chinese-only menu mappings match the design spec.

- [ ] **Step 1: Add failing priority translation assertions**

```ts
expect(i18n._('Customers')).toBe('客户管理')
expect(i18n._('Audiences')).toBe('客群划分')
expect(i18n._('Automation Journeys')).toBe('营销自动化')
expect(i18n._('Content Center')).toBe('素材模板')
```

- [ ] **Step 2: Run the translation test and verify RED**

Run: `cd console; npm test -- --run src/i18n/zhCN.priority.test.ts`

Expected: failures with the previous Chinese values.

- [ ] **Step 3: Extract, update Chinese translations, and compile catalogs**

Run: `cd console; npm run lingui:extract`

Update the four Chinese message values and all new page descriptions, then run: `npm run lingui:compile`.

Inspect catalog diffs against the pre-task baseline so existing delivery-center translations remain intact.

- [ ] **Step 4: Run the complete focused frontend suite**

Run: `cd console; npm test -- --run src/__tests__/WorkspaceLayout.test.tsx src/components/navigation src/components/customers src/components/broadcasts/UpsertBroadcastDrawer.test.tsx src/pages/CustomersPage.test.tsx src/pages/ListsPage.test.tsx src/services/api/customer.test.ts src/services/api/marketing.test.ts src/i18n/zhCN.priority.test.ts src/__tests__/pages.smoke.test.tsx`

Expected: PASS.

- [ ] **Step 5: Run complete backend verification**

Run: `go test ./internal/domain ./internal/repository ./internal/service ./internal/http ./internal/migrations ./internal/database/schema`

Expected: PASS.

- [ ] **Step 6: Run frontend quality gates**

Run: `cd console; npm run lint`

Run: `cd console; npm run build`

Expected: both exit 0 with no warnings promoted to errors.

- [ ] **Step 7: Perform local visual verification**

Open the local console and verify `/analytics`, every data-analysis tab, `/customers`, Customer 360 at desktop and 375px, `/lists`, and Broadcast audience selection. Confirm one page title per analytics tab, consistent order, no horizontal document overflow, editable profile feedback, and import list binding.

- [ ] **Step 8: Audit the final diff**

Run: `git diff --check`

Run: `git status --short --branch`

Compare the final diff with the initial dirty-file list and report which pre-existing user changes remain untouched. Do not stage mixed catalog or v46 changes.
