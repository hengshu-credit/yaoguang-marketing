import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { Locale } from '../../i18n'
import { localeNames, locales } from '../../i18n'
import type { TranslationItem } from '../../i18n/catalogInventory'
import type { Workspace } from '../../services/api/workspace'
import { workspaceService } from '../../services/api/workspace'
import { UITranslationsSettings } from './UITranslationsSettings'

vi.mock('@tanstack/react-router', () => ({
  useBlocker: () => ({ status: 'idle', proceed: undefined, reset: undefined })
}))

vi.mock('../../i18n/catalogInventory', async () => {
  const actual = await vi.importActual<typeof import('../../i18n/catalogInventory')>(
    '../../i18n/catalogInventory'
  )
  return { ...actual, loadStaticCatalogInventory: vi.fn() }
})

vi.mock('../../services/api/workspace', () => {
  return {
    workspaceService: {
      setUITranslations: vi.fn().mockResolvedValue({ status: 'success', message: 'saved' })
    }
  }
})

import { loadStaticCatalogInventory } from '../../i18n/catalogInventory'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const localeValues = (source: string): Record<Locale, string> =>
  Object.fromEntries(
    locales.map((locale) => [locale, locale === 'en' ? source : `${source} (${locale})`])
  ) as Record<Locale, string>

const inventory: TranslationItem[] = [
  {
    id: 'workspaceName',
    source: 'Workspace name',
    references: ['src/components/settings/GeneralSettings.tsx:1'],
    menuKey: 'Settings',
    pageKey: 'General',
    values: localeValues('Workspace name')
  },
  {
    id: 'workspaceTimezone',
    source: 'Workspace timezone',
    references: ['src/components/settings/GeneralSettings.tsx:2'],
    menuKey: 'Settings',
    pageKey: 'General',
    values: localeValues('Workspace timezone')
  },
  {
    id: 'blogTitle',
    source: 'Blog title',
    references: ['src/components/settings/BlogSettings.tsx:1'],
    menuKey: 'Settings',
    pageKey: 'Blog',
    values: localeValues('Blog title')
  }
]

const makeWorkspace = (
  uiTranslations: Record<string, Record<string, string>> = {}
): Workspace =>
  ({
    id: 'workspace-1',
    name: 'Workspace One',
    settings: {
      timezone: 'UTC',
      email_tracking_enabled: true,
      default_language: 'en',
      languages: ['en'],
      ui_translations: uiTranslations
    },
    created_at: '2026-08-30T00:00:00Z',
    updated_at: '2026-08-30T00:00:00Z'
  }) as Workspace

interface RenderOptions {
  currentLocale?: Locale
  isOwner?: boolean
  overrides?: Record<string, Record<string, string>>
}

const renderSettings = ({
  currentLocale = 'zh-CN',
  isOwner = true,
  overrides = {}
}: RenderOptions = {}) => {
  const refreshWorkspaces = vi.fn().mockResolvedValue(undefined)
  render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <UITranslationsSettings
            workspace={makeWorkspace(overrides)}
            isOwner={isOwner}
            currentLocale={currentLocale}
            refreshWorkspaces={refreshWorkspaces}
          />
        </App>
      </ConfigProvider>
    </I18nProvider>
  )
  return { refreshWorkspaces }
}

const searchFor = async (query: string) => {
  const search = await screen.findByRole('searchbox', { name: 'Search translations' })
  fireEvent.change(search, { target: { value: query } })
}

const translationInput = async (source: string, locale: Locale = 'zh-CN') =>
  screen.findByRole('textbox', {
    name: `Translation for ${source} in ${localeNames[locale]}`
  })

const save = () => fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))

