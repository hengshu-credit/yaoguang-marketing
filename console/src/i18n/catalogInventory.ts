import { locales, type Locale } from './index'
import { parsePOCatalog, type POEntry } from './po'

export interface CompiledCatalog {
  [messageId: string]: unknown
}

export interface TranslationItem {
  /** Persisted Lingui compiled-message ID. */
  id: string
  /** English source literal shown to workspace owners. */
  source: string
  references: string[]
  menuKey: string
  pageKey: string
  /** Bundled value for every supported UI locale, falling back to English. */
  values: Record<Locale, string>
}

interface Hierarchy {
  menuKey: string
  pageKey: string
  order: number
}

interface HierarchyRule extends Omit<Hierarchy, 'order'> {
  matches: RegExp
}

// These rules are intentionally ordered. One source string can have several
// references, so the first product area below gives the item a stable home.
const hierarchyRules: HierarchyRule[] = [
  { menuKey: 'Navigation', pageKey: 'Workspace', matches: /^src\/layouts\/WorkspaceLayout\.tsx:/ },
  { menuKey: 'Settings', pageKey: 'Navigation', matches: /^src\/components\/settings\/SettingsSidebar\.tsx:/ },
  { menuKey: 'Settings', pageKey: 'Workspace', matches: /^src\/(?:components\/settings\/|pages\/WorkspaceSettingsPage\.tsx:)/ },
  { menuKey: 'Automations', pageKey: 'Automations', matches: /^src\/(?:components\/automations\/|pages\/AutomationsPage\.tsx:)/ },
  { menuKey: 'Web Analytics', pageKey: 'Web Analytics', matches: /^src\/(?:components\/web_analytics\/|pages\/WebAnalytics[^/]*\.tsx:)/ },
  { menuKey: 'Transactional Notifications', pageKey: 'Transactional Notifications', matches: /^src\/(?:components\/transactional\/|pages\/Transactional[^/]*\.tsx:)/ },
  { menuKey: 'Templates', pageKey: 'Templates', matches: /^src\/(?:components\/templates\/|pages\/TemplatesPage\.tsx:)/ },
  { menuKey: 'Lists', pageKey: 'Lists', matches: /^src\/(?:components\/lists\/|pages\/ListsPage\.tsx:)/ },
  { menuKey: 'Contacts', pageKey: 'Contacts', matches: /^src\/(?:components\/contacts\/|pages\/ContactsPage\.tsx:)/ },
  { menuKey: 'Segments', pageKey: 'Segments', matches: /^src\/(?:components\/segments\/|pages\/SegmentsPage\.tsx:)/ },
  { menuKey: 'Broadcasts', pageKey: 'Broadcasts', matches: /^src\/(?:components\/broadcasts\/|pages\/BroadcastsPage\.tsx:)/ },
  { menuKey: 'Blog', pageKey: 'Blog', matches: /^src\/(?:components\/blog\/|pages\/Blog[^/]*\.tsx:)/ },
]

const sharedHierarchy: Hierarchy = {
  menuKey: 'Shared',
  pageKey: 'Other',
  order: hierarchyRules.length,
}

let staticInventoryLoad: Promise<TranslationItem[]> | null = null

/**
 * Load the editable static-message inventory without making the app's normal
 * startup path download the English PO source or every locale catalog.
 */
export function loadStaticCatalogInventory(): Promise<TranslationItem[]> {
  if (!staticInventoryLoad) {
    staticInventoryLoad = Promise.all([
      import('./locales/en.po?raw').then((module) => module.default),
      loadCompiledCatalogs(),
    ])
      .then(([source, catalogs]) => buildStaticCatalogInventory(parsePOCatalog(source), catalogs))
      .catch((error) => {
        staticInventoryLoad = null
        throw error
      })
  }
  return staticInventoryLoad
}

/** Return the supported UI locales with the active locale in the first column. */
export function orderLocales(active: Locale): Locale[] {
  return [active, ...locales.filter((locale) => locale !== active)]
}

/**
 * Join English source entries to their compiled IDs and locale bundles. Kept
 * separate from lazy imports so the inventory rules have a small, real-data
 * test surface.
 */
