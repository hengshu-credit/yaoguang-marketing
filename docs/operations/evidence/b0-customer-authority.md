# B0 Customer authority acceptance evidence

Acceptance date: 2026-08-30 (Asia/Shanghai)

Branch: `main`

Application version: `47.0`

## Contract evidence

- Customer number format: `U00012026083010300008aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa` (`U` + four-digit Workspace sequence + `yyyyMMddHHmmss` + `08` + 32-character UUID without hyphens).
- The Customer profile integration suite passed the same `external_user_id` in two Workspaces and rejected a duplicate owner inside one Workspace.
- The concurrent identity-owner case passed with one accepted owner and typed conflicts for competing writes.
- `TestCustomerServiceBatchPreservesAllTenThousandOrderedResults` passed with 10,000 accepted results, no losses, no duplicates and the same index order as the input.
- `TestCustomerProfileAPIIntegration/clean_workspace_reconciliation_has_zero_gaps` persisted a completed reconciliation run with zero missing references and zero conflicts.
- Legacy Contact batch import passed against PostgreSQL after routing through Customer authority, including all standard/custom field projections, list membership and last-write-wins duplicate Email handling.

## Quality gates

| Gate | Result | Evidence |
| --- | --- | --- |
| Backend unit and repository suite | Passed, exit 0 | `go test ./internal/domain ./internal/database/schema ./internal/migrations ./internal/repository ./internal/service ./internal/http ./internal/app -count=1` |
| Customer profile core integration | Passed, exit 0 | `go test -tags integration -timeout 20m ./tests/integration -run "^TestCustomerProfileAPIIntegration$" -count=1` |
| Full Customer/Contact/Ingest integration gate | Passed, exit 0 | `go test -tags integration -timeout 20m ./tests/integration -run "Customer\|Contact\|Ingest" -count=1`; 206.556s with PostgreSQL 17 and Mailpit |
| Contact compatibility integration groups | Passed, exit 0 | API/data/database; Customer/demo/preferences/segments; Web Analytics identity/ingest; bulk import and list workflow groups all passed against PostgreSQL 17 |
| 10,000-row conservation | Passed, exit 0 | `go test ./internal/service -run TestCustomerServiceBatchPreservesAllTenThousandOrderedResults -count=1` |
| Frontend tests | Passed, exit 0 | 116 files, 1,668 tests: `npm test -- --run --reporter=dot` |
| Frontend lint | Passed, exit 0 | `npm run lint` with zero warnings |
| Frontend production build | Passed, exit 0 | `npm run build`; only the existing Vite chunk-size advisory remains |
| 375px responsive check | Passed | `innerWidth=375`, `document.scrollWidth=375`, no horizontal overflow |

The integration test role and Mailpit service are test-only infrastructure. No production credentials or production data were used.

## Visual evidence

The Customer list was validated in the local in-app browser at a 375 x 812 viewport. The navigation is collapsed, customer numbers wrap inside cards and the page has no horizontal scrollbar.

Screenshot: [b0-customer-authority-375.png](./b0-customer-authority-375.png)
