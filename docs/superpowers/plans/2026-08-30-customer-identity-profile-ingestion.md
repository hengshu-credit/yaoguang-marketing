# Customer Identity and Profile Ingestion Implementation Plan

> **Execution rule:** implement this plan in the current checkout with strict red-green-refactor cycles. The approved platform specification is authoritative where the program plan is less specific.

**Goal:** Introduce the UUID-based Customer identity layer, immutable Yaoguang customer numbers, workspace-scoped external identities, idempotent Profile APIs, and explicit anonymous-to-known merge while keeping the existing email-keyed Contact runtime operational through a transactional compatibility projection.

**Architecture:** The system database permanently allocates a four-digit sequence to each Workspace. Every Workspace database owns its Customer tables, so external IDs and identity fingerprints are unique only inside that database. New HTTP routes call a Customer Service which authorizes once, validates and normalizes input, and delegates each accepted item to one repository transaction. That transaction converges the Customer aggregate, idempotency record, email Contact projection, tags, list memberships, and merge audit. Legacy Contact APIs remain available and no new execution path uses email as the Customer authority.

**Tech stack:** Go 1.25.4, PostgreSQL, `database/sql`, pgx, AES-GCM/HMAC-SHA256 through the existing crypto package, `net/http`, OpenAPI YAML, React/TypeScript permission matrix, sqlmock, testify, gomock, Vitest.

**Authoritative spec:** `docs/superpowers/specs/2026-08-29-yaoguang-marketing-platform-design.md`, especially sections 5-9.

## Confirmed scope and non-goals

- Customer IDs are UUIDs and are exposed together with `customer_no` and optional `external_user_id`.
- `customer_no` is exactly `U{workspace_seq:04d}{yyyyMMddHHmmss}{08}{uuid32}` using Asia/Shanghai time and the full lowercase UUID without hyphens.
- A Workspace sequence is in `0001..9999`, is never reused after deletion, and exhaustion produces a typed capacity error.
- Email, phone, anonymous ID, device ID, WhatsApp, Telegram, and custom identities are normalized and unique only inside one Workspace database. Sensitive values are encrypted and searched by an HMAC-SHA256 lookup fingerprint.
- Profile updates distinguish whole-object `set`, recursive object `merge`, and dot-path `unset`; JSON null is data and never doubles as omission/deletion.
- Upsert never implicitly merges two Customers. Batch 2 supports only explicit anonymous-source to known-target merge. This corrects the contradictory “automatic” phrase in the program plan to match the approved spec.
- The synchronous batch route accepts at most 10,000 items and returns one durable result for every input item. Durable manifests, asynchronous 100,000-item batches, file imports, resumable reconciliation, and background limit administration remain Batch 4.
- Existing approval behavior is unchanged. No approval workflow is added.
- Existing email-keyed Contacts, lists, segments, automations, and sending paths remain compatibility projections in this batch.

## Public API contract

### `POST /api/customers.upsert`

Request is `domain.UpsertCustomerRequest`:

```go
type UpsertCustomerRequest struct {
    WorkspaceID   string              `json:"workspace_id"`
    IdempotencyKey string             `json:"idempotency_key"`
    Customer      CustomerUpsertInput `json:"customer"`
}
```

`CustomerUpsertInput` can identify an existing Customer with one locator or create one from `external_user_id` or at least one identity. It contains optional `profile`, identity mutations, a tag replacement, and list memberships. A successful response is `CustomerMutationResult` and always includes `customer_id`, `customer_no`, and `external_user_id` when present. Reusing an idempotency key with the same normalized payload returns the stored result; reusing it with a different payload returns `409 idempotency_conflict`.

### `POST /api/customers.batchUpsert`

Request contains `workspace_id` and `customers`, where every item carries its own `idempotency_key` and `customer`. The service authenticates once, rejects an empty batch or more than 10,000 items, processes every valid item, and returns ordered per-index results plus `success_count` and `failure_count`. Item validation/conflict errors do not erase successful sibling transactions.

### `GET /api/customers.get`

