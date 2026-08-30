import { useCallback, useEffect, useMemo, useState } from 'react'
import { Alert, App, Badge, Button, Input, Popconfirm, Space, Table } from 'antd'
import { SearchOutlined } from '@ant-design/icons'
import type { Key } from 'react'
import type { ColumnsType } from 'antd/es/table'
import { useLingui } from '@lingui/react/macro'
import { localeNames, locales, type Locale } from '../../i18n'
import {
  loadStaticCatalogInventory,
  orderLocales,
  type TranslationItem
} from '../../i18n/catalogInventory'
import {
  workspaceService,
  type UITranslations,
  type Workspace
} from '../../services/api/workspace'
import { SettingsSaveBar } from './SettingsSaveBar'
import { SettingsSectionHeader } from './SettingsSectionHeader'

interface UITranslationsSettingsProps {
  workspace: Workspace
  isOwner: boolean
  currentLocale: Locale
  refreshWorkspaces: () => Promise<void>
}

interface TranslationTreeNode {
  key: string
  kind: 'menu' | 'page' | 'item'
  label: string
  item?: TranslationItem
  children?: TranslationTreeNode[]
}

type RestoreScope = Pick<TranslationItem, 'id' | 'menuKey' | 'pageKey'>

const EMPTY_TRANSLATIONS: UITranslations = {}

