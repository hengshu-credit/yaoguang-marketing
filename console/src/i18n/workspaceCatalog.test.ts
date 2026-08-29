import { beforeEach, describe, expect, it, vi } from 'vitest'
import { i18n } from '@lingui/core'
import { loadLocale } from './index'
import {
  clearWorkspaceCatalog,
  getBaseCatalog,
  setWorkspaceCatalog,
} from './workspaceCatalog'

const frenchCatalog = vi.hoisted(() => {
  let resolve: ((messages: Record<string, string>) => void) | undefined
  const promise = new Promise<Record<string, string>>((done) => {
    resolve = done
  })
  return {
    promise,
    resolve: (messages: Record<string, string>) => resolve?.(messages),
  }
})

vi.mock('./locales/en.po', () => ({
  messages: { 'Sidebar title': 'Workspace', 'Another message': 'Base value' },
}))
vi.mock('./locales/es.po', () => ({
  messages: { 'Sidebar title': 'Espacio de trabajo', 'Another message': 'Valor base' },
}))
vi.mock('./locales/fr.po', async () => ({ messages: await frenchCatalog.promise }))

describe('workspace catalogs', () => {
  beforeEach(() => {
    i18n.load('en', {})
    i18n.activate('en')
  })

  it('applies matching locale overrides without changing another locale base catalog', async () => {
    await loadLocale('en')
    await setWorkspaceCatalog('workspace-a', { en: { 'Sidebar title': 'Team space' } })

    expect(i18n._('Sidebar title')).toBe('Team space')

    await loadLocale('es')
    expect(i18n._('Sidebar title')).toBe('Espacio de trabajo')
  })

  it('ignores an override for an unknown message ID', async () => {
    await loadLocale('en')
    await setWorkspaceCatalog('workspace-a', { en: { 'Unknown message': 'Do not add me' } })

    expect(i18n._('Unknown message')).toBe('Unknown message')
    expect(i18n._('Another message')).toBe('Base value')
  })

  it('restores the bundled catalog after clearing its active workspace', async () => {
    await loadLocale('en')
    await setWorkspaceCatalog('workspace-a', { en: { 'Sidebar title': 'Team space' } })
    await clearWorkspaceCatalog('workspace-a')

    expect(i18n._('Sidebar title')).toBe('Workspace')
  })

  it('does not carry an old workspace value into the next workspace', async () => {
    await loadLocale('en')
    await setWorkspaceCatalog('workspace-a', { en: { 'Sidebar title': 'A space' } })
    await setWorkspaceCatalog('workspace-b', { en: { 'Another message': 'B value' } })

    expect(i18n._('Sidebar title')).toBe('Workspace')
    expect(i18n._('Another message')).toBe('B value')
  })

  it('does not let a stale workspace load activate after the next workspace', async () => {
    i18n.load('fr', {})
    i18n.activate('fr')

    const staleWorkspace = setWorkspaceCatalog('workspace-a', { fr: { 'Sidebar title': 'A space' } })
    const currentWorkspace = setWorkspaceCatalog('workspace-b', { fr: { 'Sidebar title': 'B space' } })
    frenchCatalog.resolve({ 'Sidebar title': 'Espace de travail' })

    await Promise.all([staleWorkspace, currentWorkspace])

    expect(i18n._('Sidebar title')).toBe('B space')
  })

  it('returns a copy of the bundled catalog that cannot mutate later loads', async () => {
    const catalog = await getBaseCatalog('en')
    catalog['Sidebar title'] = 'Mutated copy'

    await loadLocale('en')
    expect(i18n._('Sidebar title')).toBe('Workspace')
  })
})
