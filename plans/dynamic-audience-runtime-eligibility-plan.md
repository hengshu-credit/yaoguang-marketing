# Dynamic Audience Runtime Eligibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add condition-tree audiences that are evaluated into a candidate snapshot when a marketing run starts and rechecked against current customer facts before every broadcast or automation touch.

**Architecture:** Audience versions own structured condition trees but no live membership. A marketing run resolves the Audience active version at actual execution time, creates one immutable candidate build, and persists the exact version/build on the run. A shared eligibility evaluator reuses the same safe query compiler for just-in-time checks; false is a terminal skip for the touch, while errors remain retryable.

**Tech Stack:** Go 1.25, PostgreSQL 17, stdlib HTTP, React 18, TypeScript, Ant Design, TanStack Query/Router, Vitest, LinguiJS.

**Spec:** `docs/superpowers/specs/2026-08-31-dynamic-audience-runtime-eligibility-design.md`

## Global Constraints

- Work directly on `main` only because the user explicitly authorized it; do not stage or commit.
- Preserve all pre-existing dirty files and hunks, especially the broadcast data-feed, runtime dependency, settings/setup, development script, and Chinese catalog edits.
- Audience configuration stores only filter logic and versions; it must not start a membership build.
- Scheduled runs resolve the Audience latest active version only when execution actually begins.
- A run never switches rule version after resolution.
- A failed eligibility query is retryable; only an explicit false result is `audience_no_longer_matched`.
- An automation eligibility skip advances to the next node and does not exit the journey.
- All client-provided values are parameterized and all fields/operators come from server whitelists.
- Every user-facing Console string is wrapped by Lingui.

---

### Task 1: Add condition leaves and customer-aware query compilation

**Files:**
- Modify: `internal/domain/audience.go`
- Modify: `internal/domain/audience_test.go`
- Modify: `internal/service/query_builder.go`
- Modify: `internal/service/query_builder_test.go`

**Interfaces:**
- Produces: `AudienceExpression.Condition *TreeNode`.
- Produces: `QueryBuilder.BuildCustomerIDSQL(tree *domain.TreeNode, placeholderOffset int) (string, []interface{}, error)`.
- Produces: `QueryBuilder.BuildCustomerMatchSQL(tree *domain.TreeNode, customerIDPlaceholder int) (string, []interface{}, error)`.

- [ ] **Step 1: Write failing expression tests**

Add literal fixtures proving `{condition: tree}` validates and hashes, and proving condition plus ref/composite is rejected:

```go
func TestAudienceExpressionAcceptsExactlyOneConditionLeaf(t *testing.T) {
    tree := validAudienceConditionTree()
    require.NoError(t, (AudienceExpression{Condition: tree}).Validate())
    assert.ErrorContains(t, (AudienceExpression{Condition: tree, LeafType: AudienceLeafList, RefID: "news"}).Validate(), "exactly one")
}
```

- [ ] **Step 2: Run the focused domain test and verify RED**

Run: `go test ./internal/domain -run 'AudienceExpression.*Condition'`

Expected: compile failure because `Condition` is absent.

- [ ] **Step 3: Implement the tagged-union validation**

Count reference, condition, and composite shapes explicitly. Validate the nested `TreeNode`, preserve canonical JSON hashing, and leave existing reference/composite JSON unchanged.

- [ ] **Step 4: Write failing QueryBuilder tests**

Use a contact status/tree literal and expect:

```go
sql, args, err := NewQueryBuilder().BuildCustomerIDSQL(tree, 2)
require.NoError(t, err)
assert.Contains(t, sql, "SELECT DISTINCT contacts.customer_id FROM contacts")
assert.Contains(t, sql, "$3")
assert.Equal(t, []interface{}{"unpaid"}, args)
```

Also assert the match query adds `contacts.customer_id = $N` without interpolating the ID.

- [ ] **Step 5: Run QueryBuilder tests and verify RED**

Run: `go test ./internal/service -run 'QueryBuilder.*Customer'`

Expected: compile failure because the methods are absent.

- [ ] **Step 6: Implement minimal customer-ID and match compilation**

Reuse `parseNode(tree, placeholderOffset+1)` so every existing field/event semantic and whitelist stays shared. Return only non-null, distinct Customer IDs. Do not route through persisted Segment SQL.

- [ ] **Step 7: Verify Task 1**

Run: `go test ./internal/domain ./internal/service -run 'AudienceExpression|QueryBuilder'`

Expected: PASS.

---

### Task 2: Compile, preview, build, and recheck condition audiences

