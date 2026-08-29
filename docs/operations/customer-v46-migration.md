# V46 Customer authority migration runbook

V46 introduces the Workspace-local `customers` authority, encrypted identities, mutable profiles, permanent Workspace sequence numbers, and the `customers` permission. It preserves `contacts` and `contact_endpoints` as compatibility projections. This migration is forward-only.

## Before the upgrade

1. Stop application writes and wait for active requests and workers to finish. Keep the maintenance window active until every Workspace is upgraded and verified.
2. Record the current application version and the complete Workspace list from the system database.
3. Take a transactionally consistent backup of the system database and **every** Workspace database in the same maintenance window. Retain the matching encryption `SECRET_KEY` separately; encrypted identities cannot be recovered without it.
4. Verify the backups can be listed/read and record their checksums and restore commands.
5. Run the following preflight in every Workspace database. Each query must return no rows:

```sql
SELECT BTRIM(external_id) AS normalized_external_id, COUNT(*)
FROM contacts
WHERE NULLIF(BTRIM(external_id), '') IS NOT NULL
GROUP BY BTRIM(external_id)
HAVING COUNT(*) > 1;

SELECT LOWER(BTRIM(email)) AS normalized_email, COUNT(*)
FROM contacts
GROUP BY LOWER(BTRIM(email))
HAVING COUNT(*) > 1;
```

Resolve collisions in the source business system and the legacy Contact data before retrying. V46 deliberately aborts instead of guessing which records to merge.

## What V46 does

The system phase creates `workspace_sequence_number_seq` with values `1..9999`, `NO CYCLE`, assigns a permanent sequence to existing Workspaces, and adds the unique/range constraints. Sequence values are never reused after a Workspace is deleted. It also copies each existing `contacts` permission into `customers` for memberships and pending invitations; existing approval behavior is otherwise unchanged.

For each Workspace, V46:

- creates `customers`, `customer_profiles`, `customer_identities`, `customer_tags`, `customer_consents`, `customer_list_memberships`, `customer_merge_log`, and `customer_idempotency`;
- adds nullable `customer_id` compatibility links to `contacts` and `contact_endpoints`;
- creates one UUID Customer and immutable 53-character `customer_no` for every legacy Contact;
- copies external IDs, profile fields/attributes, tags, and active list memberships;
- normalizes, encrypts, masks, and fingerprints email identities and valid E.164 phone identities; and
- leaves invalid legacy phone text only in the Contact/Profile compatibility data instead of making it searchable as an identity.

The migration creates tables and indexes and scans the Contact/profile/tag/list tables. The backfill updates all legacy Contact rows. Plan the maintenance window for the largest Workspace, ensure adequate free disk/WAL capacity, and avoid concurrent Contact writes while it runs. Migrations run system-first and then Workspace-by-Workspace; do not send traffic until the complete fleet has reached V46.

## Post-upgrade verification

In the system database:

```sql
SELECT COUNT(*) AS invalid_workspace_sequences
FROM workspaces
WHERE workspace_sequence NOT BETWEEN 1 AND 9999;

SELECT workspace_sequence, COUNT(*)
FROM workspaces
GROUP BY workspace_sequence
HAVING COUNT(*) > 1;

SELECT COUNT(*) AS memberships_missing_customers_permission
FROM user_workspaces
WHERE permissions IS NOT NULL AND NOT permissions ? 'customers';

SELECT COUNT(*) AS invitations_missing_customers_permission
FROM workspace_invitations
WHERE permissions IS NOT NULL AND NOT permissions ? 'customers';
```

All four results must be zero rows or a zero count. In every Workspace database:

```sql
SELECT
  (SELECT COUNT(*) FROM contacts) AS contacts,
  (SELECT COUNT(*) FROM customers) AS customers,
  (SELECT COUNT(*) FROM contacts WHERE customer_id IS NULL) AS contacts_without_customer,
  (SELECT COUNT(*) FROM customer_profiles) AS customer_profiles;

SELECT customer_no
FROM customers
WHERE customer_no !~ '^U[0-9]{4}[0-9]{14}08[0-9a-f]{32}$'
LIMIT 20;

SELECT external_user_id, COUNT(*)
FROM customers
WHERE external_user_id IS NOT NULL
GROUP BY external_user_id
HAVING COUNT(*) > 1;

SELECT identity_type, lookup_fingerprint, COUNT(*)
FROM customer_identities
GROUP BY identity_type, lookup_fingerprint
HAVING COUNT(*) > 1;

SELECT COUNT(*) AS invalid_ciphertexts
FROM customer_identities
WHERE value_ciphertext !~ '^[0-9a-f]+$'
   OR LENGTH(value_ciphertext) < 56
   OR LENGTH(value_ciphertext) % 2 <> 0;
```

`contacts` and `customers` must match for the initial backfill, `contacts_without_customer` must be zero, malformed/duplicate queries must return no rows, and `invalid_ciphertexts` must be zero. Do not select or log decrypted identity values during verification.

After SQL verification, smoke-test `POST /api/customers.upsert`, `.get`, `.batch`, and `.merge` with a non-production identity. Confirm that reads return only `display_hint`, idempotent retries set `replayed: true`, and merged source lookup resolves to the target.

The synchronous batch default is 10,000 items. Operators can set `CUSTOMER_SYNC_MAX_BATCH_SIZE` to another positive value, but must also account for the 32 MiB HTTP body limit, transaction throughput, and client timeouts. This endpoint always returns one ordered result per accepted request item; durable asynchronous file imports above this envelope are delivered separately.

## Recovery and rollback

There is no down migration. If any Workspace fails or verification finds material inconsistency:

1. keep all application traffic stopped;
2. preserve database/application logs and identify the first failed Workspace;
3. discard the partially upgraded databases; and
4. restore the system database and **all** Workspace databases from the coordinated pre-V46 backup, together with the matching `SECRET_KEY`.

Never restore only the system database or one Workspace and resume traffic. The permanent Workspace sequence, permissions, and Workspace-local Customer links form one release boundary. After the coordinated restore, deploy the previous application version, rerun the preflight checks, fix the cause, and schedule a fresh upgrade.
