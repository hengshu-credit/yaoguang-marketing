import { useEffect, useState } from 'react'
import { Alert, Button, Input, Popconfirm, Space, Table, Tabs, Tag, Typography } from 'antd'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { businessApi, type BusinessDefinition, type BusinessDefinitionKind } from '../../services/api/business'
import { WorkspacePageTitle } from '../navigation/WorkspacePageTitle'
import { BusinessDefinitionEditor } from './BusinessDefinitionEditor'
import { useBusinessCatalogue } from './useBusinessCatalogue'
import { businessDefinitionDisplayName } from './builtin-event-labels'
import { CustomerAttributesSettings } from './CustomerAttributesSettings'
import { BusinessConfigurationGuide } from './BusinessConfigurationGuide'
import type { Workspace } from '../../services/api/types'
import { EventFieldsExtensionDrawer } from './EventFieldsExtensionDrawer'
import { builtinEventOriginLabel, getBuiltinEventReference } from './builtin-event-reference'

interface Props { workspaceId: string; canManage: boolean; initialKind?: BusinessDefinitionKind; onWorkspaceUpdate?: (workspace: Workspace) => void }

export function BusinessConfigurationSettings(props: Props) {
  return <ConfigurationContent key={`${props.workspaceId}:${props.initialKind ?? 'event'}`} {...props} />
}

