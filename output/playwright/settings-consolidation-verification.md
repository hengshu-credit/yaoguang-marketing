# Settings consolidation verification

## Changes

- The standalone Custom Fields sidebar item is removed. Its legacy route opens the unified Customer attributes tab. Named legacy fields and flexible attributes are listed together; unused predefined slots can be shown on demand. Rename/reset preserves data and other labels. Overlapping existing attribute definitions remain accessible.
- Existing events support single/multiple selection for optional report field extensions. Schema changes and revision snapshots are transactional; conflicting fields roll back the batch. Native event meanings and trusted producers are protected; reseeding preserves added fields. A device fingerprint schema example and reporting guidance are available.
- Object types and relations include a contract/repayment example, configuration order, identity guidance and instance-linking requirements. Object type editors start on schema fields. Customer ownership is automatic, and invalid customer-as-object relations cannot be newly configured.
- State configuration follows Tianshu's AND/OR group and nested-tree interaction in React, retaining the marketing engine's StateRule contract. Equality defaults, controlled operands, numeric/boolean values, existing rule switching, related-object scoping and full new-state reset are handled.
- Saved-state inspection accepts a customer UUID/email/external ID or business object UUID, refreshes derived state, and returns current values/revision/time without sending marketing triggers. Unsaved edits disable inspection and stale inspection responses are discarded. Coverage analysis is accessible in the same settings page.

## Evidence

- Frontend: 18 cases across five focused suites passed, followed by the updated three-case attribute consolidation suite (one additional case for overlapping definitions). TypeScript, scoped ESLint and Vite production build passed.
- Go domain/service regressions passed. The full API compiled.
- A stale local backend process was still holding port 8080 and preventing hot reload. Restarted only the local backend container; it returned healthy and both new API routes now return the expected unauthenticated 401 response.
- Real PostgreSQL, isolated databases: TestConfigurationEventFieldsAndStateInspectionIntegration, TestNativeBusinessEventCataloguePersistence, and TestSubjectStateFunnelPostgresIntegration passed. Covered accepted fingerprint values, rejected wrong types, atomic rollback, native seed persistence, saved state true-to-false recalculation and no emitted marketing events during inspection.
- Real browser with isolated API fixtures: label edit preserving another label; adding optional fields to built-in/custom events; schema-first object editing; relation guidance; numeric state save and current-state inspection; unsaved-rule guard; new-state reset; 1024px layout without whole-page horizontal overflow; legacy attribute route and filtered-empty state.
- Tianshu sources were read for reference only; no files there were modified. No Vue runtime was introduced into the React console.

Browser fixture responses and real PostgreSQL/service integration checks are separate; no real customer data was written by the browser acceptance steps. Device fingerprint values must be supplied by the actual reporting application, and historical events are not backfilled.

Screenshots: settings-unified-attributes.png, settings-event-fields.png, settings-object-fields.png, settings-object-relations.png, settings-state-inspection.png, settings-state-1024.png, settings-attributes-empty.png.

User-facing configuration/API examples are in docs/guides/configurable-marketing.md.