The query contains `workspace_id` plus exactly one of `customer_id`, `customer_no`, `external_user_id`, or `identity_type` + `identity_value`. A merged source UUID redirects to and returns the target Customer while exposing `resolved_from_customer_id`.

### `POST /api/customers.merge`

The request carries `workspace_id`, `idempotency_key`, source locator, target locator, and optional reason. Source must have no external user ID, target must have one, and the two resolved UUIDs must differ. Identity conflicts with a third Customer return `409`; duplicate source/target identities converge onto the target. The response is idempotent.

## Task 1: Pin the customer number and identity contracts

**Files:**

- Create: `internal/domain/customer.go`
- Create: `internal/domain/customer_test.go`
- Modify: `plans/yaoguang-marketing-platform-program-plan.md`

1. Add failing table-driven tests that prove the exact 53-character customer number, Asia/Shanghai conversion, fixed `08`, lower-case 32-character UUID suffix, and workspace sequence boundaries 1 and 9999.
2. Add failing tests for locator exclusivity, external ID length/whitespace normalization, supported identity types, email normalization, phone normalization to a strict E.164 representation, case-preserving provider IDs, tag/list validation, and the set/merge/unset Profile patch semantics.
3. Implement `GenerateCustomerNumber(workspaceSequence uint16, at time.Time, customerID uuid.UUID) (string, error)`, Customer aggregate/request/result structs, typed not-found/conflict/idempotency/merge errors, validation, canonical hashing, identity normalization, masked display hints, and recursive JSON profile patching.
4. Correct Batch 2 in the program plan to say only explicit anonymous-to-known merge is allowed.
5. Run `go test ./internal/domain -run 'Test(Customer|GenerateCustomerNumber|ApplyCustomerProfilePatch)'` in the Go 1.25.4 container and commit the domain contract.

## Task 2: Permanently allocate Workspace sequences

**Files:**

- Modify: `internal/database/schema/system_tables.go`
- Modify: `internal/database/schema/system_tables_test.go`
- Modify: `internal/domain/workspace.go`
- Modify: `internal/domain/workspace_test.go`
- Modify: `internal/repository/workspace_postgres.go`
- Modify: `internal/repository/workspace_postgres_test.go`
- Modify: `internal/migrations/manager.go`
- Modify: `internal/migrations/manager_test.go`

1. Add failing schema tests for `workspace_sequence_number_seq` with `MAXVALUE 9999 NO CYCLE`, a non-null `workspaces.workspace_sequence`, a unique index, and an in-range check.
2. Add failing repository tests proving creation obtains the sequence in the same system-row insert using `RETURNING workspace_sequence`, scans it on get/list, maps SQLSTATE `2200H` to `*domain.ErrWorkspaceSequenceCapacity`, and does not reset the sequence on Workspace deletion.
3. Add `Sequence uint16` to `domain.Workspace`, its scan shape, and manager Workspace loading. Change migration Workspace discovery to read through the active system transaction so V46 system changes are visible before its Workspace phase.
4. Keep the database-first Workspace creation cleanup behavior, but persist and return the allocated sequence on the subsequent system insert.
5. Run affected domain/repository/migration tests and commit the allocation layer.

## Task 3: Add V46 Customer schema and backfill

**Files:**

- Create: `internal/database/schema/customer_tables.go`
- Create: `internal/database/schema/customer_tables_test.go`
- Create: `internal/migrations/v46.go`
- Create: `internal/migrations/v46_test.go`
- Modify: `internal/database/init.go`
- Modify: `internal/database/init_test.go`
- Modify: `config/config.go`
- Modify: `config/config_test.go`

