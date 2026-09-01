# Workspace Template Category Management Design

## Goal

Replace the console's hard-coded material-template category lists with Workspace-owned categories that administrators can maintain, without changing the compliance meaning of existing templates or breaking stored category IDs.

## Scope and compatibility

- Categories are scoped to one Workspace database.
- Existing template rows keep their current `category` string. No template rewrite is required.
- Migration v55 creates category records for every built-in ID and for any distinct legacy category already present in `templates`.
- Built-in IDs remain stable for integrations, demo data, broadcasts and API clients.
- Existing clients may continue sending `category: "marketing"`; the template API remains backward compatible.
- Category administration uses existing Template read/write permissions.

## Data model

`template_categories` contains:

- `id VARCHAR(20)` — stable lower-case identifier (`[a-z0-9]+(?:[_-][a-z0-9]+)*`).
- `name VARCHAR(64)` — administrator-facing display name.
- `purpose VARCHAR(20)` — immutable `marketing` or `transactional` behavior.
- `sort_order INTEGER` — stable UI ordering from 0 through 10,000.
- `is_system BOOLEAN` — built-in records cannot be deleted or have identity/purpose changed.
- `is_active BOOLEAN` — inactive records remain resolvable for existing templates but cannot be selected for new assignments.
- timestamps.

The API also returns `usage_count`, calculated from non-deleted template rows. A custom category can be deleted only when usage is zero. System categories cannot be deleted.

Built-in purpose mapping:

- marketing: `marketing`, `blog`
- transactional: `transactional`, `welcome`, `opt_in`, `unsubscribe`, `bounce`, `blocklist`, `other`

Distinct legacy category IDs not in the built-in set are seeded as active custom categories with transactional purpose. This conservative default prevents an existing arbitrary category from unexpectedly bypassing unsubscribe rules.

## Template contract

Templates continue to persist `category` as the category ID. TemplateService stamps the immutable purpose into the server-owned `settings.category_purpose` metadata, and repository reads expose it as `category_purpose`; if a legacy row lacks that metadata, the built-in mapping is used and unknown IDs conservatively fall back to transactional.

Create validates that the category exists and is active. Update accepts an inactive category only when it is already assigned to that template; changing to an inactive or missing category is rejected. Domain validation remains limited to category ID shape/length because database membership requires Workspace context.

All runtime decisions that currently compare `category === "marketing"` or treat `blog` as marketing switch to `category_purpose`. A defensive fallback preserves the old mapping when reading an older API response or a test fixture without the new field.

## API and permissions

Authenticated routes:

- `GET /api/templateCategories.list?workspace_id=...&include_inactive=true`
- `POST /api/templateCategories.create`
- `POST /api/templateCategories.update`
- `POST /api/templateCategories.delete`

List requires Template read. Mutations require Template write. Duplicate IDs return validation/conflict errors. Deleting a system or used category returns conflict. Updates can change only name, order and active state; purpose is chosen once at creation.

## Console experience

Template Management loads the category catalogue and builds its filter from it. A `Manage categories` button opens a focused drawer containing:

- ordered system and custom categories;
- usage count and purpose badges;
- create/edit forms;
- activate/deactivate action;
- delete action available only for unused custom categories.

Email, SMS/Push and omnichannel drawers use one shared `TemplateCategorySelect`. Active categories are selectable; the current inactive category remains visible while editing an existing template. Built-in names use existing localized labels until customized categories are involved; custom names are shown verbatim.

## Error and deletion behavior

- Missing/inactive assignment is rejected before template persistence.
- Deleting an in-use category is never cascaded and never rewrites templates.
- Deactivating a category removes it from new-template choices and filters by default, but existing templates keep rendering their category.
- Concurrent delete/use races are closed with a repository transaction and a second usage check before delete.

## Verification

- Domain validation tests for IDs, purpose, order and mutation rules.
- Migration tests for v55 table/default/legacy seeding and version registration.
- Repository tests for CRUD, ordering, usage count and delete conflict.
- Service/handler permission and error-mapping tests.
- Template create/update category assignment tests.
- Console API, manager, dynamic filter and all three editor selector tests.
- Production build, lint, focused backend suites, `git diff --check`, and authenticated browser CRUD/select/filter acceptance when a signed-in tab is available.