export function buildStaticCatalogInventory(
  entries: POEntry[],
  catalogs: Record<Locale, CompiledCatalog>,
): TranslationItem[] {
  const idsBySource = new Map<string, string[]>()

  for (const [id, compiledMessage] of Object.entries(catalogs.en)) {
    const source = simpleLiteral(compiledMessage)
    if (source === null) continue
    const ids = idsBySource.get(source)
    if (ids) ids.push(id)
    else idsBySource.set(source, [id])
  }

  const seenIds = new Set<string>()
  const classifiedItems: Array<{ item: TranslationItem; hierarchy: Hierarchy }> = []
  for (const entry of entries) {
    for (const id of idsBySource.get(entry.msgid) ?? []) {
      if (seenIds.has(id)) continue
      seenIds.add(id)

      const hierarchy = classifyReferences(entry.references)
      classifiedItems.push({
        hierarchy,
        item: {
          id,
          source: entry.msgid,
          references: [...entry.references],
          menuKey: hierarchy.menuKey,
          pageKey: hierarchy.pageKey,
          values: bundledValues(id, entry.msgid, catalogs),
        },
      })
    }
  }

  return classifiedItems
    .sort((left, right) => compareItems(left.item, left.hierarchy, right.item, right.hierarchy))
    .map(({ item }) => item)
}

async function loadCompiledCatalogs(): Promise<Record<Locale, CompiledCatalog>> {
  const modules = await Promise.all(locales.map(async (locale) => [locale, await loadCompiledCatalog(locale)] as const))
  return Object.fromEntries(modules.map(([locale, module]) => [locale, module.messages])) as Record<Locale, CompiledCatalog>
}

async function loadCompiledCatalog(locale: Locale): Promise<{ messages: CompiledCatalog }> {
  switch (locale) {
    case 'en': return import('./locales/en.po') as Promise<{ messages: CompiledCatalog }>
    case 'fr': return import('./locales/fr.po') as Promise<{ messages: CompiledCatalog }>
    case 'es': return import('./locales/es.po') as Promise<{ messages: CompiledCatalog }>
    case 'de': return import('./locales/de.po') as Promise<{ messages: CompiledCatalog }>
    case 'ca': return import('./locales/ca.po') as Promise<{ messages: CompiledCatalog }>
    case 'pt-BR': return import('./locales/pt-BR.po') as Promise<{ messages: CompiledCatalog }>
    case 'ja': return import('./locales/ja.po') as Promise<{ messages: CompiledCatalog }>
    case 'it': return import('./locales/it.po') as Promise<{ messages: CompiledCatalog }>
    case 'zh-CN': return import('./locales/zh-CN.po') as Promise<{ messages: CompiledCatalog }>
  }
}

function bundledValues(
  id: string,
  source: string,
  catalogs: Record<Locale, CompiledCatalog>,
): Record<Locale, string> {
  return Object.fromEntries(
    locales.map((locale) => [locale, simpleLiteral(catalogs[locale][id]) ?? source]),
  ) as Record<Locale, string>
}

function simpleLiteral(compiledMessage: unknown): string | null {
  return Array.isArray(compiledMessage) && compiledMessage.length === 1 && typeof compiledMessage[0] === 'string'
    ? compiledMessage[0]
    : null
}

function classifyReferences(references: string[]): Hierarchy {
  const ruleIndex = hierarchyRules.findIndex((rule) => references.some((reference) => rule.matches.test(reference)))
  if (ruleIndex === -1) return sharedHierarchy

  const rule = hierarchyRules[ruleIndex]
  return { menuKey: rule.menuKey, pageKey: rule.pageKey, order: ruleIndex }
}

function compareItems(
  left: TranslationItem,
  leftHierarchy: Hierarchy,
  right: TranslationItem,
  rightHierarchy: Hierarchy,
): number {
  return leftHierarchy.order - rightHierarchy.order
    || compareText(left.menuKey, right.menuKey)
    || compareText(left.pageKey, right.pageKey)
    || compareText(left.source, right.source)
    || compareText(left.id, right.id)
}

function compareText(left: string, right: string): number {
  return left < right ? -1 : left > right ? 1 : 0
}
