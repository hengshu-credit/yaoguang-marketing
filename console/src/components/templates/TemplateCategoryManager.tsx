import { useState } from 'react'
import { App, Button, Drawer, Form, Input, InputNumber, Modal, Select, Space, Switch, Table, Tag, Typography } from 'antd'
import { PlusOutlined, SettingOutlined } from '@ant-design/icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import {
  templateCategoriesApi,
  type TemplateCategoryDefinition,
  type TemplateCategoryPurpose
} from '../../services/api/templateCategories'
import { templateCategoryDisplayName } from './templateCategoryLabels'

interface Props {
  workspaceId: string
  canWrite: boolean
}

interface CategoryFormValues {
  id: string
  name: string
  purpose: TemplateCategoryPurpose
  sort_order: number
  is_active: boolean
}

const TemplateCategoryManager: React.FC<Props> = ({ workspaceId, canWrite }) => {
  const { t } = useLingui()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<TemplateCategoryDefinition>()
  const [form] = Form.useForm<CategoryFormValues>()
  const { data, isLoading } = useQuery({
    queryKey: ['template-categories', workspaceId, true],
    queryFn: () => templateCategoriesApi.list(workspaceId, true),
    enabled: open
  })
  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['template-categories', workspaceId] })
  const createMutation = useMutation({
    mutationFn: templateCategoriesApi.create,
    onSuccess: async () => { await invalidate(); setFormOpen(false); message.success(t`Category created`) },
    onError: (error: Error) => message.error(error.message)
  })
  const updateMutation = useMutation({
    mutationFn: templateCategoriesApi.update,
    onSuccess: async () => { await invalidate(); setFormOpen(false); message.success(t`Category updated`) },
    onError: (error: Error) => message.error(error.message)
  })
  const deleteMutation = useMutation({
    mutationFn: ({ id }: { id: string }) => templateCategoriesApi.delete(workspaceId, id),
    onSuccess: async () => { await invalidate(); message.success(t`Category deleted`) },
    onError: (error: Error) => message.error(error.message)
  })
  const systemNames: Record<string, string> = {
    marketing: t`Marketing`, transactional: t`Transactional`, welcome: t`Welcome`, opt_in: t`Opt-in`,
    unsubscribe: t`Unsubscribe`, bounce: t`Bounce`, blocklist: t`Blocklist`, blog: t`Blog`, other: t`Other`
  }
  const startCreate = () => {
    setEditing(undefined)
    form.setFieldsValue({ id: '', name: '', purpose: 'transactional', sort_order: 100, is_active: true })
    setFormOpen(true)
  }
  const startEdit = (category: TemplateCategoryDefinition) => {
    setEditing(category)
    form.setFieldsValue(category)
    setFormOpen(true)
  }
  const save = (values: CategoryFormValues) => {
    if (editing) {
      updateMutation.mutate({ workspace_id: workspaceId, id: editing.id, name: values.name, sort_order: values.sort_order, is_active: values.is_active })
    } else {
      createMutation.mutate({ workspace_id: workspaceId, id: values.id, name: values.name, purpose: values.purpose, sort_order: values.sort_order })
    }
  }

  return <>
    <Button icon={<SettingOutlined />} onClick={() => setOpen(true)}>{t`Manage categories`}</Button>
    <Drawer title={t`Template categories`} open={open} onClose={() => setOpen(false)} size={760}
      extra={<Button type="primary" icon={<PlusOutlined />} disabled={!canWrite} onClick={startCreate}>{t`Add category`}</Button>}>
      <Table rowKey="id" loading={isLoading} pagination={false} dataSource={data?.categories || []} columns={[
        { title: t`Category`, render: (_: unknown, category: TemplateCategoryDefinition) => <Space><Typography.Text strong>{templateCategoryDisplayName(category, systemNames)}</Typography.Text>{category.is_system && <Tag>{t`System`}</Tag>}</Space> },
        { title: t`Purpose`, dataIndex: 'purpose', render: (purpose: TemplateCategoryPurpose) => <Tag color={purpose === 'marketing' ? 'green' : 'blue'}>{purpose === 'marketing' ? t`Marketing` : t`Transactional`}</Tag> },
        { title: t`Templates`, dataIndex: 'usage_count' },
        { title: t`Enabled`, render: (_: unknown, category: TemplateCategoryDefinition) => <Switch checked={category.is_active} disabled={!canWrite} onChange={(is_active) => updateMutation.mutate({ workspace_id: workspaceId, id: category.id, name: category.name, sort_order: category.sort_order, is_active })} /> },
        { title: '', render: (_: unknown, category: TemplateCategoryDefinition) => <Space><Button type="link" disabled={!canWrite} onClick={() => startEdit(category)}>{t`Edit`}</Button><Button type="link" danger disabled={!canWrite || category.is_system || category.usage_count > 0} onClick={() => Modal.confirm({ title: t`Delete category?`, onOk: () => deleteMutation.mutate({ id: category.id }) })}>{t`Delete`}</Button></Space> }
      ]} />
    </Drawer>
    <Modal title={editing ? t`Edit category` : t`Add category`} open={formOpen} onCancel={() => setFormOpen(false)}
      onOk={() => form.submit()} okText={t`Save`} confirmLoading={createMutation.isPending || updateMutation.isPending}>
      <Form form={form} layout="vertical" onFinish={save}>
        <Form.Item name="id" label={t`Category ID`} rules={[{ required: true }, { pattern: /^[a-z0-9]+(?:[_-][a-z0-9]+)*$/, message: t`Use lowercase letters, numbers, underscores or hyphens` }]}><Input disabled={Boolean(editing)} maxLength={20} /></Form.Item>
        <Form.Item name="name" label={t`Name`} rules={[{ required: true }]}><Input maxLength={64} /></Form.Item>
        <Form.Item name="purpose" label={t`Purpose`} rules={[{ required: true }]}><Select disabled={Boolean(editing)} options={[{ value: 'transactional', label: t`Transactional` }, { value: 'marketing', label: t`Marketing` }]} /></Form.Item>
        <Form.Item name="sort_order" label={t`Sort order`} rules={[{ required: true }]}><InputNumber min={0} max={10000} className="w-full" /></Form.Item>
        {editing && <Form.Item name="is_active" label={t`Enabled`} valuePropName="checked"><Switch /></Form.Item>}
      </Form>
    </Modal>
  </>
}

export default TemplateCategoryManager