**Files:**
- Modify: `internal/domain/audience.go`
- Modify: `internal/repository/audience_postgres.go`
- Modify: `internal/repository/audience_postgres_test.go`
- Modify: `internal/service/audience_service.go`
- Modify: `internal/service/audience_service_test.go`
- Modify: `internal/app/app.go`

**Interfaces:**
- Produces repository compiler callback:

```go
type AudienceConditionCompiler func(*domain.TreeNode, int) (string, []interface{}, error)
```

- Produces repository method `BuildAudienceSnapshot(ctx, workspaceID, audienceID string, version int) (*domain.AudienceBuild, error)`.
- Produces service method `MatchesCustomer(ctx, workspaceID, audienceID string, version int, customerID string) (bool, error)`.

- [ ] **Step 1: Write failing compiler/repository tests**

Tests must prove a condition leaf uses the injected compiler with the current placeholder offset, preview returns customer rows/count, and a missing compiler returns an explicit error instead of panicking.

- [ ] **Step 2: Run repository tests and verify RED**

Run: `go test ./internal/repository -run 'Audience.*Condition|Audience.*Snapshot'`

Expected: compile/test failure because condition compilation is unsupported.

- [ ] **Step 3: Add condition compiler injection**

Add `SetConditionCompiler` on the concrete repository and let `audienceSQLCompiler` append the callback arguments. Wire `NewQueryBuilder().BuildCustomerIDSQL` in `internal/app/app.go` after repository construction.

- [ ] **Step 4: Write failing atomic snapshot tests**

Expect one transaction to insert the build, execute an `INSERT INTO audience_memberships ... SELECT ... FROM (<compiled>)`, update count/status, and commit. Assert rollback and failed status on query error.

- [ ] **Step 5: Run snapshot tests and verify RED**

Run: `go test ./internal/repository -run 'Audience.*Snapshot'`

Expected: failure because only chunked builds exist.

- [ ] **Step 6: Implement the atomic runtime snapshot**

Create a build with `building`, insert all matching distinct IDs in one transaction with stable ordinal assignment, update `member_count` and `completed_at`, and never update `audiences.active_build_id` for a runtime snapshot.

- [ ] **Step 7: Write failing service tests for configuration and eligibility**

Assert `Create` and `UpdateDefinition` do not call build; `ResolveLatestAndBuild` reads active version at call time; `MatchesCustomer` reads the named version and propagates repository errors separately from false.

- [ ] **Step 8: Implement service orchestration and verify Task 2**

Run: `go test ./internal/repository ./internal/service -run 'Audience'`

Expected: PASS.

---

### Task 3: Persist exact runtime Audience resolution

**Files:**
- Create: `internal/migrations/v53.go`
- Create: `internal/migrations/v53_test.go`
- Modify: `internal/database/schema/marketing_tables.go`
- Modify: `internal/database/schema/marketing_tables_test.go`
- Modify: `config/config.go`
- Modify: `CHANGELOG.md`
- Modify: `internal/domain/campaign.go`
- Modify: `internal/domain/campaign_test.go`
- Modify: `internal/repository/campaign_postgres.go`
- Modify: `internal/repository/campaign_postgres_test.go`

**Interfaces:**
- `CampaignRun` gains `AudienceID`, `AudienceVersion`, and `AudienceBuildID`.
- Campaign configuration accepts an Audience ID without requiring a version; the run owns resolved version/build.
- Repository consumes exactly `CampaignRun.AudienceBuildID`, never the latest completed build.

- [ ] **Step 1: Write failing schema and migration tests**

Pin additive columns and indexes:

```sql
ALTER TABLE campaign_runs ADD COLUMN IF NOT EXISTS audience_id UUID;
ALTER TABLE campaign_runs ADD COLUMN IF NOT EXISTS audience_version INTEGER;
ALTER TABLE campaign_runs ADD COLUMN IF NOT EXISTS audience_build_id UUID;
```

Add the same columns to fresh schema creation, foreign keys to Audience version/build, and bump `VERSION` from `52.0` to `53.0`.

- [ ] **Step 2: Run migration/schema tests and verify RED**

Run: `go test ./internal/migrations ./internal/database/schema -run 'V53|CampaignRunAudience'`

Expected: failure because V53 and columns are absent.

- [ ] **Step 3: Implement idempotent V53 migration and fresh schema**

Follow the existing major migration registration pattern. Do not edit V52. Add one unreleased Feature bullet to the current Changelog version.

- [ ] **Step 4: Write failing Campaign domain/repository tests**

Prove Audience configuration may omit version, runtime fields round-trip, and member lookup uses the exact build ID:

```go
assert.Contains(t, query, "membership.build_id = NULLIF($1, '')::uuid")
```

