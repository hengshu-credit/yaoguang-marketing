import { useEffect } from 'react'
import { Alert, Button, Card, Form, Input, InputNumber, Select, Space, Switch, Typography } from 'antd'
import type { FrequencyPolicy, FrequencyPolicyScope, SaveFrequencyPolicyRequest } from '../../services/api/frequency_policy'

const { Text } = Typography

interface FormValues {
  enabled: boolean
  name: string
  scope_ref?: string
  channel: string
  max_events: number
  window_kind: 'sliding' | 'calendar'
  window_value: number
  window_unit: 'hour' | 'day'
  timezone?: string
  deny_action: 'suppress' | 'defer'
}

interface FrequencyPolicyFormProps {
  scope: FrequencyPolicyScope
  policy?: FrequencyPolicy
  defaultScopeRef?: string
  fixedScopeRef?: string
  saving?: boolean
  onSave: (request: Omit<SaveFrequencyPolicyRequest, 'workspace_id'>) => Promise<void> | void
}

const scopeCopy: Record<FrequencyPolicyScope, { title: string; description: string; refLabel?: string }> = {
  campaign: { title: '本活动限制', description: '只统计同一营销活动内的触达，不影响其他活动。', refLabel: '活动 ID' },
  trigger: { title: '事件 / 定时触发限制', description: '限制同一自动化触发器产生的消息；不改变客户是否进入 Journey。', refLabel: '自动化:触发器标识' },
  workspace_global: { title: 'Workspace 全量限制', description: '跨活动和自动化统一保护每位客户，适合作为安全底线。' }
}

export function FrequencyPolicyForm({
  scope,
  policy,
  defaultScopeRef,
  fixedScopeRef,
  saving,
  onSave
}: FrequencyPolicyFormProps) {
  const [form] = Form.useForm<FormValues>()
  const windowKind = Form.useWatch('window_kind', form)
  const enabled = Form.useWatch('enabled', form)
  const copy = scopeCopy[scope]

  useEffect(() => {
    const seconds = policy?.window_seconds ?? (scope === 'workspace_global' ? 86400 : 3600)
    const unit = seconds % 86400 === 0 ? 'day' : 'hour'
    form.setFieldsValue({
      enabled: policy?.enabled ?? false,
      name: policy?.name ?? copy.title,
      scope_ref: fixedScopeRef ?? policy?.scope_ref ?? defaultScopeRef,
      channel: policy?.channel ?? 'email',
      max_events: policy?.max_events ?? (scope === 'workspace_global' ? 3 : 1),
      window_kind: policy?.window_kind ?? (scope === 'workspace_global' ? 'calendar' : 'sliding'),
      window_value: unit === 'day' ? Math.max(1, seconds / 86400) : Math.max(1, seconds / 3600),
      window_unit: unit,
      timezone: policy?.timezone ?? 'Asia/Shanghai',
      deny_action: policy?.deny_action ?? 'suppress'
    })
  }, [copy.title, defaultScopeRef, fixedScopeRef, form, policy, scope])

  const submit = async () => {
    const values = await form.validateFields()
    const multiplier = values.window_unit === 'day' ? 86400 : 3600
    await onSave({
      id: policy?.id ?? '', name: values.name, scope, scope_ref: scope === 'workspace_global' ? undefined : fixedScopeRef ?? values.scope_ref,
      channel: values.channel, max_events: values.max_events, window_kind: values.window_kind,
      window_seconds: values.window_value * multiplier,
      timezone: values.window_kind === 'calendar' ? values.timezone : undefined,
      deny_action: values.deny_action, priority: policy?.priority ?? (scope === 'workspace_global' ? 100 : 200), enabled: values.enabled
    })
  }

  return (
    <Card title={copy.title} extra={<Switch aria-label={`${copy.title}开关`} checked={Boolean(enabled)} onChange={(checked) => form.setFieldValue('enabled', checked)} />}>
      <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
        <Text type="secondary">{copy.description}</Text>
        {scope === 'trigger' && <Alert type="info" showIcon title="入场频次与消息频控是两套规则" description="“每个客户一次 / 每次事件重新进入”决定 Journey 入场；本卡只决定入场后消息是否触达。" />}
        <Form form={form} layout="vertical" disabled={!enabled}>
          <Form.Item name="enabled" valuePropName="checked" hidden><Switch /></Form.Item>
          <Form.Item name="name" label="策略名称" rules={[{ required: true }]}><Input /></Form.Item>
          {copy.refLabel && !fixedScopeRef && <Form.Item name="scope_ref" label={copy.refLabel} rules={[{ required: enabled, message: `请输入${copy.refLabel}` }]}><Input placeholder={scope === 'trigger' ? 'automation-id:event' : 'campaign-id'} /></Form.Item>}
          <Form.Item name="channel" label="触达渠道" rules={[{ required: true }]}>
            <Select options={['email', 'sms', 'push', 'whatsapp', 'telegram', 'in_app', 'webhook'].map((value) => ({ value, label: value }))} />
          </Form.Item>
          <Space align="end" wrap>
            <Form.Item name="window_value" label="每位客户在" rules={[{ required: true }]}><InputNumber min={1} max={365} /></Form.Item>
            <Form.Item name="window_unit" label="时间范围"><Select style={{ width: 100 }} options={[{ value: 'hour', label: '小时' }, { value: 'day', label: '天' }]} /></Form.Item>
            <Form.Item name="max_events" label="最多触达（次）" rules={[{ required: true }]}><InputNumber min={1} max={1000} /></Form.Item>
          </Space>
          <Form.Item name="window_kind" label="窗口计算方式"><Select options={[{ value: 'sliding', label: '滚动窗口' }, { value: 'calendar', label: '自然日 / 自然周期' }]} /></Form.Item>
          {windowKind === 'calendar' && <Form.Item name="timezone" label="自然周期时区" rules={[{ required: true }]}><Select showSearch options={[{ value: 'Asia/Shanghai', label: 'Asia/Shanghai (UTC+08)' }, { value: 'UTC', label: 'UTC' }]} /></Form.Item>}
          <Form.Item name="deny_action" label="超过限制后"><Select options={[{ value: 'suppress', label: '本次不发送' }, { value: 'defer', label: '延后重试' }]} /></Form.Item>
          <Button type="primary" loading={saving} onClick={() => { void submit() }}>保存此层策略</Button>
        </Form>
      </Space>
    </Card>
  )
}
