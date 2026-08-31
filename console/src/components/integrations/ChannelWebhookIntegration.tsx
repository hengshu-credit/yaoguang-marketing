import { useState } from 'react'
import { App, Button, Card, Drawer, Form, Input, InputNumber, Select, Space, Tag, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { ChannelDefinition } from '../../services/api/channels'
import type { ChannelWebhookSettings, Integration, Workspace } from '../../services/api/workspace'
import { workspaceService } from '../../services/api/workspace'

const { Text } = Typography

interface ChannelWebhookIntegrationProps {
  workspace: Workspace
  definitions: ChannelDefinition[]
  isOwner: boolean
  onSaved: () => Promise<void>
}

interface ChannelWebhookFormValues {
  name: string
  endpoint_url: string
  secret?: string
  channels: string[]
  timeout_seconds: number
}

const ChannelWebhookIntegration: React.FC<ChannelWebhookIntegrationProps> = ({ workspace, definitions, isOwner, onSaved }) => {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [form] = Form.useForm<ChannelWebhookFormValues>()
  const [open, setOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [editing, setEditing] = useState<Integration | null>(null)
  const integrations = (workspace.integrations || []).filter((integration) => integration.type === 'channel_webhook')
  const sendableChannels = definitions.filter((definition) => definition.delivery_modes.includes('signed_webhook'))

  const beginCreate = () => {
    setEditing(null)
    form.setFieldsValue({ name: '', endpoint_url: '', secret: '', channels: [], timeout_seconds: 5 })
    setOpen(true)
  }
  const beginEdit = (integration: Integration) => {
    const settings = integration.channel_webhook_settings
    setEditing(integration)
    form.setFieldsValue({
      name: integration.name, endpoint_url: settings?.endpoint_url || '', secret: '',
      channels: settings?.channels || [], timeout_seconds: settings?.timeout_seconds || 5
    })
    setOpen(true)
  }
  const save = async (values: ChannelWebhookFormValues) => {
    setSaving(true)
    try {
      const settings: ChannelWebhookSettings = {
        endpoint_url: values.endpoint_url.trim(), channels: values.channels,
        timeout_seconds: values.timeout_seconds, ...(values.secret?.trim() ? { secret: values.secret } : {})
      }
      if (editing) {
        await workspaceService.updateIntegration({
          workspace_id: workspace.id, integration_id: editing.id, name: values.name.trim(),
          channel_webhook_settings: settings
        })
      } else {
        await workspaceService.createIntegration({
          workspace_id: workspace.id, name: values.name.trim(), type: 'channel_webhook',
          channel_webhook_settings: settings
        })
      }
      await onSaved()
      message.success(editing ? t`Channel Webhook updated` : t`Channel Webhook created`)
      setOpen(false)
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Failed to save Channel Webhook`)
    } finally {
      setSaving(false)
    }
  }

  return <div className="mb-6">
    <div className="mb-3 flex items-center justify-between gap-3">
      <div><Text strong>{t`Channel delivery Webhooks`}</Text><div><Text type="secondary">{t`Deliver new messaging channels through a signed HTTPS bridge.`}</Text></div></div>
      {isOwner && <Button type="primary" ghost onClick={beginCreate}>{t`Add Channel Webhook`}</Button>}
    </div>
    <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
      {integrations.map((integration) => {
        const settings = integration.channel_webhook_settings
        const hint = integration.credential_hints?.['channel_webhook.secret']
        return <Card key={integration.id} size="small" title={integration.name} extra={isOwner ? <Button type="link" aria-label={`Edit ${integration.name}`} onClick={() => beginEdit(integration)}>{t`Edit`}</Button> : null}>
          <Space orientation="vertical" size={6}>
            <Text code>{settings?.endpoint_url}</Text>
            <Space wrap>{settings?.channels.map((channel) => <Tag key={channel}>{definitions.find((definition) => definition.id === channel)?.label_key || channel}</Tag>)}</Space>
            <Text type="secondary">{hint ? t`Secret configured, ends in ${hint}` : t`Secret configured`}</Text>
          </Space>
        </Card>
      })}
    </div>
    <Drawer title={editing ? t`Edit Channel Webhook` : t`Add Channel Webhook`} open={open} onClose={() => setOpen(false)} size={560} destroyOnHidden extra={<Button type="primary" loading={saving} onClick={() => form.submit()}>{t`Save`}</Button>}>
      <Form form={form} layout="vertical" onFinish={save}>
        <Form.Item label={t`Name`} name="name" rules={[{ required: true }]}><Input maxLength={64} /></Form.Item>
        <Form.Item label={t`HTTPS endpoint`} name="endpoint_url" rules={[{ required: true }, { pattern: /^https:\/\/[^\s]+$/i, message: t`Enter an absolute HTTPS URL` }]}><Input placeholder="https://bridge.example/channel" /></Form.Item>
        <Form.Item label={editing ? t`Replace secret` : t`Signing secret`} name="secret" rules={editing ? undefined : [{ required: true }]}><Input.Password autoComplete="new-password" placeholder={editing ? t`Leave blank to keep the current secret` : undefined} /></Form.Item>
        <Form.Item label={t`Allowed channels`} name="channels" rules={[{ required: true }]}><Select mode="multiple" options={sendableChannels.map((definition) => ({ label: definition.label_key, value: definition.id }))} /></Form.Item>
        <Form.Item label={t`Timeout (seconds)`} name="timeout_seconds" rules={[{ required: true }]}><InputNumber min={1} max={30} className="w-full" /></Form.Item>
      </Form>
    </Drawer>
  </div>
}

export default ChannelWebhookIntegration
