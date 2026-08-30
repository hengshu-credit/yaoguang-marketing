# Customer V47 authority cutover

V47 keeps every Workspace isolated in its own database. Customer remains the write authority, while `contacts` and the Email columns on legacy marketing tables remain compatibility projections and read fallbacks.

## Cutover order

1. Deploy application version 47.0 and wait for every Workspace migration to finish.
2. Call `POST /api/customers.reconciliation.scan` with exactly one `workspace_id`.
3. Inspect the run with `GET /api/customers.reconciliation.get?workspace_id=...&run_id=...`.
4. If only repairable missing UUID references remain, call `POST /api/customers.reconciliation.repair` for that Workspace.
5. Run scan again. Enable Customer-authority traffic only when repairable missing counts are zero and every conflict has been investigated.
6. Observe reconciliation, Customer write failures, Contact compatibility reads, event ingestion and delivery for at least 24 hours.

All three endpoints require Customers write permission. Runs use a Workspace-local advisory lock, process rows in batches of 2,000 and persist a stable-key checkpoint. A restarted repair resumes the running job. Repair only sets a missing `customer_id` when the legacy Email resolves to one Contact Customer; it never replaces a non-null conflicting UUID.

## Interpreting findings

- `contacts_without_customer`: legacy Contacts that still need an explicit Customer migration; automatic repair does not create Customers.
- `customers_without_contact`: Customers with a primary Email identity but no Contact projection; investigate identity/projection creation rather than copying masked values.
- `identity_contact_mismatch`: a Contact points to a Customer without an enabled primary Email identity.
- Legacy table findings expose missing, conflict, repairable, repaired and unrepairable counts separately.

Do not declare the cutover healthy by looking only at the total missing count. Non-repairable and conflict findings require investigation even when a repair run completed successfully.

## Rollback

Do not roll back or delete Customer rows written after cutover. Stop new producers, keep V47 schema and data, and temporarily route reads through the existing Contact/Email fallback while the cause is repaired. Re-run scan and repair per Workspace before restoring Customer-authority traffic. The fallback is a compatibility path, not permission to overwrite conflicting UUID associations.
