import { i18n, type Messages } from '@lingui/core'
import type { Locale } from './index'

export type UITranslations = Record<string, Record<string, string>>

interface WorkspaceScope {
  id: string
  overrides: UITranslations
}

const baseCatalogs = new Map<Locale, Messages>()
const baseCatalogLoads = new Map<Locale, Promise<Messages>>()
let activeWorkspace: WorkspaceScope | null = null
let loadGeneration = 0

/**
 * Start a catalog operation. Only the newest operation may activate a locale.
 */
export function nextCatalogGeneration(): number {
  loadGeneration += 1
  return loadGeneration
}

export function isCatalogGenerationCurrent(generation: number): boolean {
  return generation === loadGeneration
}

/**
 * Load a pristine bundled catalog. Callers receive a copy so workspace merges
 * can never modify the catalog cached for later workspace switches.
 */
export async function getBaseCatalog(locale: Locale): Promise<Messages> {
  const cached = baseCatalogs.get(locale)
  if (cached) return { ...cached }

  let catalogLoad = baseCatalogLoads.get(locale)
  if (!catalogLoad) {
    catalogLoad = import(`./locales/${locale}.po`)
      .then(({ messages }) => {
        const baseCatalog = Object.freeze({ ...messages }) as Messages
        baseCatalogs.set(locale, baseCatalog)
        return baseCatalog
      })
      .catch((error) => {
        baseCatalogLoads.delete(locale)
        throw error
      })
    baseCatalogLoads.set(locale, catalogLoad)
  }

  return { ...(await catalogLoad) }
}

/**
 * Load the current workspace's effective catalog for a generation. The base
 * catalog is still loaded for stale calls so a failed newer request can fall
 * back to a catalog that did arrive.
 */
export async function loadWorkspaceCatalog(
  locale: Locale,
  generation: number,
): Promise<boolean> {
  const baseCatalog = await getBaseCatalog(locale)
  i18n.load(locale, mergeWorkspaceOverrides(locale, baseCatalog))
  return generation === loadGeneration
}

/**
 * Apply a workspace's sparse static-message overrides to the active locale.
 */
export async function setWorkspaceCatalog(
  workspaceId: string,
  overrides: UITranslations,
): Promise<void> {
  activeWorkspace = { id: workspaceId, overrides }
  const generation = nextCatalogGeneration()
  const locale = i18n.locale as Locale
  if (!locale) return

  restoreLoadedBaseCatalog(locale)
  if (await loadWorkspaceCatalog(locale, generation)) {
    i18n.activate(locale)
  }
}

/**
 * Remove the active workspace's overrides without clearing a newer scope.
 */
export async function clearWorkspaceCatalog(workspaceId: string): Promise<void> {
  if (activeWorkspace?.id !== workspaceId) return

  activeWorkspace = null
  const generation = nextCatalogGeneration()
  const locale = i18n.locale as Locale
  if (!locale) return

  restoreLoadedBaseCatalog(locale)
  if (await loadWorkspaceCatalog(locale, generation)) {
    i18n.activate(locale)
  }
}

function restoreLoadedBaseCatalog(locale: Locale): void {
  const baseCatalog = baseCatalogs.get(locale)
  if (baseCatalog) {
    i18n.load(locale, { ...baseCatalog })
    i18n.activate(locale)
  }
}

function mergeWorkspaceOverrides(locale: Locale, baseCatalog: Messages): Messages {
  const overrides = activeWorkspace?.overrides[locale]
  if (!overrides) return { ...baseCatalog }

  const mergedCatalog = { ...baseCatalog }
  for (const [messageId, value] of Object.entries(overrides)) {
    if (Object.prototype.hasOwnProperty.call(baseCatalog, messageId)) {
      mergedCatalog[messageId] = value
    }
  }
  return mergedCatalog
}
