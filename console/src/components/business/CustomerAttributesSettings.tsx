import { useEffect, useState } from 'react'
import { Alert, Button, Checkbox, Input, Modal, Space, Table, Tag, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { workspaceService } from '../../services/api/workspace'
import type { Workspace } from '../../services/api/types'
import { getCustomFieldLabel } from '../../hooks/useCustomFieldLabel'
import type { BusinessDefinition } from '../../services/api/business'

interface Props { workspaceId: string; definitions: BusinessDefinition[]; canManage: boolean; onEdit: (definition: BusinessDefinition) => void; onCreate: () => void; onWorkspaceUpdate?: (workspace: Workspace) => void }

export function CustomerAttributesSettings({ workspaceId, definitions, canManage, onEdit, onCreate, onWorkspaceUpdate }: Props) {
  const { t } = useLingui()
  const [workspace, setWorkspace] = useState<Workspace>()
  const [search, setSearch] = useState('')
  const [showUnused, setShowUnused] = useState(false)
  const [editing, setEditing] = useState<string>()
  const [name, setName] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  useEffect(() => { let current = true; workspaceService.get(workspaceId).then(result => { if (current) setWorkspace(result.workspace) }).catch(cause => { if (current) setError(String(cause)) }); return () => { current = false } }, [workspaceId])
  const predefined = ['string', 'number', 'datetime', 'json'].flatMap(type => Array.from({ length: 5 }, (_, index) => ({ key: `custom_${type}_${index + 1}`, type })))
  const rows = [
    ...predefined.filter(field => showUnused || !!search.trim() || workspace?.settings.custom_field_labels?.[field.key] || definitions.some(definition => definition.kind === 'customer_attribute' && definition.key === field.key)).map(field => ({ ...field, name: getCustomFieldLabel(field.key, workspace), predefined: true, definition: definitions.find(definition => definition.kind === 'customer_attribute' && definition.key === field.key) })),
    ...definitions.filter(definition => definition.kind === 'customer_attribute' && !predefined.some(field => field.key === definition.key)).map(definition => ({ key: definition.key, type: String(definition.schema.type ?? 'string'), name: definition.name, predefined: false, definition }))
  ].filter(field => `${field.name} ${field.key}`.toLowerCase().includes(search.toLowerCase()))
  const typeLabels: Record<string, string> = { string: t`Text`, number: t`Number`, integer: t`Integer`, boolean: t`Boolean`, datetime: t`Date and time`, json: 'JSON', array: t`Array`, object: t`Object` }
  const save = async () => {
    if (!workspace || !editing || !canManage) return
    setSaving(true); setError('')
    try {
      // Refresh before merging labels so other configured fields are preserved.
      const current = (await workspaceService.get(workspaceId)).workspace
      const labels = { ...current.settings.custom_field_labels }
      if (name.trim()) labels[editing] = name.trim(); else delete labels[editing]
      await workspaceService.setCustomFieldLabels({ workspace_id: workspaceId, custom_field_labels: labels })
      const updated = (await workspaceService.get(workspaceId)).workspace
      setWorkspace(updated); onWorkspaceUpdate?.(updated); setEditing(undefined)
    } catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)) }
    finally { setSaving(false) }
  }
  return <div className="space-y-4">
    <Alert type="info" showIcon title={t`One place for customer attributes`} description={t`Existing custom fields and new customer attributes are managed here. Renaming a predefined field keeps its key and historical data; new attributes are not limited to predefined slots. Import templates and exports follow this configuration.`} />
    {error && <Alert type="error" showIcon title={error} />}
    <div className="flex flex-wrap items-center justify-between gap-3"><Input.Search style={{ maxWidth: 360 }} placeholder={t`Search by name or key`} value={search} onChange={event => setSearch(event.target.value)} /><Checkbox checked={showUnused} onChange={event => setShowUnused(event.target.checked)}>{t`Show unused predefined fields`}</Checkbox>{canManage && <Button type="primary" onClick={onCreate}>{t`New customer attribute`}</Button>}</div>
    <Table rowKey="key" dataSource={rows} loading={!workspace && !error} scroll={{ x: 720 }} pagination={{ pageSize: 20 }} columns={[
      { title: t`Name`, dataIndex: 'name' }, { title: t`Key`, dataIndex: 'key', render: key => <code>{key}</code> },
      { title: t`Field type`, dataIndex: 'type', render: type => typeLabels[type] || type }, { title: t`Origin`, render: (_, row) => <Tag>{row.predefined ? t`Predefined field` : t`Customer attribute`}</Tag> },
      { title: t`Actions`, render: (_, row) => <Space><Button type="link" disabled={row.predefined && (!canManage || !workspace)} onClick={() => { if (!row.predefined && row.definition) onEdit(row.definition); else { setError(''); setEditing(row.key); setName(workspace?.settings.custom_field_labels?.[row.key] ?? '') } }}>{row.predefined ? t`Edit display name` : t`View`}</Button>{row.predefined && row.definition && <Button type="link" onClick={() => onEdit(row.definition!)}>{t`Configuration`}</Button>}</Space> }
    ]} />
    <Modal open={!!editing} title={t`Edit display name`} onCancel={() => setEditing(undefined)} onOk={() => void save()} confirmLoading={saving} okButtonProps={{ disabled: !canManage }}>
      {error && <Alert type="error" title={error} className="mb-3" />}
      <Typography.Paragraph><code>{editing}</code></Typography.Paragraph>
      <Input aria-label={t`Display name`} value={name} onChange={event => setName(event.target.value)} />
      <Typography.Paragraph type="secondary" className="mt-2">{t`Leave the name empty to restore its default label. The field and its data will not be deleted.`}</Typography.Paragraph>
    </Modal>
  </div>
}
