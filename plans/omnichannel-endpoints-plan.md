# Omnichannel endpoints implementation plan

## Task 1: Schema and migration

- Add schema definitions and trigger functions for encrypted contact endpoints.
- Install them for new workspaces and add v42 for existing workspaces.
- Test table constraints, indexes, safe timeline payloads, and migration registration.

## Task 2: Domain contract

- Add endpoint provider/platform/operation constants and validated mutations.
- Extend contact ingest records with endpoint mutations.
- Add repository interface and endpoint metadata model.
- Test normalization, invalid combinations, and secret-free JSON output.

## Task 3: Encrypted repository

- Encrypt addresses with the application secret and persist a SHA-256 fingerprint.
- Implement idempotent upsert/disable and active endpoint selection.
- Test SQL behavior and verify diagnostics never expose addresses.

## Task 4: Ingest and application wiring

- Inject the endpoint repository into the ingest service.
- Apply endpoint mutations after contact/profile mutations.
- Wire the repository in the application and regenerate affected mocks if required.
- Extend HTTP/service tests and external API documentation.

## Task 5: Verification

- Run focused unit, repository, migration, HTTP, and service tests.
- Rebuild the API image, restart Compose, and run a real authenticated endpoint ingest.
- Verify encrypted storage, safe timeline events, outbox publication, and ClickHouse
  projection.
- Commit the v42 endpoint foundation to `dev`.
