import { useEffect, useMemo } from 'react'
import { Alert, Button, Card, Form, Input, InputNumber, Select, Space, Switch, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
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

export function FrequencyPolicyForm({
  scope,
  policy,
  defaultScopeRef,
  fixedScopeRef,
  saving,
  onSave
}: FrequencyPolicyFormProps) {
  const { t } = useLingui()
  const [form] = Form.useForm<FormValues>()
  const windowKind = Form.useWatch('window_kind', form)
  const enabled = Form.useWatch('enabled', form)
  const copy = useMemo<Record<FrequencyPolicyScope, {
    title: string
    description: string
    refLabel?: string
    refRequiredMessage?: string
  }>>(() => ({
    campaign: {
      title: t`Campaign limit`,
      description: t`Count only deliveries in the same marketing campaign without affecting other campaigns.`,
      refLabel: t`Campaign ID`,
      refRequiredMessage: t`Enter the campaign ID`
    },
    trigger: {
      title: t`Event / scheduled trigger limit`,
      description: t`Limit messages produced by the same automation trigger without changing whether the customer enters the Journey.`,
      refLabel: t`Automation:trigger identifier`,
      refRequiredMessage: t`Enter the automation:trigger identifier`
    },
    workspace_global: {
      title: t`Workspace-wide limit`,
      description: t`Protect each customer across campaigns and automations as a Workspace-wide safety limit.`
    }
  }), [t])
  const activeCopy = copy[scope]

  useEffect(() => {
    const seconds = policy?.window_seconds ?? (scope === 'workspace_global' ? 86400 : 3600)
    const unit = seconds % 86400 === 0 ? 'day' : 'hour'
    form.setFieldsValue({
      enabled: policy?.enabled ?? false,
      name: policy?.name ?? activeCopy.title,
      scope_ref: fixedScopeRef ?? policy?.scope_ref ?? defaultScopeRef,
      channel: policy?.channel ?? 'email',
      max_events: policy?.max_events ?? (scope === 'workspace_global' ? 3 : 1),
      window_kind: policy?.window_kind ?? (scope === 'workspace_global' ? 'calendar' : 'sliding'),
      window_value: unit === 'day' ? Math.max(1, seconds / 86400) : Math.max(1, seconds / 3600),
      window_unit: unit,
      timezone: policy?.timezone ?? 'Asia/Shanghai',
      deny_action: policy?.deny_action ?? 'suppress'
    })
  }, [activeCopy.title, defaultScopeRef, fixedScopeRef, form, policy, scope])

  const channelOptions = [
    { value: 'email', label: t`Email` },
    { value: 'sms', label: t`SMS` },
    { value: 'push', label: t`Push` },
    { value: 'whatsapp', label: t`WhatsApp` },
    { value: 'telegram', label: t`Telegram` },
    { value: 'in_app', label: t`In-App` },
    { value: 'webhook', label: t`Webhook` }
  ]

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
    <Card title={activeCopy.title} extra={<Switch aria-label={`${t`Toggle`} ${activeCopy.title}`} checked={Boolean(enabled)} onChange={(checked) => form.setFieldValue('enabled', checked)} />}>
      <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
        <Text type="secondary">{activeCopy.description}</Text>
        {scope === 'trigger' && <Alert type="info" showIcon title={t`Entry frequency and message frequency control are separate rules`} description={t`“Once per customer / re-enter on every event” controls Journey entry. This policy controls message delivery after entry.`} />}
        <Form form={form} layout="vertical" disabled={!enabled}>
          <Form.Item name="enabled" valuePropName="checked" hidden><Switch /></Form.Item>
          <Form.Item name="name" label={t`Policy name`} rules={[{ required: true }]}><Input /></Form.Item>
          {activeCopy.refLabel && !fixedScopeRef && <Form.Item name="scope_ref" label={activeCopy.refLabel} rules={[{ required: enabled, message: activeCopy.refRequiredMessage }]}><Input placeholder={scope === 'trigger' ? 'automation-id:event' : 'campaign-id'} /></Form.Item>}
          <Form.Item name="channel" label={t`Delivery channel`} rules={[{ required: true }]}>
            <Select options={channelOptions} />
          </Form.Item>
          <Space align="end" wrap>
            <Form.Item name="window_value" label={t`For each customer in`} rules={[{ required: true }]}><InputNumber min={1} max={365} /></Form.Item>
            <Form.Item name="window_unit" label={t`Time range`}><Select style={{ width: 100 }} options={[{ value: 'hour', label: t`Hours` }, { value: 'day', label: t`Days` }]} /></Form.Item>
            <Form.Item name="max_events" label={t`Maximum deliveries`} rules={[{ required: true }]}><InputNumber min={1} max={1000} /></Form.Item>
          </Space>
          <Form.Item name="window_kind" label={t`Window calculation`}><Select options={[{ value: 'sliding', label: t`Rolling window` }, { value: 'calendar', label: t`Calendar day / cycle` }]} /></Form.Item>
          {windowKind === 'calendar' && <Form.Item name="timezone" label={t`Calendar cycle timezone`} rules={[{ required: true }]}><Select showSearch options={[{ value: 'Asia/Shanghai', label: 'Asia/Shanghai (UTC+08)' }, { value: 'UTC', label: 'UTC' }]} /></Form.Item>}
          <Form.Item name="deny_action" label={t`After reaching the limit`}><Select options={[{ value: 'suppress', label: t`Do not send this message` }, { value: 'defer', label: t`Retry later` }]} /></Form.Item>
          <Button type="primary" loading={saving} onClick={() => { void submit() }}>{t`Save this policy`}</Button>
        </Form>
      </Space>
    </Card>
  )
}