describe('UITranslationsSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(loadStaticCatalogInventory).mockResolvedValue(inventory)
    vi.mocked(workspaceService.setUITranslations).mockResolvedValue({
      status: 'success',
      message: 'saved'
    })
  })

  it('orders all supported locale columns with the current locale first', async () => {
    renderSettings({ currentLocale: 'zh-CN' })

    const region = await screen.findByRole('region', { name: 'Workspace UI translations' })
    const header = region.querySelector('.ant-table-header')
    expect(header).not.toBeNull()
    const headers = within(header as HTMLElement)
      .getAllByRole('columnheader')
      .map((header) => header.textContent?.replace('Current', '').trim())

    expect(headers.slice(1)).toEqual([
      '简体中文',
      'English',
      'Français',
      'Español',
      'Deutsch',
      'Català',
      'Português (Brasil)',
      '日本語',
      'Italiano'
    ])
  })

  it('keeps the complete header visible while the translation grid scrolls', async () => {
    renderSettings()

    const region = await screen.findByRole('region', { name: 'Workspace UI translations' })
    const stickyHeader = region.querySelector('.ant-table-header')

    expect(stickyHeader).not.toBeNull()
    expect(within(stickyHeader as HTMLElement).getAllByRole('columnheader')).toHaveLength(
      locales.length + 1
    )
  })

  it('marks both frozen columns as opaque scroll boundaries', async () => {
    renderSettings()

    const region = await screen.findByRole('region', { name: 'Workspace UI translations' })
    const stickyHeader = region.querySelector('.ant-table-header')
    const body = region.querySelector('.ant-table-tbody')
    const firstDataRow = body?.querySelector('tr.ant-table-row')

    expect(stickyHeader?.querySelectorAll('th.translations-fixed-col')).toHaveLength(2)
    expect(firstDataRow?.querySelectorAll('td.translations-fixed-col')).toHaveLength(2)
  })

  it('gives every frozen cell its own non-transparent background', async () => {
    renderSettings()

    const region = await screen.findByRole('region', { name: 'Workspace UI translations' })
    const frozenCells = region.querySelectorAll<HTMLElement>('.translations-fixed-col')

    expect(frozenCells.length).toBeGreaterThan(0)
    for (const cell of frozenCells) {
      expect(cell.style.backgroundColor).not.toBe('')
      expect(cell.style.backgroundColor).not.toBe('transparent')
    }
  })

  it('keeps the declared frozen-column widths during horizontal scrolling', async () => {
    renderSettings()

    const region = await screen.findByRole('region', { name: 'Workspace UI translations' })
    const bodyTable = region.querySelector<HTMLTableElement>('.ant-table-body table')
    const columns = bodyTable?.querySelectorAll('col')

    expect(bodyTable).not.toBeNull()
    expect(bodyTable?.getAttribute('style')).toContain('table-layout: fixed')
    expect(
      [...(columns ?? [])].slice(0, 2).map((column) => column.getAttribute('style'))
    ).toEqual(['width: 260px;', 'width: 290px;'])
  })

  it('expands and collapses hierarchy rows when their content is clicked', async () => {
    renderSettings()

    const menuRow = (await screen.findByText('Settings')).closest('tr')
    expect(menuRow).not.toBeNull()

    fireEvent.click(menuRow as HTMLTableRowElement)
    const pageRow = (await screen.findByText('General')).closest('tr')
    expect(pageRow).not.toBeNull()

    fireEvent.click(pageRow as HTMLTableRowElement)
    expect(await screen.findByText('Workspace name')).toBeInTheDocument()

    fireEvent.click(menuRow as HTMLTableRowElement)
    await waitFor(() => expect(screen.queryByText('Workspace name')).not.toBeInTheDocument())
  })

  it('starts with menu and page rows collapsed and expands matching ancestors during search', async () => {
    renderSettings()

    await screen.findByText('Settings')
    expect(screen.queryByText('Workspace name')).not.toBeInTheDocument()

    await searchFor('Workspace name')
    expect(await translationInput('Workspace name')).toBeInTheDocument()

    fireEvent.change(screen.getByRole('searchbox', { name: 'Search translations' }), {
      target: { value: '' }
    })
    await waitFor(() => expect(screen.queryByText('Workspace name')).not.toBeInTheDocument())
  })

  it('finds descendants by their localized hierarchy labels', async () => {
    renderSettings()

    await searchFor('Settings')

    expect(await translationInput('Workspace name')).toBeInTheDocument()
    expect(await translationInput('Blog title')).toBeInTheDocument()
  })

  it('stores only the edited cell and removes it when the bundled default is entered', async () => {
    renderSettings()
    await searchFor('Workspace name')
    const input = await translationInput('Workspace name')

    fireEvent.change(input, { target: { value: '工作区名称' } })
    expect(screen.getByText('Override')).toBeInTheDocument()
    save()

    await waitFor(() =>
      expect(workspaceService.setUITranslations).toHaveBeenLastCalledWith({
        workspace_id: 'workspace-1',
        ui_translations: { 'zh-CN': { workspaceName: '工作区名称' } }
      })
    )

    fireEvent.change(input, { target: { value: 'Workspace name (zh-CN)' } })
    save()
    await waitFor(() =>
      expect(workspaceService.setUITranslations).toHaveBeenLastCalledWith({
        workspace_id: 'workspace-1',
        ui_translations: {}
      })
    )
  })

  it('rejects whitespace-only edits inline without calling the API', async () => {
    renderSettings()
    await searchFor('Workspace name')

    fireEvent.change(await translationInput('Workspace name'), { target: { value: '   ' } })

    expect(
      screen.getByRole('alert', {
        name: 'Enter a translation or use Restore to inherit the default'
      })
    ).toBeInTheDocument()
    save()
    expect(workspaceService.setUITranslations).not.toHaveBeenCalled()
  })

  it('restores one cell without removing another locale override', async () => {
    renderSettings({
      overrides: {
        'zh-CN': { workspaceName: '工作区名称' },
        fr: { workspaceName: 'Nom personnalisé' }
      }
    })
    await searchFor('Workspace name')

    fireEvent.click(
      screen.getByRole('button', { name: 'Restore Workspace name in 简体中文' })
    )
    save()

    await waitFor(() =>
      expect(workspaceService.setUITranslations).toHaveBeenCalledWith({
        workspace_id: 'workspace-1',
        ui_translations: { fr: { workspaceName: 'Nom personnalisé' } }
      })
    )
  })

  it('restores every locale for one item without touching sibling items', async () => {
    renderSettings({
      overrides: {
        'zh-CN': { workspaceName: '工作区名称', workspaceTimezone: '时区' },
        fr: { workspaceName: 'Nom personnalisé' }
      }
    })
    await searchFor('Workspace name')

    fireEvent.click(screen.getByRole('button', { name: 'Restore item Workspace name' }))
    save()

    await waitFor(() =>
      expect(workspaceService.setUITranslations).toHaveBeenCalledWith({
        workspace_id: 'workspace-1',
        ui_translations: { 'zh-CN': { workspaceTimezone: '时区' } }
      })
    )
  })

  it('restores only descendants of a page', async () => {
    renderSettings({
      overrides: {
        'zh-CN': {
          workspaceName: '工作区名称',
          workspaceTimezone: '时区',
          blogTitle: '博客标题'
        }
      }
    })
    await searchFor('Workspace name')

    fireEvent.click(screen.getByRole('button', { name: 'Restore page General' }))
    save()

    await waitFor(() =>
      expect(workspaceService.setUITranslations).toHaveBeenCalledWith({
        workspace_id: 'workspace-1',
        ui_translations: { 'zh-CN': { blogTitle: '博客标题' } }
      })
    )
  })

  it('restores every descendant of a menu', async () => {
    renderSettings({
      overrides: {
        'zh-CN': { workspaceName: '工作区名称', blogTitle: '博客标题' },
        fr: { workspaceTimezone: 'Fuseau personnalisé' }
      }
    })
    await searchFor('Workspace name')

    fireEvent.click(screen.getByRole('button', { name: 'Restore menu Settings' }))
    save()

    await waitFor(() =>
      expect(workspaceService.setUITranslations).toHaveBeenCalledWith({
        workspace_id: 'workspace-1',
        ui_translations: {}
      })
    )
  })

  it('requires confirmation before restoring all overrides', async () => {
    renderSettings({ overrides: { 'zh-CN': { workspaceName: '工作区名称' } } })

    fireEvent.click(await screen.findByRole('button', { name: 'Restore all' }))
    expect(workspaceService.setUITranslations).not.toHaveBeenCalled()
    fireEvent.click(await screen.findByRole('button', { name: 'Restore all overrides' }))
    save()

    await waitFor(() =>
      expect(workspaceService.setUITranslations).toHaveBeenCalledWith({
        workspace_id: 'workspace-1',
        ui_translations: {}
      })
    )
  })

  it('refreshes workspace state after saving the exact sparse map', async () => {
    const { refreshWorkspaces } = renderSettings()
    await searchFor('Blog title')
    fireEvent.change(await translationInput('Blog title', 'fr'), {
      target: { value: 'Titre du blog personnalisé' }
    })

    save()

    await waitFor(() => expect(refreshWorkspaces).toHaveBeenCalledOnce())
    expect(workspaceService.setUITranslations).toHaveBeenCalledWith({
      workspace_id: 'workspace-1',
      ui_translations: { fr: { blogTitle: 'Titre du blog personnalisé' } }
    })
  })

  it('discards edits back to the saved snapshot', async () => {
    renderSettings({ overrides: { 'zh-CN': { workspaceName: '已保存名称' } } })
    await searchFor('Workspace name')
    const input = await translationInput('Workspace name')
    fireEvent.change(input, { target: { value: '未保存名称' } })

    fireEvent.click(screen.getByRole('button', { name: 'Discard' }))

    expect(input).toHaveValue('已保存名称')
    expect(screen.queryByRole('button', { name: 'Save Changes' })).not.toBeInTheDocument()
  })

  it('does not expose translation inputs or save controls to a non-owner', async () => {
    renderSettings({ isOwner: false })

    expect(await screen.findByText('Only workspace owners can manage UI translations.')).toBeInTheDocument()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Save Changes' })).not.toBeInTheDocument()
  })
})
