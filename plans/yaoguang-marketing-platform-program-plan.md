# Yaoguang Marketing Platform Program Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement each delivery batch.

**Goal:** Evolve the existing Notifuse-based application into the production-ready 瑶光营销平台 while preserving workspace isolation, existing delivery behavior, AGPL obligations, and upgrade compatibility.

**Architecture:** Keep the current Go modular monolith and React console, introduce multi-role execution over PostgreSQL, RabbitMQ, Redis, ClickHouse, and object storage, and deliver the approved design in independently deployable batches. Persistent identifiers keep backward-compatible names unless a migration explicitly changes them; new public product and configuration identifiers use Yaoguang naming.

**Tech Stack:** Go, PostgreSQL, RabbitMQ, Redis, ClickHouse, S3-compatible object storage, React, TypeScript, Ant Design, TanStack Query/Router, Lingui, Vitest, Playwright.

**Spec:** `docs/superpowers/specs/2026-08-29-yaoguang-marketing-platform-design.md`

## Global Constraints

- Preserve every workspace as an independent tenant boundary; `external_user_id` is unique only within one workspace.
- Keep internal UUID primary keys while exposing the approved `customer_no` format.
- Preserve existing approval behavior; do not add a new approval workflow in this program.
- Preserve upstream copyright and AGPL notices.
- Use `YAOGUANG_*` for new public configuration and keep documented `NOTIFUSE_*` fallback for one compatibility cycle.
- No acknowledged customer, event, import, or delivery-command data may be silently lost.
- Add failing tests before behavioral changes and run fresh batch-specific and repository-level verification before completion claims.
- Do not mix unrelated pre-existing working-tree changes into Yaoguang commits.

## Delivery Batches

### Batch 1: Brand Shell and Engineering Identity

- Add a reusable logo/name/tagline lockup to authenticated, sign-in, and setup surfaces.
- Update browser metadata and product-facing defaults.
- Rename the Go module and first-party import paths to `github.com/hengshu-credit/yaoguang-marketing`.
- Add `YAOGUANG_ROLE` with `NOTIFUSE_ROLE` fallback and update runtime packaging examples.
- Add the `zh-CN` locale entry point without weakening catalogue completeness checks.
- Detailed plan: `plans/yaoguang-brand-shell-plan.md`.

### Batch 2: Customer Identity and Profile Ingestion

- Add workspace sequence allocation, `customer_no`, `external_user_id`, aliases, anonymous identities, and merge history.
- Implement single and bulk Customer Profile APIs with idempotency and workspace-scoped lookup.
- Add merge rules: anonymous-to-known is automatic; known-to-known requires an explicit operation.
- Provide migration, repository, service, HTTP, and integration tests.

### Batch 3: Event Ledger and Realtime Triggering

- Persist immutable events with internal `event_uuid` and workspace-scoped external `event_id` idempotency.
- Add profile upsert coupling, transactional outbox publication, schema validation, and replay controls.
- Route accepted events to dynamic-segment maintenance and Journey entry evaluation.
- Verify sustained/peak ingestion with a reproducible benchmark harness.

### Batch 4: Reliable Bulk Intake and Import Operations

- Add configurable sync/async/file limits using the approved defaults.
- Persist manifests and raw input before acknowledgement, process `(job_id,row_number)` idempotently, and reconcile every row.
- Add retry, quarantine, `needs_attention`, resumability, expiry, and operational UI.
- Prove `total = success + duplicate + invalid + failed` and prohibit terminal success while unresolved rows remain.

### Batch 5: Dynamic Audiences and Segments

- Add versioned filter DSL, deterministic preview, materialized membership, incremental event/profile updates, and full rebuild.
- Record definition version and membership snapshot used by each activation.
- Add query-cost guards and workspace-scoped scheduling.

### Batch 6: Campaigns, Journeys, and Scheduling

- Separate broadcast Campaign execution from event/time-triggered Journey execution.
- Add Journey versions, nodes, edges, wait states, branching, re-entry policy, stop conditions, and execution state.
- Add one-time, recurring, timezone-aware scheduling and deterministic audience snapshotting.

### Batch 7: Omnichannel Delivery Adapters

- Retain native Email, Twilio SMS, and FCM Push adapters.
- Add signed generic Webhook adapters for WhatsApp, Telegram, In-App, and domestic providers.
- Normalize delivery command, provider response, retry class, receipt, and channel-address handling.
- Keep externally visible sends idempotent.

### Batch 8: Four-Layer Frequency Control

- Add activity-level, event/time-trigger, broadcast, and global-marketing policies.
- Add channel and customer overrides, quiet hours, defer/skip outcomes, Redis reservations, and PostgreSQL audit.
- Provide policy simulation and explainable enforcement results.

### Batch 9: Experimentation and Marketing Analytics

- Add deterministic A/B assignment, control groups, attribution windows, conversions, and guardrails.
- Store high-volume facts in ClickHouse and expose operational and outcome dashboards.
- Keep authoritative configuration/state in PostgreSQL.

### Batch 10: Capacity, Recovery, and Release Readiness

- Validate 10M customers/workspace, 2k sustained and 5k peak events/s, P95 trigger-to-command under 3s, and 10M broadcast execution.
- Document backup/restore, replay, dead-letter recovery, secret rotation, observability, and upgrade paths.
- Complete security, localization, accessibility, license, and deployment reviews.

## Batch Completion Gate

Each batch is complete only when its detailed plan is committed, migrations are reversible or explicitly forward-only with recovery instructions, tests have demonstrated red-green behavior, targeted tests pass, repository-level affected tests pass, operational documentation is updated, and the resulting Git diff contains no unrelated user changes.