export function UITranslationsSettings({
  workspace,
  isOwner,
  currentLocale,
  refreshWorkspaces
}: UITranslationsSettingsProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [inventory, setInventory] = useState<TranslationItem[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [savedOverrides, setSavedOverrides] = useState<UITranslations>(EMPTY_TRANSLATIONS)
  const [draftOverrides, setDraftOverrides] = useState<UITranslations>(EMPTY_TRANSLATIONS)
  const [search, setSearch] = useState('')
  const [expandedKeys, setExpandedKeys] = useState<Key[]>([])
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    let active = true
    setLoading(true)
    setLoadError(false)
    loadStaticCatalogInventory()
      .then((items) => {
        if (!active) return
        setInventory(items)
      })
      .catch(() => {
        if (!active) return
        setLoadError(true)
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [])

  const savedFingerprint = JSON.stringify(workspace.settings.ui_translations ?? EMPTY_TRANSLATIONS)
  useEffect(() => {
    if (inventory.length === 0) return
    const nextSaved = sanitizeSavedOverrides(
      workspace.settings.ui_translations ?? EMPTY_TRANSLATIONS,
      inventory
    )
    setSavedOverrides(nextSaved)
    setDraftOverrides(cloneOverrides(nextSaved))
  }, [inventory, savedFingerprint, workspace.id]) // eslint-disable-line react-hooks/exhaustive-deps

  const localeOrder = useMemo(() => orderLocales(currentLocale), [currentLocale])
  const hierarchyLabels = useMemo<Record<string, string>>(
    () => ({
      Navigation: t`Navigation`,
      Settings: t`Settings`,
      Workspace: t`Workspace`,
      Integrations: t`Integrations`,
      Webhooks: t`Webhooks`,
      'Web Analytics': t`Web Analytics`,
      Dashboard: t`Dashboard`,
      Analytics: t`Analytics`,
      'File Manager': t`File Manager`,
      Logs: t`Logs`,
      Automations: t`Automations`,
      'Transactional Notifications': t`Transactional Notifications`,
      Templates: t`Templates`,
      Lists: t`Lists`,
      Contacts: t`Contacts`,
      Segments: t`Segments`,
      Broadcasts: t`Broadcasts`,
      Blog: t`Blog`,
      Shared: t`Shared`,
      Other: t`Other`
    }),
    [t]
  )
  const dirty = useMemo(
    () => serializeOverrides(draftOverrides) !== serializeOverrides(savedOverrides),
    [draftOverrides, savedOverrides]
  )
  const invalidCells = useMemo(() => findInvalidCells(draftOverrides), [draftOverrides])
  const overrideCount = countOverrides(draftOverrides)
  const normalizedSearch = search.trim().toLocaleLowerCase()
  const visibleItems = useMemo(
    () =>
      normalizedSearch
        ? inventory.filter((item) =>
            [
              hierarchyLabels[item.menuKey] ?? item.menuKey,
              hierarchyLabels[item.pageKey] ?? item.pageKey,
              ...localeOrder.map((locale) => effectiveValue(item, locale, draftOverrides))
            ].some((value) => value.toLocaleLowerCase().includes(normalizedSearch))
          )
        : inventory,
    [draftOverrides, hierarchyLabels, inventory, localeOrder, normalizedSearch]
  )
  const tree = useMemo(() => buildTree(visibleItems), [visibleItems])
  const searchExpandedKeys = useMemo(
    () => (normalizedSearch ? expandableKeys(tree) : expandedKeys),
    [expandedKeys, normalizedSearch, tree]
  )

  const updateCell = useCallback((item: TranslationItem, locale: Locale, value: string) => {
    setDraftOverrides((current) => {
      if (value === item.values[locale]) return removeOverrides(current, [item.id], locale)
      return setOverride(current, locale, item.id, value)
    })
  }, [])

  const restoreScope = useCallback((scope: RestoreScope, kind: RestoreScopeKind) => {
    const ids = inventory
      .filter((item) => scopeIncludesItem(scope, kind, item))
      .map((item) => item.id)
    setDraftOverrides((current) => removeOverrides(current, ids))
  }, [inventory])

  const handleSave = async () => {
    if (invalidCells.size > 0 || !dirty) return
    const payload = normalizeOverrides(draftOverrides)
    setSaving(true)
    try {
      await workspaceService.setUITranslations({
        workspace_id: workspace.id,
        ui_translations: payload
      })
      setSavedOverrides(cloneOverrides(payload))
      setDraftOverrides(cloneOverrides(payload))
    } catch (error) {
      console.error(error)
      message.error(t`Failed to save UI translations`)
      setSaving(false)
      return
    }

    try {
      await refreshWorkspaces()
      message.success(t`UI translations saved`)
    } catch (error) {
      console.error(error)
      message.warning(t`Translations were saved, but the workspace could not be refreshed`)
    } finally {
      setSaving(false)
    }
  }

  const columns = useMemo<ColumnsType<TranslationTreeNode>>(
    () => [
      {
        title: t`Item`,
        key: 'item',
        fixed: 'left',
        width: 260,
        onHeaderCell: () => ({ className: 'translations-fixed-col' }),
        onCell: () => ({ className: 'translations-fixed-col' }),
        render: (_, record) => {
          const displayLabel = record.item
            ? record.label
            : (hierarchyLabels[record.label] ?? record.label)
          const restoredCount = record.item
            ? countItemOverrides(draftOverrides, record.item.id)
            : countScopedOverrides(draftOverrides, inventory, record)
          const restoreLabel = record.item
            ? t`Restore item ${displayLabel}`
            : record.kind === 'page'
              ? t`Restore page ${displayLabel}`
              : t`Restore menu ${displayLabel}`
          return (
            <div className="flex min-w-0 items-center justify-between gap-2">
              <span className={record.item ? 'truncate text-sm' : 'font-medium'}>
                {displayLabel}
              </span>
              <Button
                type="link"
                size="small"
                aria-label={restoreLabel}
                disabled={restoredCount === 0}
                onClick={(event) => {
                  event.stopPropagation()
                  if (record.item) restoreScope(record.item, 'item')
                  else restoreScope(recordScope(record), record.kind)
                }}
              >
                {t`Restore`}
              </Button>
            </div>
          )
        }
      },
      ...localeOrder.map((locale, index) => ({
        title: (
          <Space size={6}>
            <span>{localeNames[locale]}</span>
            {index === 0 ? <Badge status="processing" text={t`Current`} /> : null}
          </Space>
        ),
        key: locale,
        width: 290,
        fixed: index === 0 ? ('left' as const) : undefined,
        onHeaderCell: () => ({
          className: index === 0 ? 'translations-fixed-col' : ''
        }),
        onCell: () => ({
          className: index === 0 ? 'translations-fixed-col' : ''
        }),
        render: (_: unknown, record: TranslationTreeNode) => {
          if (!record.item) return null
          const item = record.item
          const overridden = hasOverride(draftOverrides, locale, item.id)
          const errorKey = cellKey(locale, item.id)
          const invalid = invalidCells.has(errorKey)
          const errorMessage = t`Enter a translation or use Restore to inherit the default`
          return (
            <div className="min-w-64">
              <Input
                value={effectiveValue(item, locale, draftOverrides)}
                aria-label={t`Translation for ${item.source} in ${localeNames[locale]}`}
                aria-describedby={invalid ? `${errorKey}-error` : undefined}
                aria-invalid={invalid}
                status={invalid ? 'error' : undefined}
                onChange={(event) => updateCell(item, locale, event.target.value)}
              />
              <div className="mt-1 flex min-h-6 items-start justify-between gap-2">
                <Badge
                  status={overridden ? 'processing' : 'default'}
                  text={overridden ? t`Override` : t`Default`}
                />
                <Button
                  type="link"
                  size="small"
                  disabled={!overridden}
                  aria-label={t`Restore ${item.source} in ${localeNames[locale]}`}
                  onClick={() =>
                    setDraftOverrides((current) => removeOverrides(current, [item.id], locale))
                  }
                >
                  {t`Restore`}
                </Button>
              </div>
              {invalid ? (
                <div
                  id={`${errorKey}-error`}
                  role="alert"
                  aria-label={errorMessage}
                  className="mt-1 text-xs text-red-600"
                >
                  {errorMessage}
                </div>
              ) : null}
            </div>
          )
        }
      }))
    ],
    [draftOverrides, hierarchyLabels, invalidCells, inventory, localeOrder, restoreScope, t, updateCell]
  )

  if (!isOwner) {
    return (
      <div>
        <SettingsSectionHeader
          title={t`Languages`}
          description={t`Customize the static interface text used throughout this workspace.`}
          className="!mb-6"
        />
        <Alert
          type="info"
          showIcon
          title={t`Only workspace owners can manage UI translations.`}
        />
      </div>
    )
  }

  return (
    <div>
      <SettingsSectionHeader
        title={t`Languages`}
        description={t`Customize the static interface text used throughout this workspace.`}
        className="!mb-6"
      />

      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <Input
          type="search"
          allowClear
          prefix={<SearchOutlined aria-hidden />}
          value={search}
          aria-label={t`Search translations`}
          placeholder={t`Search translations`}
          onChange={(event) => setSearch(event.target.value)}
          className="max-w-md"
        />
        <Popconfirm
          title={t`Restore all translations?`}
          description={t`All workspace overrides will fall back to the bundled defaults.`}
          okText={t`Restore all overrides`}
          cancelText={t`Cancel`}
          okButtonProps={{ danger: true }}
          onConfirm={() => setDraftOverrides(EMPTY_TRANSLATIONS)}
        >
          <Button danger disabled={overrideCount === 0}>
            {t`Restore all`}
          </Button>
        </Popconfirm>
      </div>

      {loadError ? (
        <Alert
          type="error"
          showIcon
          title={t`UI translations could not be loaded`}
          description={t`Reload this page to try again.`}
        />
      ) : (
        <div role="region" aria-label={t`Workspace UI translations`}>
          <Table<TranslationTreeNode>
            rowKey="key"
            columns={columns}
            dataSource={tree}
            loading={loading}
            pagination={false}
            size="small"
            scroll={{ x: 'max-content', y: 'calc(100vh - 360px)' }}
            expandable={{
              expandedRowKeys: searchExpandedKeys,
              onExpandedRowsChange: (keys) => setExpandedKeys([...keys]),
              rowExpandable: (record) => Boolean(record.children?.length),
              expandRowByClick: true
            }}
            locale={{ emptyText: t`No translations found` }}
          />
        </div>
      )}

      <SettingsSaveBar
        dirty={dirty}
        saving={saving}
        onSave={handleSave}
        onDiscard={() => setDraftOverrides(cloneOverrides(savedOverrides))}
        leaveWarning={t`Your UI translation changes have not been saved.`}
      />
    </div>
  )
}

type RestoreScopeKind = TranslationTreeNode['kind']

function buildTree(items: TranslationItem[]): TranslationTreeNode[] {
  const menus = new Map<string, Map<string, TranslationItem[]>>()
  for (const item of items) {
    let pages = menus.get(item.menuKey)
    if (!pages) {
      pages = new Map()
      menus.set(item.menuKey, pages)
    }
    const pageItems = pages.get(item.pageKey)
    if (pageItems) pageItems.push(item)
    else pages.set(item.pageKey, [item])
  }

  return [...menus].map(([menuKey, pages]) => ({
    key: `menu:${menuKey}`,
    kind: 'menu',
    label: menuKey,
    children: [...pages].map(([pageKey, pageItems]) => ({
      key: `page:${menuKey}:${pageKey}`,
      kind: 'page',
      label: pageKey,
      children: pageItems.map((item) => ({
        key: `item:${item.id}`,
        kind: 'item',
        label: item.source,
        item
      }))
    }))
  }))
}

function expandableKeys(nodes: TranslationTreeNode[]): Key[] {
  return nodes.flatMap((node) => [
    ...(node.children?.length ? [node.key] : []),
    ...expandableKeys(node.children ?? [])
  ])
}

function recordScope(record: TranslationTreeNode): RestoreScope {
  const item = firstItem(record)
  return { id: item?.id ?? '', menuKey: item?.menuKey ?? '', pageKey: item?.pageKey ?? '' }
}

function firstItem(record: TranslationTreeNode): TranslationItem | undefined {
  if (record.item) return record.item
  for (const child of record.children ?? []) {
    const item = firstItem(child)
    if (item) return item
  }
  return undefined
}

function scopeIncludesItem(
  scope: RestoreScope,
  kind: RestoreScopeKind,
  item: TranslationItem
): boolean {
  if (kind === 'item') return item.id === scope.id
  if (kind === 'page') return item.menuKey === scope.menuKey && item.pageKey === scope.pageKey
  return item.menuKey === scope.menuKey
}

function countScopedOverrides(
  overrides: UITranslations,
  inventory: TranslationItem[],
  record: TranslationTreeNode
): number {
  const scope = recordScope(record)
  const ids = new Set(
    inventory
      .filter((item) => scopeIncludesItem(scope, record.kind, item))
      .map((item) => item.id)
  )
  return Object.values(overrides).reduce(
    (count, messages) =>
      count + Object.keys(messages).filter((messageId) => ids.has(messageId)).length,
    0
  )
}

function countItemOverrides(overrides: UITranslations, messageId: string): number {
  return Object.values(overrides).filter((messages) =>
    Object.prototype.hasOwnProperty.call(messages, messageId)
  ).length
}

function sanitizeSavedOverrides(
  overrides: UITranslations,
  inventory: TranslationItem[]
): UITranslations {
  const itemsById = new Map(inventory.map((item) => [item.id, item]))
  const result: UITranslations = {}
  for (const locale of locales) {
    const messages = overrides[locale]
    if (!messages) continue
    for (const [messageId, value] of Object.entries(messages)) {
      const item = itemsById.get(messageId)
      if (!item || value === item.values[locale]) continue
      if (!result[locale]) result[locale] = {}
      result[locale][messageId] = value
    }
  }
  return normalizeOverrides(result)
}

function setOverride(
  overrides: UITranslations,
  locale: Locale,
  messageId: string,
  value: string
): UITranslations {
  return {
    ...overrides,
    [locale]: { ...overrides[locale], [messageId]: value }
  }
}

function removeOverrides(
  overrides: UITranslations,
  messageIds: string[],
  onlyLocale?: Locale
): UITranslations {
  const ids = new Set(messageIds)
  const result: UITranslations = {}
  for (const [locale, messages] of Object.entries(overrides)) {
    if (onlyLocale && locale !== onlyLocale) {
      result[locale] = { ...messages }
      continue
    }
    const remaining = Object.fromEntries(
      Object.entries(messages).filter(([messageId]) => !ids.has(messageId))
    )
    if (Object.keys(remaining).length > 0) result[locale] = remaining
  }
  return normalizeOverrides(result)
}

function effectiveValue(
  item: TranslationItem,
  locale: Locale,
  overrides: UITranslations
): string {
  return overrides[locale]?.[item.id] ?? item.values[locale]
}

function hasOverride(overrides: UITranslations, locale: Locale, messageId: string): boolean {
  return Object.prototype.hasOwnProperty.call(overrides[locale] ?? {}, messageId)
}

function findInvalidCells(overrides: UITranslations): Set<string> {
  const invalid = new Set<string>()
  for (const [locale, messages] of Object.entries(overrides)) {
    for (const [messageId, value] of Object.entries(messages)) {
      if (value.trim().length === 0) invalid.add(cellKey(locale, messageId))
    }
  }
  return invalid
}

function cellKey(locale: string, messageId: string): string {
  return `translation-${encodeURIComponent(locale)}-${encodeURIComponent(messageId)}`
}

function countOverrides(overrides: UITranslations): number {
  return Object.values(overrides).reduce((total, messages) => total + Object.keys(messages).length, 0)
}

function cloneOverrides(overrides: UITranslations): UITranslations {
  return Object.fromEntries(
    Object.entries(overrides).map(([locale, messages]) => [locale, { ...messages }])
  )
}

function normalizeOverrides(overrides: UITranslations): UITranslations {
  const result: UITranslations = {}
  for (const locale of locales) {
    const messages = overrides[locale]
    if (!messages || Object.keys(messages).length === 0) continue
    result[locale] = Object.fromEntries(
      Object.entries(messages).sort(([left], [right]) => left.localeCompare(right))
    )
  }
  return result
}

function serializeOverrides(overrides: UITranslations): string {
  return JSON.stringify(normalizeOverrides(overrides))
}
