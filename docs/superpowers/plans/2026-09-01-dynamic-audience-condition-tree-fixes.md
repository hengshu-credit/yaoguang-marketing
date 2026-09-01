# Dynamic Audience Condition Tree Fixes Implementation Plan

> **For agentic workers:** Execute inline in the current session. The repository instructions prohibit subagents when they do not accelerate the work, and the user explicitly authorized implementation on `main` without commits.

**Goal:** Make dynamic-audience conditions save the rule the user sees, evaluate against authoritative Customer data with legacy compatibility, and allow existing definitions to be viewed and versioned from the console.

**Architecture:** Keep legacy Segment SQL unchanged, but give `BuildCustomerIDSQL` and `BuildCustomerMatchSQL` a Customer-rooted compiler that reads canonical profiles, tags, memberships and customer-linked facts while falling back to legacy projections. Enrich `audiences.get` with the active definition and reuse the existing drawer for create and edit. Treat an open condition editor as an uncommitted state, and clear fields that become hidden after an operator/source change.

**Tech Stack:** Go 1.25, PostgreSQL, React 18, TypeScript, Ant Design 6, TanStack Query, Vitest/Testing Library.

**Spec:** `docs/superpowers/specs/2026-08-29-yaoguang-marketing-platform-design.md`

## Global Constraints

- Work directly on the already-authorized `main` checkout.
- Do not create a branch, commit, push, or use subagents.
- Preserve legacy Segment behavior; only Audience Customer-ID compilation moves to the Customer authority.
- Every production behavior change starts with a failing focused test.
- Do not create or save audit-only audiences during browser verification.

---

### Task 1: Customer-authoritative Audience condition compiler

**Files:**
- Create: `internal/service/customer_condition_builder.go`
- Modify: `internal/service/query_builder.go`
- Test: `internal/service/query_builder_test.go`

**Interfaces:**
- Consumes: `QueryBuilder.BuildCustomerIDSQL(tree, placeholderOffset)` and `BuildCustomerMatchSQL` callers.
- Produces: Customer-rooted SQL returning `customer.id`, plus identical condition arguments and placeholder-offset guarantees.

- [ ] Add failing tests proving profile status/attributes/tags read canonical tables, list conditions use `customer_list_memberships` with legacy fallback, and a Customer without `contacts` remains eligible.
- [ ] Run the focused Go tests and confirm they fail on the current `FROM contacts` SQL.
- [ ] Implement recursive Customer-tree compilation while leaving `BuildSQL` unchanged.
- [ ] Add `customer_id`-first, Email-fallback predicates for timeline/custom-event compatibility.
- [ ] Run QueryBuilder, Audience repository, snapshot, and per-customer match tests.

### Task 2: Return and edit the active Audience definition

**Files:**
- Modify: `internal/domain/audience.go`
- Modify: `internal/service/audience_service.go`
- Test: `internal/service/audience_service_test.go`
- Modify: `console/src/services/api/marketing.ts`
- Modify: `console/src/components/audiences/AudienceDrawer.tsx`
- Modify: `console/src/pages/AudiencesPage.tsx`
- Test: `console/src/components/audiences/AudienceDrawer.test.tsx`
- Test: `console/src/pages/AudiencesPage.test.tsx`

**Interfaces:**
- Produces: `Audience.definition?: AudienceExpression` on `audiences.get` only.
- Produces: `AudienceDrawer` create mode and `audienceId` edit mode; edit saves through `audienceApi.update` and refreshes versions.

- [ ] Add failing service test that `Get` attaches the active version definition.
- [ ] Add failing UI tests for opening a dynamic row in edit mode and updating rather than creating.
- [ ] Attach the active definition in the service and type it in the client.
- [ ] Load edit data in the drawer, show the current tree, and invalidate list/detail queries after update.
- [ ] Add an accessible Edit action for dynamic rows.
- [ ] Run focused service and console tests.

### Task 3: Make preview and save use one committed rule

**Files:**
- Modify: `console/src/components/segment/input.tsx`
- Modify: `console/src/components/audiences/AudienceDrawer.tsx`
- Test: `console/src/components/audiences/AudienceDrawer.test.tsx`
- Test: `console/src/components/segment/tree_draft.test.tsx`

**Interfaces:**
- Adds: `TreeNodeInputProps.onEditingChange?: (editing: boolean) => void`.
- Enforces: Audience save is disabled while any condition form is open or a draft exists.

- [ ] Add a failing test where the preview sees a changed draft while Save remains unavailable.
- [ ] Add explicit editor-state reporting from `TreeNodeInput`.
- [ ] Gate both the button and save function on committed editor state.
- [ ] Run draft-tree and drawer tests.

### Task 4: Clear hidden condition values

**Files:**
- Modify: `console/src/components/segment/form_leaf.tsx`
- Modify: `console/src/components/segment/tree_completeness.ts`
- Test: `console/src/components/segment/form_leaf_draft.test.tsx`
- Test: `console/src/components/segment/form_leaf_web_kinds.test.tsx`
- Test: `console/src/components/segment/tree_completeness.test.ts`

**Interfaces:**
- Enforces: `not_in` carries no hidden list status; non-email Activity kinds carry no template/broadcast/link scope; cleared day inputs are incomplete rather than the string `null`.

- [ ] Add failing tests for list `in -> not_in`, email `-> web`, and cleared relative-day inputs.
- [ ] Clear invalid fields at the source-change handlers and set conditional items to `preserve={false}`.
- [ ] Normalize cleared day values to an empty array and strengthen completeness checks.
- [ ] Run all condition-form tests.

### Task 5: Localize and restrict condition choices

**Files:**
- Modify: `console/src/components/segment/input.tsx`
- Modify: `console/src/components/segment/input_dimension_filters.tsx`
- Modify: `console/src/components/segment/type_string.tsx`
- Modify: `console/src/components/segment/type_number.tsx`
- Modify: `console/src/components/segment/type_time.tsx`
- Modify: `console/src/components/segment/type_json.tsx`
- Test: focused Segment input tests.

**Interfaces:**
- Enforces: fields with `shown: false` are absent from the picker; schema source labels, field labels, operators and placeholders follow the active locale.

- [ ] Add failing tests for hidden fields and Chinese labels in the picker/operator controls.
- [ ] Filter the available schema fields by `shown !== false`.
- [ ] Route source/field/operator display strings through Lingui without changing stored field or operator values.
- [ ] Run focused localization and segment editor tests.

### Task 6: End-to-end verification

**Files:** No production files.

- [ ] Run focused Go domain/service/repository tests.
- [ ] Run focused Vitest suites for Audience and Segment editors.
- [ ] Run console TypeScript build and record unrelated gates separately if any.
- [ ] Rebuild/reload the local console if required.
- [ ] In authenticated Chrome, verify create, draft edit/save gating, update/version increment, list/activity source switching, count preview, and Chinese labels without saving audit-only data.
- [ ] Run `git diff --check` and inspect `git status --short` to confirm only intended uncommitted files changed.