- [ ] **Step 5: Implement the resolved-run contract and verify Task 3**

Run: `go test ./internal/domain ./internal/repository ./internal/migrations ./internal/database/schema -run 'Campaign|V53'`

Expected: PASS.

---

### Task 4: Resolve and freeze Audience when Broadcast execution starts

**Files:**
- Modify: `internal/service/campaign_service.go`
- Modify: `internal/service/campaign_service_test.go`
- Modify: `internal/service/broadcast_service.go`
- Modify: `internal/service/broadcast_service_test.go`
- Modify: `internal/service/broadcast/orchestrator.go`
- Modify: `internal/service/broadcast/orchestrator_process_test.go`
- Modify: `internal/service/broadcast/factory.go`
- Modify: `internal/app/app.go`

**Interfaces:**
- Produces `AudienceRunResolver.ResolveBroadcastAudience(ctx, workspaceID string, broadcast *domain.Broadcast) (*domain.CampaignRun, error)`.
- `BroadcastService.ScheduleBroadcast` no longer prepares dynamic Audience snapshots.
- `BroadcastOrchestrator.Process` resolves/prepares once at the first actual task execution.

- [ ] **Step 1: Write failing scheduling tests**

Assert scheduling an Audience-targeted Broadcast creates the send task without calling Campaign preparation and leaves `campaign_run_id` empty.

- [ ] **Step 2: Run the scheduling test and verify RED**

Run: `go test ./internal/service -run 'ScheduleBroadcast.*Audience.*Execution'`

Expected: failure because scheduling currently prepares the snapshot.

- [ ] **Step 3: Remove eager dynamic snapshot preparation**

Keep list compatibility behavior only where it is already required; dynamic Audience resolution moves to actual execution.

- [ ] **Step 4: Write failing orchestrator tests**

The first `Process` call must resolve latest version, create one build/run, persist `campaign_run_id`, and return incomplete until the snapshot is dispatching. A repeated call reuses the persisted run and does not resolve again.

- [ ] **Step 5: Implement execution-time resolution**

Inject a small resolver interface into the orchestrator/factory without coupling the broadcast package to the full Audience service. Preserve existing constructor callers through an option or nil-safe trailing dependency.

- [ ] **Step 6: Verify Task 4**

Run: `go test ./internal/service ./internal/service/broadcast -run 'Broadcast.*Audience|Audience.*Broadcast|CampaignSnapshot'`

Expected: PASS.

---

### Task 5: Apply just-in-time eligibility to Broadcast touches

**Files:**
- Modify: `internal/domain/delivery.go`
- Modify: `internal/domain/delivery_test.go`
- Modify: `internal/service/broadcast/queue_message_sender.go`
- Modify: `internal/service/broadcast/queue_message_sender_delivery_test.go`
- Modify: `internal/service/broadcast/message_sender.go`
- Modify: `internal/service/broadcast/factory.go`
- Modify: `internal/app/app.go`

**Interfaces:**
- Produces `AudienceEligibilityChecker.MatchesCustomer(ctx, workspaceID, audienceID string, version int, customerID string) (bool, error)`.
- Delivery/queue context carries Audience ID/version/build and terminal skip reason.

- [ ] **Step 1: Write failing sender tests**

Cover three observable outcomes:

```go
// true: one delivery intent/queue entry
// false: zero queue entries and one terminal audience_no_longer_matched decision
// error: zero terminal skip and the error is returned for retry
```

Repeat the false attempt and assert it never becomes a send after the checker changes to true.

- [ ] **Step 2: Run sender tests and verify RED**

Run: `go test ./internal/service/broadcast -run 'Eligibility|AudienceNoLongerMatched'`

Expected: compile/test failure because no eligibility checker exists.

- [ ] **Step 3: Implement the eligibility gate immediately before intent/enqueue**

Use the exact version from Campaign Run. List-only broadcasts skip this dynamic Audience gate. Record false as a deterministic terminal outcome using the existing delivery effect key; return errors unchanged.

- [ ] **Step 4: Verify Task 5**

Run: `go test ./internal/domain ./internal/service/broadcast -run 'Delivery|Eligibility|QueueMessageSender'`

Expected: PASS.

---

### Task 6: Start Automation journeys from a candidate snapshot and recheck each touch

