import { useMemo, useState } from 'react'
import { Avatar, Button, Collapse, Divider, Input, Popconfirm, Select, Space, Tag, Typography } from 'antd'
import { CheckOutlined, CloseOutlined, DeleteOutlined, EditOutlined, PlusOutlined, UserOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import type { Customer, CustomerUpdatePatch } from '../../services/api/customer'

interface CustomerProfilePanelProps {
  customer: Customer
  canWrite: boolean
  saving: boolean
  onUpdate: (patch: CustomerUpdatePatch) => Promise<void>
}

function EditableRow({
  label,
  value,
  canWrite,
  saving,
  onSave,
  onRemove,
  horizontal = false
}: {
  label: string
  value?: string
  canWrite: boolean
  saving: boolean
  onSave: (value: string) => Promise<void>
  onRemove?: () => Promise<void>
  horizontal?: boolean
}) {
  const { t } = useLingui()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(value ?? '')

  if (editing) {
    return (
      <div className={horizontal ? 'grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] items-start gap-3 py-2' : 'py-2'}>
        <Typography.Text type="secondary" className={horizontal ? 'min-w-0 pt-2 text-left text-xs' : 'block text-xs'}>{label}</Typography.Text>
        <Space.Compact className={horizontal ? 'col-span-2 w-full' : 'mt-1 w-full'}>
          <Input value={draft} onChange={(event) => setDraft(event.target.value)} autoFocus />
          <Button
            aria-label={t`Save ${label}`}
            icon={<CheckOutlined />}
            loading={saving}
            onClick={async () => {
              await onSave(draft.trim())
              setEditing(false)
            }}
          />
          <Button aria-label={t`Cancel`} icon={<CloseOutlined />} onClick={() => setEditing(false)} />
        </Space.Compact>
      </div>
    )
  }

  return (
    <div className={horizontal ? 'group grid min-h-12 grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] items-center gap-3 border-b border-gray-100 py-2' : 'group flex min-h-12 items-center justify-between gap-3 border-b border-gray-100 py-2'}>
      {horizontal ? (
        <>
          <Typography.Text type="secondary" className="min-w-0 break-words text-left text-xs">{label}</Typography.Text>
          <Typography.Text className="min-w-0 break-words text-right">{value || '—'}</Typography.Text>
        </>
      ) : (
        <div className="min-w-0">
          <Typography.Text type="secondary" className="block text-xs">{label}</Typography.Text>
          <Typography.Text className="break-words">{value || '—'}</Typography.Text>
        </div>
      )}
      {canWrite ? <Space size={0}>
        <Button
          type="text"
          size="small"
          aria-label={t`Edit ${label}`}
          icon={<EditOutlined />}
          onClick={() => {
            setDraft(value ?? '')
            setEditing(true)
          }}
        />
        {onRemove ? (
          <Popconfirm title={t`Remove this profile attribute?`} onConfirm={onRemove}>
            <Button type="text" danger size="small" aria-label={t`Remove ${label}`} icon={<DeleteOutlined />} />
          </Popconfirm>
        ) : null}
      </Space> : null}
    </div>
  )
}

function parseAttributeValue(raw: string): unknown {
  const trimmed = raw.trim()
  if (!trimmed) return ''
  try {
    return JSON.parse(trimmed)
  } catch {
    return trimmed
  }
}

export function CustomerProfilePanel({ customer, canWrite, saving, onUpdate }: CustomerProfilePanelProps) {
  const { t } = useLingui()
  const [editingTags, setEditingTags] = useState(false)
  const [tags, setTags] = useState<string[]>(customer.tags ?? [])
  const [addingAttribute, setAddingAttribute] = useState(false)
  const [attributeKey, setAttributeKey] = useState('')
  const [attributeValue, setAttributeValue] = useState('')
  const attributes = customer.profile?.attributes ?? {}
  const displayName = customer.external_user_id || customer.identities?.[0]?.display_hint || customer.customer_no
  const identityGroups = useMemo(() => customer.identities ?? [], [customer.identities])
  const primaryIdentity = identityGroups.find((identity) => identity.primary) ?? identityGroups[0]

  return (
    <aside data-testid="customer-profile-panel" className="h-full bg-slate-50 p-5 md:w-[380px] md:min-w-[380px] md:overflow-y-auto">
      <div className="mb-5">
        <Space align="start" size="middle">
          <Avatar size={52} icon={<UserOutlined />} className="bg-blue-600" />
          <div className="min-w-0">
            <Typography.Text type="secondary" className="text-xs uppercase tracking-wide">{t`Customer profile`}</Typography.Text>
            <Typography.Title level={3} className="mb-0 mt-1 break-words">{displayName}</Typography.Title>
            {primaryIdentity ? <Typography.Text type="secondary" className="block break-words">{primaryIdentity.display_hint}</Typography.Text> : null}
          </div>
        </Space>
        <Space wrap className="mt-3">
          <Tag color={customer.profile?.status === 'active' ? 'green' : 'default'}>{customer.profile?.status || t`No status`}</Tag>
          <Typography.Text type="secondary">v{customer.version}</Typography.Text>
        </Space>
      </div>

      <EditableRow label={t`External user ID`} value={customer.external_user_id} canWrite={canWrite} saving={saving} onSave={(value) => onUpdate({ external_user_id: value })} />
      <EditableRow label={t`Status`} value={customer.profile?.status} canWrite={canWrite} saving={saving} onSave={(value) => onUpdate({ profile: { status: value } })} />
      <EditableRow label={t`Language`} value={customer.profile?.language} canWrite={canWrite} saving={saving} onSave={(value) => onUpdate({ profile: { language: value } })} />
      <EditableRow label={t`Timezone`} value={customer.profile?.timezone} canWrite={canWrite} saving={saving} onSave={(value) => onUpdate({ profile: { timezone: value } })} />

      <Divider className="my-4" />
      <div className="mb-4">
        <div className="mb-2 flex items-center justify-between">
          <Typography.Text strong>{t`Tags`}</Typography.Text>
          {canWrite && !editingTags ? <Button type="text" size="small" icon={<EditOutlined />} aria-label={t`Edit tags`} onClick={() => setEditingTags(true)} /> : null}
        </div>
        {editingTags ? (
          <Space orientation="vertical" className="w-full">
            <Select mode="tags" value={tags} onChange={setTags} tokenSeparators={[',']} className="w-full" open={false} />
            <Space>
              <Button size="small" type="primary" loading={saving} onClick={async () => { await onUpdate({ tags }); setEditingTags(false) }}>{t`Save`}</Button>
              <Button size="small" onClick={() => { setTags(customer.tags ?? []); setEditingTags(false) }}>{t`Cancel`}</Button>
            </Space>
          </Space>
        ) : customer.tags?.length ? <Space wrap>{customer.tags.map((tag) => <Tag key={tag}>{tag}</Tag>)}</Space> : <Typography.Text type="secondary">{t`No tags`}</Typography.Text>}
      </div>

      <Divider className="my-4" />
      <div className="mb-4">
        <div className="mb-1 flex items-center justify-between">
          <Typography.Text strong>{t`Profile attributes`}</Typography.Text>
          {canWrite ? <Button type="text" size="small" icon={<PlusOutlined />} aria-label={t`Add profile attribute`} onClick={() => setAddingAttribute(true)} /> : null}
        </div>
        {Object.entries(attributes).map(([key, value]) => (
          <EditableRow
            key={key}
            label={key}
            value={typeof value === 'string' ? value : JSON.stringify(value)}
            canWrite={canWrite}
            saving={saving}
            onSave={(next) => onUpdate({ profile: { attributes: { merge: { [key]: parseAttributeValue(next) } } } })}
            onRemove={() => onUpdate({ profile: { attributes: { unset: [key] } } })}
            horizontal
          />
        ))}
        {Object.keys(attributes).length === 0 && !addingAttribute ? <Typography.Text type="secondary">{t`No profile attributes`}</Typography.Text> : null}
        {addingAttribute ? (
          <Space orientation="vertical" className="mt-2 w-full">
            <Input placeholder={t`Attribute name`} value={attributeKey} onChange={(event) => setAttributeKey(event.target.value)} />
            <Input.TextArea placeholder={t`Value (text or JSON)`} value={attributeValue} onChange={(event) => setAttributeValue(event.target.value)} autoSize />
            <Space>
              <Button size="small" type="primary" disabled={!attributeKey.trim()} loading={saving} onClick={async () => { await onUpdate({ profile: { attributes: { merge: { [attributeKey.trim()]: parseAttributeValue(attributeValue) } } } }); setAttributeKey(''); setAttributeValue(''); setAddingAttribute(false) }}>{t`Add`}</Button>
              <Button size="small" onClick={() => setAddingAttribute(false)}>{t`Cancel`}</Button>
            </Space>
          </Space>
        ) : null}
      </div>

      <Divider className="my-4" />
      <Typography.Text strong>{t`Identity aliases`}</Typography.Text>
      <div className="mt-2">
        {identityGroups.length ? identityGroups.map((identity) => (
          <div key={identity.id} className="mb-2 rounded-md bg-white p-2">
            <Typography.Text className="block">{identity.display_hint}</Typography.Text>
            <Typography.Text type="secondary" className="text-xs">{identity.type}{identity.primary ? ` · ${t`Primary`}` : ''}</Typography.Text>
          </div>
        )) : <Typography.Text type="secondary">{t`No identity aliases`}</Typography.Text>}
      </div>

      <Collapse ghost size="small" className="mt-4" items={[{
        key: 'system',
        label: t`System identifiers`,
        children: <Space orientation="vertical" size="small">
          <Typography.Text copyable>{customer.customer_no}</Typography.Text>
          <Typography.Text copyable>{customer.customer_id}</Typography.Text>
          <Typography.Text type="secondary">{t`Created`}: {customer.created_at}</Typography.Text>
          <Typography.Text type="secondary">{t`Updated`}: {customer.updated_at}</Typography.Text>
        </Space>
      }]} />
    </aside>
  )
}
