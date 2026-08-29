import { describe, expect, it } from 'vitest'
import { locales, type Locale } from './index'
import { buildStaticCatalogInventory, orderLocales, type CompiledCatalog } from './catalogInventory'
import type { POEntry } from './po'

const catalog = (messages: CompiledCatalog): Record<Locale, CompiledCatalog> =>
  Object.fromEntries(locales.map((locale) => [locale, { ...messages }])) as Record<Locale, CompiledCatalog>

describe('catalog inventory', () => {
  it('includes only compiled single-literal messages and supplies every locale with English fallback', () => {
    const entries: POEntry[] = [
      {
        msgid: 'Create automation',
        msgstr: 'Create automation',
        references: ['src/components/automations/AutomationDrawer.tsx:12'],
      },
      {
        msgid: 'Hello {name}',
        msgstr: 'Hello {name}',
        references: ['src/components/automations/AutomationDrawer.tsx:13'],
      },
      {
        msgid: 'Save',
        msgstr: 'Save',
        references: ['src/unknown/SaveButton.tsx:5'],
      },
    ]
    const catalogs = catalog({
      automation: ['Create automation'],
      dynamic: ['Hello ', ['name']],
      save: ['Save'],
    })
    catalogs.fr = { ...catalogs.fr, automation: ['Créer une automatisation'] }
    delete catalogs.it.save

    const inventory = buildStaticCatalogInventory(entries, catalogs)

    expect(inventory.map((item) => item.id)).toEqual(['automation', 'save'])
    expect(inventory[0]).toMatchObject({
      source: 'Create automation',
      menuKey: 'Automations',
      pageKey: 'Automations',
      references: ['src/components/automations/AutomationDrawer.tsx:12'],
    })
    expect(inventory[0].values.fr).toBe('Créer une automatisation')
    expect(inventory[0].values.it).toBe('Create automation')
    expect(Object.keys(inventory[0].values)).toEqual(locales)
    expect(inventory[1]).toMatchObject({ menuKey: 'Shared', pageKey: 'Other' })
  })

  it('keeps a deterministic hierarchy and places the active locale first without dropping any locale', () => {
    const entries: POEntry[] = [
      { msgid: 'Shared', msgstr: 'Shared', references: ['src/unknown/A.tsx:1'] },
      { msgid: 'Template', msgstr: 'Template', references: ['src/components/templates/A.tsx:1'] },
      { msgid: 'Dashboard', msgstr: 'Dashboard', references: ['src/layouts/WorkspaceLayout.tsx:1'] },
      { msgid: 'Create automation', msgstr: 'Create automation', references: ['src/pages/AutomationsPage.tsx:1'] },
    ]
    const catalogs = catalog({
      shared: ['Shared'],
      template: ['Template'],
      dashboard: ['Dashboard'],
      automationPage: ['Create automation'],
    })

    expect(buildStaticCatalogInventory(entries, catalogs).map((item) => item.id)).toEqual([
      'dashboard',
      'automationPage',
      'template',
      'shared',
    ])
    expect(orderLocales('zh-CN')).toEqual([
      'zh-CN',
      'en',
      'fr',
      'es',
      'de',
      'ca',
      'pt-BR',
      'ja',
      'it',
    ])
  })

  it('excludes test-only explicit IDs without losing a colliding runtime message ID', () => {
    const entries: POEntry[] = [
      {
        msgid: 'nav.dashboard',
        msgstr: 'Dashboard',
        references: ['src/__tests__/helpers/catalog.ts:101'],
        isExplicitId: true,
      },
      {
        msgid: 'Dashboard',
        msgstr: 'Dashboard',
        references: ['src/layouts/WorkspaceLayout.tsx:298'],
      },
      {
        msgid: 'Sidebar title',
        msgstr: 'Sidebar title',
        references: ['src/__mocks__/workspaceCatalog.ts:39'],
        isExplicitId: true,
      },
      {
        msgid: 'nav.settings',
        msgstr: 'Settings',
        references: ['src/layouts/WorkspaceLayout.tsx:300'],
        isExplicitId: true,
      },
      {
        msgid: 'Save',
        msgstr: 'Save',
        references: ['src/i18n/po.test.ts:20', 'src/components/common/SaveButton.tsx:4'],
      },
    ]
    const inventory = buildStaticCatalogInventory(entries, catalog({
      'nav.dashboard': ['Dashboard'],
      '7p5kLi': ['Dashboard'],
      'Sidebar title': ['Sidebar title'],
      'nav.settings': ['Settings'],
      save: ['Save'],
    }))

    expect(inventory.map((item) => item.id)).toEqual(['7p5kLi', 'nav.settings', 'save'])
    expect(inventory[0]).toMatchObject({
      references: ['src/layouts/WorkspaceLayout.tsx:298'],
      menuKey: 'Navigation',
      pageKey: 'Workspace',
    })
    expect(inventory[1]).toMatchObject({
      id: 'nav.settings',
      source: 'Settings',
      values: expect.objectContaining({ en: 'Settings', it: 'Settings' }),
    })
    expect(inventory[2].references).toEqual(['src/components/common/SaveButton.tsx:4'])
  })

  it.each([
    ['segment component', 'src/components/segment/input.tsx:20', 'Segments'],
    ['debug segment page', 'src/pages/DebugSegmentPage.tsx:20', 'Segments'],
    ['dashboard', 'src/pages/DashboardPage.tsx:20', 'Dashboard'],
    ['analytics', 'src/pages/AnalyticsPage.tsx:20', 'Analytics'],
    ['file manager', 'src/components/file_manager/fileManager.tsx:20', 'File Manager'],
    ['logs', 'src/pages/LogsPage.tsx:20', 'Logs'],
    ['integration', 'src/components/integrations/LLMIntegration.tsx:20', 'Integrations'],
    ['webhook', 'src/components/webhooks/OutgoingWebhooksTab.tsx:20', 'Webhooks'],
  ])('classifies %s sources outside the priority pages', (_name, reference, menuKey) => {
    const inventory = buildStaticCatalogInventory([
      { msgid: 'Example', msgstr: 'Example', references: [reference] },
    ], catalog({ example: ['Example'] }))

    expect(inventory[0]).toMatchObject({ menuKey, pageKey: menuKey })
  })
})