**Files:**
- Modify: `internal/domain/automation.go`
- Modify: `internal/domain/automation_test.go`
- Modify: `internal/repository/automation_postgres.go`
- Modify: `internal/repository/automation_postgres_test.go`
- Create: `internal/service/automation_audience_run_service.go`
- Create: `internal/service/automation_audience_run_service_test.go`
- Modify: `internal/service/automation_executor.go`
- Modify: `internal/service/automation_executor_test.go`
- Modify: `internal/service/automation_node_executor.go`
- Modify: `internal/service/automation_node_executor_test.go`
- Modify: `internal/http/automation_handler.go`
- Modify: `internal/http/automation_handler_test.go`
- Modify: `internal/app/app.go`

**Interfaces:**
- Produces `POST /api/automations.startAudience` with `workspace_id`, `automation_id`, and `audience_id`.
- Journey context persists `audience_id`, `audience_version`, `audience_build_id`, and `audience_customer_id`.
- Repository gains idempotent `CreateAudienceJourney` keyed by build/customer/automation.

- [ ] **Step 1: Write failing start-run service tests**

Assert the service requires a live Automation, resolves/builds the Audience at call time, creates one journey per snapshot customer, sets the trigger's next node, and produces no duplicates when retried.

- [ ] **Step 2: Run start-run tests and verify RED**

Run: `go test ./internal/service -run 'AutomationAudienceRun'`

Expected: compile failure because the service is absent.

- [ ] **Step 3: Add repository enrollment and run service**

Create ContactAutomation rows with deterministic IDs derived from automation/build/customer, include the resolved Audience context, increment enrolled only for inserted rows, and schedule the existing executor.

- [ ] **Step 4: Write failing executor eligibility tests**

For Email, SMS, and Push nodes, assert false produces completed node output:

```json
{"skipped": true, "skip_reason": "audience_no_longer_matched"}
```

with `NextNodeID` unchanged. Assert a later touch invokes the checker again. Non-touch nodes must not call the checker. Checker errors must fail the node for retry.

- [ ] **Step 5: Implement the common touch gate**

Gate in `AutomationExecutor` before dispatching a touch executor so Email/SMS/Push share identical behavior and node executors remain focused. Use Customer ID from journey context and the stored Audience version; a journey without Audience context preserves existing behavior.

- [ ] **Step 6: Add HTTP contract tests and handler**

Require Automations write and Segments/Contacts read through service authorization. Return run/build/version/candidate count.

- [ ] **Step 7: Verify Task 6**

Run: `go test ./internal/domain ./internal/repository ./internal/service ./internal/http -run 'Automation.*Audience|Audience.*Automation|Eligibility'`

Expected: PASS.

---

### Task 7: Build the Audience condition editor and marketing entry points

**Files:**
- Create: `console/src/pages/AudiencesPage.tsx`
- Create: `console/src/pages/AudiencesPage.test.tsx`
- Create: `console/src/components/audiences/AudienceDrawer.tsx`
- Create: `console/src/components/audiences/AudienceDrawer.test.tsx`
- Modify: `console/src/services/api/marketing.ts`
- Modify: `console/src/services/api/marketing.test.ts`
- Modify: `console/src/router.tsx`
- Modify: `console/src/layouts/WorkspaceLayout.tsx`
- Modify: `console/src/__tests__/WorkspaceLayout.test.tsx`
- Modify: `console/src/components/broadcasts/UpsertBroadcastDrawer.tsx`
- Modify: `console/src/components/broadcasts/UpsertBroadcastDrawer.test.tsx`
- Modify: `console/src/pages/AutomationsPage.tsx`
- Modify: `console/src/pages/AutomationsPage.test.tsx`

**Interfaces:**
- `AudienceExpression` gains `condition?: TreeNode`.
- `/audiences` renders `AudiencesPage` rather than redirecting.
- Broadcast submission supports `{ audience_id }` without client-selected version/build.
- `automationApi.startAudience(workspaceId, automationId, audienceId)` starts a run.

- [ ] **Step 1: Write failing API contract tests**

Assert create/update/preview preserve the structured condition tree and Automation start posts the exact IDs without SQL or a client version.

- [ ] **Step 2: Run API tests and verify RED**

Run: `npm test -- --run src/services/api/marketing.test.ts src/services/api/automation.test.ts`

Expected: compile/test failure on the condition/start contracts.

- [ ] **Step 3: Implement typed API contracts**

Import the existing segment `TreeNode` interface instead of duplicating loose `Record<string, unknown>` shapes.

- [ ] **Step 4: Write failing Audience drawer/page tests**

Tests must prove:

- editing a complete draft tree debounces preview of the whole draft;
- an incomplete draft makes no request and dims the last valid total;
- save calls create/update only and never build;
- the page lists current versions and handles empty/error states;
- route and sidebar point to `/audiences` while `/lists` remains list management.

- [ ] **Step 5: Run Audience UI tests and verify RED**

