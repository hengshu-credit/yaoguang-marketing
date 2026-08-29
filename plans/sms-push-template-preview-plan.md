# SMS and Push Template Preview Implementation Plan

1. Add domain models, validation, localized resolution, preview requests, and preview response types.
2. Add v43 workspace migration and fresh-workspace schema columns.
3. Extend template repository create/read/list/update SQL and scanners.
4. Add authenticated bounded Liquid preview service with SMS segmentation and push platform warnings.
5. Expose `/api/templates.preview` and update console API types/client.
6. Run focused tests, repository/package tests, vet, all-package compilation, Compose migration, and real preview API smoke tests.
