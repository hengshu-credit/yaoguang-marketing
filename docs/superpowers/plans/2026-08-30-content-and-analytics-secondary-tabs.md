# Content and Analytics Secondary Tabs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore every retained Notifuse content and analytics page as a permission-aware, route-backed secondary tab while preserving Yaoguang's nine flat first-level navigation entries.

**Architecture:** Add one presentational page-tab primitive and two domain-specific tab sets, then mount them in the existing leaf pages without restructuring TanStack Router. Update only the two WorkspaceLayout landing rules whose available children now span multiple permission resources.

**Tech Stack:** React 18, TypeScript, Ant Design 6 Tabs, TanStack Router, Lingui, Vitest, Testing Library.

**Spec:** `docs/superpowers/specs/2026-08-30-content-and-analytics-secondary-tabs-design.md`

## Global Constraints

- Keep exactly nine flat first-level sidebar entries and do not render an Ant Design SubMenu.
- Preserve every existing content and analytics URL.
- Content Center tabs are Template Management, Blog Content and File Manager.
- Data Analytics tabs are Marketing Overview, Website Overview, Live Visitors, Multidimensional Analysis, Conversion Goals, Attribution Rules and Analytics Annotations.
- Filter tabs by the current Workspace permissions; do not add permission resources or backend authorization.
- Keep changes on the existing `main` branch as explicitly requested.
- Do not stage or overwrite unrelated dirty Delivery Center, frequency, migration or localization work.

---

### Task 1: Route-backed section tab components

**Files:**
- Create: `console/src/components/navigation/WorkspaceSectionTabs.tsx`
- Create: `console/src/components/navigation/WorkspaceSectionTabs.test.tsx`

**Interfaces:**
- Produces: `ContentCenterTabs({ workspaceId, activeKey })` where activeKey is `templates | blog | file-manager`.
- Produces: `DataAnalyticsTabs({ workspaceId, activeKey })` where activeKey is `marketing | dashboard | live | explore | goals | filters | annotations`.
- Consumes: `useWorkspacePermissions(workspaceId)` and existing TanStack `Link` route patterns.

- [ ] **Step 1: Write failing route, active-state and permission tests**

Render both exported components with complete permissions and assert all literal labels and destinations. Re-render with templates denied and assert Template Management is absent; render analytics with message history only and web analytics only to prove the two permission families do not leak into each other.

- [ ] **Step 2: Run the focused test and verify RED**

```powershell
Set-Location console
npm test -- --run src/components/navigation/WorkspaceSectionTabs.test.tsx
Set-Location ..
```

Expected: FAIL because `WorkspaceSectionTabs.tsx` does not exist.

- [ ] **Step 3: Implement the smallest shared tab primitive and domain exports**

Use controlled Ant Design Tabs with route Link labels. Apply `aria-label` to the navigation container, `tabBarStyle={{ marginBottom: 20 }}`, and a class that limits overflow to the tab strip. Return no tab whose required read permission is false.

- [ ] **Step 4: Run the focused test and verify GREEN**

```powershell
Set-Location console
npm test -- --run src/components/navigation/WorkspaceSectionTabs.test.tsx
Set-Location ..
```

Expected: all component tests pass.

### Task 2: Restore Content Center secondary navigation

**Files:**
- Modify: `console/src/pages/TemplatesPage.tsx`
- Modify: `console/src/pages/BlogPage.tsx`
- Modify: `console/src/pages/FileManagerPage.tsx`
- Create: `console/src/pages/ContentCenterNavigation.test.tsx`

**Interfaces:**
- Consumes: `ContentCenterTabs` from Task 1.
- Produces: three existing pages with one stable Content Center tab row and correct active key.

- [ ] **Step 1: Write failing page integration tests**

Render each page with its external data services isolated and assert the Content Center navigation exists once with the correct selected tab.

- [ ] **Step 2: Run the focused test and verify RED**

```powershell
Set-Location console
npm test -- --run src/pages/ContentCenterNavigation.test.tsx
Set-Location ..
```

Expected: FAIL because the pages do not render ContentCenterTabs.

- [ ] **Step 3: Mount ContentCenterTabs in all three pages**

Place the tab row below the page title and above page-specific filters or content. Keep the Blog category sidebar and File Manager provider unchanged.

- [ ] **Step 4: Run the focused tests and verify GREEN**

```powershell
Set-Location console
npm test -- --run src/components/navigation/WorkspaceSectionTabs.test.tsx src/pages/ContentCenterNavigation.test.tsx
Set-Location ..
```

Expected: both test files pass.

### Task 3: Restore Data Analytics secondary navigation

**Files:**
- Modify: `console/src/pages/AnalyticsPage.tsx`
- Modify: `console/src/pages/WebAnalyticsPage.tsx`
- Modify: `console/src/pages/WebAnalyticsLivePage.tsx`
- Create: `console/src/pages/DataAnalyticsNavigation.test.tsx`