Run: `npm test -- --run src/components/audiences/AudienceDrawer.test.tsx src/pages/AudiencesPage.test.tsx src/__tests__/WorkspaceLayout.test.tsx`

Expected: compile/test failure because the page/drawer are absent and route redirects.

- [ ] **Step 6: Implement the editor and navigation**

Reuse `TreeNodeInput`, `isTreeQueryable`, `onDraftTreeChange`, workspace timezone, TanStack Query invalidation, and Lingui. Show the last valid count during incomplete/error states.

- [ ] **Step 7: Write failing Broadcast and Automation entry tests**

Assert dynamic Audience selection clears list/version/build fields and shows the execution-time/JIT warning. Assert a live Automation can select an Audience, preview candidate count, confirm, and call `startAudience`.

- [ ] **Step 8: Implement both entry points and verify Task 7**

Run: `npm test -- --run src/services/api/marketing.test.ts src/components/audiences/AudienceDrawer.test.tsx src/pages/AudiencesPage.test.tsx src/components/broadcasts/UpsertBroadcastDrawer.test.tsx src/pages/AutomationsPage.test.tsx src/__tests__/WorkspaceLayout.test.tsx`

Expected: PASS.

---

### Task 8: OpenAPI, integration regression, and complete verification

**Files:**
- Modify: `openapi/components/schemas/audience.yaml`
- Modify: `openapi/paths/audiences.yaml`
- Modify: `openapi/paths/journeys.yaml`
- Modify: `openapi/components/schemas/journey.yaml`
- Modify: `openapi/openapi.yaml`
- Modify: `openapi.json`
- Create: `tests/integration/audience_runtime_eligibility_test.go`
- Modify carefully: `console/src/i18n/locales/zh-CN.po`
- Modify carefully: `console/src/i18n/locales/zh-CN.js`

**Interfaces:**
- OpenAPI documents the condition leaf, execution-time latest-version resolution, Automation start, and skip reason.

- [ ] **Step 1: Add the failing integration regression**

The scenario creates an unpaid customer, saves an unpaid condition Audience, starts a run, then changes the customer to paid before dispatch. Assert the customer remains in the candidate snapshot, no message intent/queue row is created, and the touch is recorded as `audience_no_longer_matched`.

- [ ] **Step 2: Run the integration test and verify RED**

Run: `go test ./tests/integration -run 'AudienceRuntimeEligibility'`

Expected: failure before the complete runtime path is wired.

- [ ] **Step 3: Complete wiring until the integration test passes**

Fix only missing feature wiring exposed by the regression. Do not weaken assertions or convert a query error into a skip.

- [ ] **Step 4: Update OpenAPI source and generated aggregate**

Add `/api/automations.startAudience` to `openapi/openapi.yaml`, define its operation in `openapi/paths/journeys.yaml`, and add request/response schemas in `openapi/components/schemas/journey.yaml`. Run `make openapi-bundle` to regenerate `openapi.json`, then `make openapi-lint`.

- [ ] **Step 5: Update translations without overwriting existing catalog hunks**

Run extraction only after saving a baseline diff of both Chinese catalog files. Merge only new messages and compile. Restore no user lines.

- [ ] **Step 6: Run focused backend verification**

Run: `go test ./internal/domain ./internal/repository ./internal/service ./internal/http ./internal/migrations ./internal/database/schema`

Run: `go test ./tests/integration -run 'Audience|Broadcast|Automation'`

Expected: PASS. If Go remains unavailable, record the environment gate and use compile/diff checks without claiming backend tests passed.

- [ ] **Step 7: Run frontend verification**

Run: `npm test -- --run src/components/audiences src/pages/AudiencesPage.test.tsx src/components/broadcasts/UpsertBroadcastDrawer.test.tsx src/pages/AutomationsPage.test.tsx src/services/api/marketing.test.ts src/__tests__/WorkspaceLayout.test.tsx`

Run: `npm run lint`

Run: `npm run build`

Expected: all focused tests, lint, and build exit 0. Keep the six unrelated full-suite baseline timeouts separate.

- [ ] **Step 8: Perform rendered browser validation**

Open `/audiences`, create conditions for profile status and an event, confirm the count updates for committed and draft changes, save without a build, select it in a Broadcast, and open the Automation Audience-start flow. Check loading, empty, preview error, zero-count, permission-disabled, and 375px layouts for overflow/clipping.

- [ ] **Step 9: Audit the final diff**

Run: `git diff --check`

Run: `git status --short --branch`

Compare with the initial dirty-file inventory. Report every pre-existing modified/untracked file that remains untouched and do not stage or commit.
