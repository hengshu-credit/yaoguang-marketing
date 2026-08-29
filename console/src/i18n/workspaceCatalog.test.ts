import { beforeEach, describe, expect, it, vi } from 'vitest'
import { i18n } from '@lingui/core'
import { loadLocale } from './index'
import {
  clearWorkspaceCatalog,
  getBaseCatalog,
  setWorkspaceCatalog,
} from './workspaceCatalog'

const frenchCatalog = vi.hoisted(() => {
  let resolve: ((messages: Record<string, string[]>) => void) | undefined
  const promise = new Promise<Record<string, string[]>>((done) => {
    resolve = done
  })
  return {
    promise,
    resolve: (messages: Record<string, string[]>) => resolve?.(messages),
  }
})

vi.mock('./locales/en.po', () => ({
  messages: { Dashboard: ['Dashboard'], Settings: ['Settings'] },
}))
vi.mock('./locales/es.po', () => ({
  messages: { Dashboard: ['Tablero'], Settings: ['Configuración'] },
}))
vi.mock('./locales/fr.po', async () => ({ messages: await frenchCatalog.promise }))

describe('workspace catalogs', () => {
  beforeEach(() => {
    i18n.load('en', {})
    i18n.activate('en')
  })

  it('applies matching locale overrides without changing another locale base catalog', async () => {
    await loadLocale('en')
    await setWorkspaceCatalog('workspace-a', { en: { Dashboard: 'Team dashboard' } })

    expect(i18n._('Dashboard')).toBe('Team dashboard')
    expect(i18n.messages['Dashboard']).toEqual(['Team dashboard'])

    await loadLocale('es')
    expect(i18n._('Dashboard')).toBe('Tablero')
  })

  it('ignores an override for an unknown message ID', async () => {
    await loadLocale('en')
    await setWorkspaceCatalog('workspace-a', { en: { UnknownCatalogId: 'Do not add me' } })

    expect(i18n._('UnknownCatalogId')).toBe('UnknownCatalogId')
    expect(i18n._('Settings')).toBe('Settings')
  })

  it('restores the bundled catalog after clearing its active workspace', async () => {
    await loadLocale('en')
    await setWorkspaceCatalog('workspace-a', { en: { Dashboard: 'Team dashboard' } })
    await clearWorkspaceCatalog('workspace-a')

    expect(i18n._('Dashboard')).toBe('Dashboard')
  })

  it('does not carry an old workspace value into the next workspace', async () => {
    await loadLocale('en')
    await setWorkspaceCatalog('workspace-a', { en: { Dashboard: 'A dashboard' } })
    await setWorkspaceCatalog('workspace-b', { en: { Settings: 'B settings' } })

    expect(i18n._('Dashboard')).toBe('Dashboard')
    expect(i18n._('Settings')).toBe('B settings')
  })

  it('does not let a stale workspace load activate after the next workspace', async () => {
    i18n.load('fr', {})
    i18n.activate('fr')

    const staleWorkspace = setWorkspaceCatalog('workspace-a', { fr: { Dashboard: 'A dashboard' } })
    const currentWorkspace = setWorkspaceCatalog('workspace-b', { fr: { Dashboard: 'B dashboard' } })
    frenchCatalog.resolve({ Dashboard: ['Tableau de bord'] })

    await Promise.all([staleWorkspace, currentWorkspace])

    expect(i18n._('Dashboard')).toBe('B dashboard')
  })

  it('does not let a caller mutate a nested compiled message in the cached catalog', async () => {
    const catalog = await getBaseCatalog('en')
    ;(catalog['Dashboard'] as string[])[0] = 'Mutated dashboard'

    await loadLocale('en')
    expect(i18n._('Dashboard')).toBe('Dashboard')
  })
})