1. Add failing schema tests for `customers`, `customer_profiles`, `customer_identities`, `customer_tags`, `customer_consents`, `customer_list_memberships`, `customer_merge_log`, and `customer_idempotency`, including UUID keys, Customer foreign keys, identity uniqueness, idempotency operation/key uniqueness, merge redirect, optimistic versions, and lookup indexes.
2. Add `customer_id UUID` compatibility columns and indexes to `contacts` and `contact_endpoints`; do not remove or rename existing email keys.
3. Implement V46 system migration to add/backfill the permanent Workspace sequence and copy every existing `contacts` permission into a new `customers` permission for memberships and invitations.
4. Implement V46 Workspace migration to create the Customer schema, preflight duplicate normalized email identities and duplicate non-empty external IDs (abort with actionable diagnostics instead of auto-merging), generate UUID/customer numbers for all legacy contacts using that Workspace sequence, copy external IDs, language/timezone, Profile attributes, tags, list memberships, and primary email/phone identities, then set `contacts.customer_id`. Encrypt backfilled identity values with the configured secret and use the same normalization/fingerprint functions as runtime writes. Preserve a legacy phone only in the Contact projection when it cannot normalize to E.164; do not invent a searchable phone identity.
5. Include the complete V46 schema in fresh system/Workspace initialization because first-run databases are stamped directly at the current code version and skip migrations.
6. Bump `config.VERSION` to `46.0`; add migration metadata/registration and rollback/recovery notes to the operator documentation. The migration is forward-only; recovery is restoring both the system and all Workspace databases from the same pre-upgrade backup.
7. Run schema, database, migration, and configuration tests and commit the migration.

## Task 4: Implement transactional Customer repository

**Files:**

- Create: `internal/repository/customer_postgres.go`
- Create: `internal/repository/customer_postgres_test.go`
- Modify: `internal/domain/customer.go`
- Generate: `internal/domain/mocks/mock_customer_repository.go`

1. Add failing repository tests for lookup by UUID, customer number, external ID, and normalized identity; merged-source redirect; cross-Customer external/identity conflicts; and not-found mapping.
2. Add failing repository tests for first upsert, same-key replay, different-payload idempotency conflict, update with optimistic version increment, profile set/merge/unset, encrypted identity storage, tag/list replacement, and Contact projection creation only when an email identity exists.
3. Implement `CustomerRepository.Upsert`, `Get`, and aggregate scanning over the Workspace connection. Each mutation uses `WorkspaceRepository.WithWorkspaceTransaction`; it claims the idempotency key first, locks/resolves the aggregate, applies every child-table and Contact projection change, stores the response, and commits once.
4. Map PostgreSQL unique violations by constraint name to typed external-ID, identity, customer-number, or generic conflict errors. Never match localized database message text.
5. Generate the repository mock with the pinned Go/mockgen toolchain, format, run repository tests, and commit the repository slice.

## Task 5: Implement Customer Service and synchronous micro-batch

**Files:**

- Create: `internal/service/customer_service.go`
- Create: `internal/service/customer_service_test.go`
- Modify: `internal/domain/customer.go`
- Generate: `internal/domain/mocks/mock_customer_service.go`

1. Add failing tests that `customers:read` gates get and `customers:write` gates upsert/batch/merge, including owner access, missing permission, and authentication failure.
2. Add failing tests proving workspace-scoped sequence lookup, canonical payload hashing, a 10,000-item accepted boundary, 10,001-item rejection, ordered complete results, and sibling success when another item is invalid or conflicts.
3. Implement `CustomerService` with a dependency struct (`Repository`, `WorkspaceRepository`, `AuthService`, `MaxSyncBatchSize`). Authenticate once per call, normalize before hashing, and never log raw identity values.
4. Generate the service mock, run focused service tests, and commit the service slice.

## Task 6: Implement explicit anonymous-to-known merge

**Files:**

- Modify: `internal/repository/customer_postgres.go`
- Modify: `internal/repository/customer_postgres_test.go`
- Modify: `internal/service/customer_service.go`
- Modify: `internal/service/customer_service_test.go`

1. Add failing tests that reject known source, anonymous target, same resolved Customer, already-merged-to-another-target source, and identity ownership by a third Customer.
2. Add failing tests proving idempotent replay and atomic movement/convergence of Profile attributes, identities, tags, consents, list memberships, and Customer-linked endpoints; the source row receives `merged_into_id`, and a merge-log snapshot is written.
3. Implement deterministic lock ordering, target-wins Profile conflict behavior (`source attributes` then `target attributes`), source redirect, and immutable merge audit. Do not rewrite immutable legacy event/timeline history.
4. Run focused repository/service merge tests and commit the merge slice.