**Interfaces:**
- Consumes: `DataAnalyticsTabs` from Task 1.
- Produces: marketing, dashboard, live, explore, goals, filters and annotations routes with route-consistent active tabs.

- [ ] **Step 1: Write failing active-route integration tests**

Test Marketing Overview on AnalyticsPage, each WebAnalytics tab value, and Live Visitors on WebAnalyticsLivePage. Assert the Live page no longer depends on its back link as the only way to switch sections.

- [ ] **Step 2: Run the focused test and verify RED**

```powershell
Set-Location console
npm test -- --run src/pages/DataAnalyticsNavigation.test.tsx
Set-Location ..
```

Expected: FAIL because the pages do not render DataAnalyticsTabs.

- [ ] **Step 3: Mount DataAnalyticsTabs and preserve page controls**

Add the navigation below each data page title area. Map the WebAnalytics route parameter directly to the matching active key and use `live` for the separate real-time route. Retain providers, gates, filters, period controls, CSV export and AI assistant placement.

- [ ] **Step 4: Run the focused tests and verify GREEN**

```powershell
Set-Location console
npm test -- --run src/components/navigation/WorkspaceSectionTabs.test.tsx src/pages/DataAnalyticsNavigation.test.tsx
Set-Location ..
```

Expected: all analytics navigation tests pass.

### Task 4: Align sidebar landing permissions and regression contracts

**Files:**
- Modify: `console/src/layouts/WorkspaceLayout.tsx`
- Modify: `console/src/__tests__/WorkspaceLayout.test.tsx`
- Modify: `console/src/i18n/zhCN.priority.test.ts`
- Modify generated catalogs only after preserving unrelated working-tree hunks: `console/src/i18n/locales/*.po`, `console/src/i18n/locales/*.js`

**Interfaces:**
- Consumes: existing `hasAccess(resource)` helper.
- Produces: Data Analytics entry visible for `message_history || web_analytics`, landing at `/analytics` first and website dashboard as fallback.

- [ ] **Step 1: Write failing landing and flat-sidebar tests**

Add cases for message-history-only and web-analytics-only users. Keep the exact nine-entry and no-submenu assertion.

- [ ] **Step 2: Run WorkspaceLayout test and verify RED**

```powershell
Set-Location console
npm test -- --run src/__tests__/WorkspaceLayout.test.tsx
Set-Location ..
```

Expected: the message-history-only landing expectation fails against the current web-analytics-only gate.

- [ ] **Step 3: Update landing route and localized labels**

Change only the Data Analytics visibility/default target. Extract and compile Lingui catalogs, merge the new labels with existing dirty localization work, and preserve all unrelated entries.

- [ ] **Step 4: Run focused navigation and localization tests**

```powershell
Set-Location console
npm test -- --run src/__tests__/WorkspaceLayout.test.tsx src/components/navigation/WorkspaceSectionTabs.test.tsx src/pages/ContentCenterNavigation.test.tsx src/pages/DataAnalyticsNavigation.test.tsx src/i18n/zhCN.priority.test.ts
Set-Location ..
```

Expected: all focused tests pass.

### Task 5: Full console verification and scoped commit

**Files:**
- Verify all files from Tasks 1-4.

**Interfaces:**
- Produces: buildable and lint-clean console change without staging unrelated work.

- [ ] **Step 1: Run the complete console test suite**

```powershell
Set-Location console
npm test -- --run
Set-Location ..
```

- [ ] **Step 2: Run lint and production build**

```powershell
Set-Location console
npm run lint
npm run build
Set-Location ..
```

- [ ] **Step 3: Review the scoped diff**

Use `git diff --check` and inspect only the navigation, page, test, spec and plan hunks. Confirm existing Delivery Center and migration changes remain unstaged.

- [ ] **Step 4: Commit only this feature**

```powershell
git add docs/superpowers/specs/2026-08-30-content-and-analytics-secondary-tabs-design.md docs/superpowers/plans/2026-08-30-content-and-analytics-secondary-tabs.md console/src/components/navigation/WorkspaceSectionTabs.tsx console/src/components/navigation/WorkspaceSectionTabs.test.tsx console/src/pages/TemplatesPage.tsx console/src/pages/BlogPage.tsx console/src/pages/FileManagerPage.tsx console/src/pages/AnalyticsPage.tsx console/src/pages/WebAnalyticsPage.tsx console/src/pages/WebAnalyticsLivePage.tsx console/src/pages/ContentCenterNavigation.test.tsx console/src/pages/DataAnalyticsNavigation.test.tsx console/src/layouts/WorkspaceLayout.tsx console/src/__tests__/WorkspaceLayout.test.tsx
git commit -m "feat(console): restore content and analytics secondary tabs"
```

Localization files with pre-existing unrelated changes must be staged by feature hunk only or left unstaged with an explicit handoff note.