function ConfigurationContent({ workspaceId, canManage, initialKind = 'event', onWorkspaceUpdate }: Props) {
  const { t, i18n } = useLingui()
  const [kind, setKind] = useState<BusinessDefinitionKind>(initialKind)
  const [items, setItems] = useState<BusinessDefinition[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [refresh, setRefresh] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [editor, setEditor] = useState<BusinessDefinition>()
  const [archiving, setArchiving] = useState<string>()
  const [extending, setExtending] = useState<string[]>()
  const catalogue = useBusinessCatalogue(workspaceId, refresh)
  const labels: Partial<Record<BusinessDefinitionKind, string>> = { event: t`Events`, customer_attribute: t`Customer attributes`, object_type: t`Business object types`, object_relation: t`Object relations`, event_mapping: t`Event field mappings` }
  const statuses = { draft: t`Draft`, active: t`Active`, archived: t`Archived` }

  useEffect(() => {
    let current = true
    setItems([]); setTotal(0); setLoading(true); setError('')
    businessApi.list(workspaceId, kind, { search, limit: 20, offset: (page - 1) * 20 }).then((result) => {
      if (current) { setItems(result.definitions ?? []); setTotal(result.total) }
    }).catch((cause) => { if (current) setError(cause instanceof Error ? cause.message : String(cause)) }).finally(() => { if (current) setLoading(false) })
    return () => { current = false }
  }, [workspaceId, kind, page, search, refresh])

  const create = () => {
    const settings: Record<string, unknown> = kind === 'event' ? { subject_type: 'customer', expiry_mode: 'never', unmatched_customer: 'pending', browser_enabled: false, authoritative: true } : kind === 'object_relation' ? { cardinality: 'one_to_many' } : kind === 'event_mapping' ? { fields: { report_id: '$.report_id', customer_attributes: '$.customer_attributes', event_code: '$.event_code', data: '$.data', triggered_at: '$.triggered_at', expires_at: '$.expires_at' }, attribute_mappings: [] } : kind === 'customer_attribute' ? { identity: false, sensitive: false, writable: true, normalization: 'trim' } : { identity_fields: [] }
    setEditor({ kind, key: '', name: '', schema: kind === 'customer_attribute' ? { type: 'string' } : { type: 'object', properties: {}, required: [] }, settings, status: 'draft' })
  }
  const archive = async (definition: BusinessDefinition) => {
    if (!canManage) return
    setArchiving(definition.key); setError('')
    try { await businessApi.archive(workspaceId, definition); setRefresh((value) => value + 1) }
    catch (cause) { setError(cause instanceof Error ? cause.message : t`Failed to archive definition`) }
    finally { setArchiving(undefined) }
  }

  return <div className="space-y-5">
    <div><WorkspacePageTitle>{t`Customer and event configuration`}</WorkspacePageTitle><Typography.Paragraph type="secondary">{t`Define customer attributes, business objects, event data and external field mappings for this workspace.`}</Typography.Paragraph></div>
    {error && <Alert type="error" showIcon title={error} action={<Button size="small" onClick={() => setRefresh((value) => value + 1)}>{t`Retry`}</Button>} />}
    {catalogue.error && <Alert type="error" showIcon title={t`Failed to load related definitions`} description={catalogue.error.message} />}
    <Tabs activeKey={kind} onChange={(next) => { setKind(next as BusinessDefinitionKind); setPage(1); setSearch('') }} items={Object.entries(labels).map(([key, label]) => ({ key, label }))} />
    <BusinessConfigurationGuide kind={kind} />
    {kind === 'event' && canManage && <Button disabled={catalogue.loading || !!catalogue.error} onClick={() => setExtending([])}>{t`Add report fields to existing events`}</Button>}
    {kind === 'customer_attribute' ? <CustomerAttributesSettings workspaceId={workspaceId} definitions={catalogue.definitions} canManage={canManage} onEdit={setEditor} onCreate={create} onWorkspaceUpdate={onWorkspaceUpdate} /> : <>
    <div className="flex flex-wrap items-center justify-between gap-3">
      <Input.Search key={kind} aria-label={t`Search definitions`} placeholder={t`Search by name or key`} allowClear style={{ maxWidth: 360 }} onSearch={(value) => { setSearch(value); setPage(1) }} />
      <Space><Button icon={<ReloadOutlined />} aria-label={t`Refresh definitions`} onClick={() => setRefresh((value) => value + 1)} />{canManage && <Button type="primary" aria-label={t`New definition`} icon={<PlusOutlined aria-hidden />} onClick={create}>{t`New definition`}</Button>}</Space>
    </div>
    <Table<BusinessDefinition> rowKey="id" loading={loading} dataSource={items} scroll={{ x: 720 }} pagination={{ current: page, pageSize: 20, total, showSizeChanger: false, onChange: setPage }} columns={[
      { title: t`Name`, key: 'name', render: (_, definition) => <div><Button type="link" style={{ padding: 0 }} onClick={() => setEditor(definition)}>{businessDefinitionDisplayName(definition, i18n)}</Button><div className="text-xs text-gray-500">{definition.description}</div></div> },
      { title: t`Key`, dataIndex: 'key', render: (key: string) => <code>{key}</code> },
      ...(kind === 'object_type' ? [{ title: t`Object identity fields`, render: (_: unknown, definition: BusinessDefinition) => Array.isArray(definition.settings.identity_fields) ? definition.settings.identity_fields.join(', ') : '—' }] : kind === 'object_relation' ? [{ title: t`Object relations`, render: (_: unknown, definition: BusinessDefinition) => <div>{String(definition.settings.from_type ?? '')} → {String(definition.settings.to_type ?? '')}<div className="text-xs text-gray-500">{definition.settings.cardinality === 'one_to_one' ? t`One to one` : definition.settings.cardinality === 'one_to_many' ? t`One to many` : t`Many to many`}</div></div> }] : []),
      { title: t`Status`, dataIndex: 'status', render: (status: BusinessDefinition['status']) => <Tag color={status === 'active' ? 'green' : undefined}>{statuses[status]}</Tag> },
      { title: t`Definition origin`, key: 'origin', render: (_, definition) => definition.builtin || definition.settings.builtin === true ? t`Built-in` : t`Custom` },
      ...(kind === 'event' ? [{ title: t`Trigger source`, key: 'trigger_source', render: (_: unknown, definition: BusinessDefinition) => {
        if (!definition.builtin && definition.settings.builtin !== true) return t`Business application report`
        const reference = getBuiltinEventReference(definition.key, i18n)
        return reference ? <Tag>{builtinEventOriginLabel(reference.origin, i18n)}</Tag> : t`Unknown producer`
      } }] : []),
      ...(canManage ? [{ title: t`Actions`, key: 'actions', render: (_: unknown, definition: BusinessDefinition) => <Space><Button type="link" onClick={() => setEditor(definition)}>{t`View`}</Button>{kind === 'event' && definition.status !== 'archived' && <Button type="link" onClick={() => setExtending([definition.key])}>{t`Add report fields`}</Button>}{definition.status !== 'archived' && !definition.builtin && definition.settings.builtin !== true && <Popconfirm title={t`Archive this definition?`} description={t`Historical reports keep their recorded definition. Archived definitions cannot accept new use.`} onConfirm={() => archive(definition)} okText={t`Archive`} cancelText={t`Cancel`}><Button type="link" danger loading={archiving === definition.key}>{t`Archive`}</Button></Popconfirm>}</Space> }] : [])
    ]} />
    </>}
    {extending && <EventFieldsExtensionDrawer workspaceId={workspaceId} events={catalogue.definitions.filter(item => item.kind === 'event' && item.status !== 'archived').map(item => ({ ...item, name: businessDefinitionDisplayName(item, i18n) }))} initialKeys={extending} onClose={() => setExtending(undefined)} onSaved={() => { setExtending(undefined); setRefresh(value => value + 1) }} />}
    {editor && <BusinessDefinitionEditor key={`${editor.kind}:${editor.key}:${editor.revision ?? 0}`} workspaceId={workspaceId} definition={editor} catalogue={catalogue.definitions} readOnly={!canManage} onClose={() => setEditor(undefined)} onSaved={() => { setEditor(undefined); setRefresh((value) => value + 1) }} />}
  </div>
}