## Task 7: Expose HTTP routes and error contracts

**Files:**

- Create: `internal/http/customer_handler.go`
- Create: `internal/http/customer_handler_test.go`
- Modify: `internal/http/utils.go`
- Modify: `internal/app/app.go`
- Modify: `internal/app/app_test.go`

1. Add failing HTTP tests for method guards, malformed JSON/query validation, authentication/permission status mapping, `404 customer_not_found`, `409 identity_conflict`, `409 idempotency_conflict`, successful upsert/get/merge, and complete ordered batch results.
2. Implement authenticated RPC-style routes with bounded JSON decoding, a response `request_id`, no sensitive identity echo, and typed service-error mapping.
3. Wire repository/service/handler in `App.InitRepositories`, `InitServices`, and `InitHandlers`; expose a Customer repository getter for integration tests.
4. Run HTTP and app tests and commit the transport slice.

## Task 8: Publish OpenAPI and permission-management support

**Files:**

- Create: `openapi/components/schemas/customer.yaml`
- Create: `openapi/paths/customers.yaml`
- Modify: `openapi/openapi.yaml`
- Modify: `internal/domain/workspace.go`
- Modify: `internal/domain/workspace_test.go`
- Modify: `console/src/services/api/permissions.ts`
- Modify: `console/src/components/settings/PermissionsMatrix.tsx`
- Modify: `console/src/components/settings/__tests__/PermissionsMatrix.test.tsx`
- Modify: `console/src/components/settings/WorkspaceMembers.test.tsx`
- Regenerate: `openapi.json`
- Regenerate: `console/src/i18n/locales/*.po`

1. Add failing backend/frontend tests that `customers` is a canonical resource, full permissions include it, partial legacy permission maps deny it unless V46 backfills it, and the matrix exposes accurate Customer route descriptions.
2. Document all request/response/error schemas, identity masking, per-item batch results, and locator exclusivity in OpenAPI.
3. Run Redocly bundle/lint, Lingui extraction/compile, permission Vitest tests, TypeScript build, and commit the public contract.

## Task 9: Integration and upgrade verification

**Files:**

- Create: `tests/integration/customer_profile_test.go`
- Create: `docs/operations/customer-v46-migration.md`

1. Add integration coverage using two Workspace databases to prove the same external ID and identity can exist in both but conflicts within one.
2. Verify legacy Contact backfill, new email Contact projection, external-ID-only Customer creation, all four lookup forms, exact customer number, idempotent retry, payload conflict, and explicit merge redirect.
3. Document pre-upgrade backup, forward-only recovery, expected lock/scan behavior, permission backfill, post-migration SQL checks, and rollback by coordinated database restore.
4. Run targeted tests, all affected Go packages, integration test, frontend tests/build, OpenAPI lint, `git diff --check`, and a clean-worktree audit. Record the unrelated baseline SNS network failure separately if it recurs.
5. Commit the completed Batch 2 and update the program plan with its detailed-plan link and completion status only after every gate passes.

## Verification commands

Use Docker for Go because the host has no Go toolchain:

```powershell
docker run --rm -v "${PWD}:/src" -w /src -v yaoguang-go-mod:/go/pkg/mod -v yaoguang-go-cache:/root/.cache/go-build golang:1.25.4 go test ./internal/domain ./internal/database/schema ./internal/database ./internal/migrations ./internal/repository ./internal/service ./internal/http ./internal/app
```

Frontend and contract checks:

```powershell
pnpm --dir console test -- --run PermissionsMatrix WorkspaceMembers
pnpm --dir console build
npx @redocly/cli bundle openapi/openapi.yaml -o openapi.json --ext json
npx @redocly/cli lint openapi/openapi.yaml
git diff --check
git status --short --branch
```
