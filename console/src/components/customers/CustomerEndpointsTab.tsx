import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { Alert, App, Button, Checkbox, Form, Input, Select, Space, Table, Typography } from 'antd'
import { customerEndpointsApi, type CustomerEndpoint, type CustomerEndpointMutation } from '../../services/api/customer_endpoints'
import { customerQueryKeys, type CustomerConsent } from '../../services/api/customer'
import { ActionableError } from '../errors/ActionableError'
interface Values { endpoint_id: string; provider: string; platform: string; address: string; locale?: string; marketing_consent: boolean }
export function CustomerEndpointsTab({ workspaceId, customerId, canWrite, consents = [] }: { workspaceId: string; customerId: string; canWrite: boolean; consents?: CustomerConsent[] }) {
 const { t } = useLingui()
 const { message } = App.useApp()
 const client = useQueryClient()
 const [channel, setChannel] = useState('sms')
 const [form] = Form.useForm<Values>()
 const query = useQuery({ queryKey: ['customer-endpoints', workspaceId, customerId, channel], queryFn: () => customerEndpointsApi.list(workspaceId, customerId, channel), refetchInterval: 60_000 })
 const mutation = useMutation({ mutationFn: ({ endpoint, consent }: { endpoint: CustomerEndpointMutation; consent?: boolean }) => customerEndpointsApi.save(workspaceId, customerId, endpoint, consent), onSuccess: async () => { await Promise.all([client.invalidateQueries({ queryKey: ['customer-endpoints', workspaceId, customerId] }), client.invalidateQueries({ queryKey: customerQueryKeys.detail(workspaceId, customerId) })]); form.resetFields(); message.success(t`Customer endpoint saved`) } })
 const effectiveConsent = consents.some((consent) => consent.purpose === 'marketing' && consent.channel === channel && ['granted', 'active'].includes(consent.status) && Date.parse(consent.valid_from) <= query.dataUpdatedAt && (!consent.revoked_at || Date.parse(consent.revoked_at) > query.dataUpdatedAt))
 return <Space orientation="vertical" style={{ width: '100%' }}>
  <Select aria-label={t`Channel`} value={channel} onChange={(value) => { setChannel(value); form.resetFields() }} options={[{ value: 'sms', label: t`SMS` }, { value: 'push', label: t`Push notification` }]} style={{ width: 240 }} />
  <Alert type={effectiveConsent ? 'success' : 'warning'} showIcon title={effectiveConsent ? t`Marketing consent is active for this channel` : t`Marketing consent is not active for this channel`} />
  {query.isError && <ActionableError error={query.error} onRetry={() => void query.refetch()} />}
  <Table size="small" rowKey="endpoint_id" dataSource={query.data?.endpoints ?? []} loading={query.isPending} pagination={false} columns={[
   { title: t`Endpoint ID`, dataIndex: 'endpoint_id' }, { title: t`Provider`, dataIndex: 'provider' }, { title: t`Platform`, dataIndex: 'platform' },
   { title: t`Status`, render: (_: unknown, endpoint: CustomerEndpoint) => endpoint.enabled ? t`Enabled` : t`Disabled` },
   { title: t`Actions`, render: (_: unknown, endpoint: CustomerEndpoint) => canWrite && endpoint.enabled ? <Button danger size="small" loading={mutation.isPending} onClick={() => mutation.mutate({ endpoint: { endpoint_id: endpoint.endpoint_id, channel, operation: 'disable' } })}>{t`Disable`}</Button> : null }
  ]} />
  {canWrite && <Form key={channel} form={form} layout="vertical" initialValues={{ marketing_consent: false, provider: channel === 'sms' ? 'twilio' : 'fcm', platform: channel === 'sms' ? 'phone' : 'android' }} onFinish={(values) => mutation.mutate({ endpoint: { operation: 'upsert', channel, endpoint_id: values.endpoint_id, provider: values.provider, platform: values.platform, address: values.address, locale: values.locale }, consent: values.marketing_consent })}>
   <Typography.Title level={5}>{t`Add or replace customer endpoint`}</Typography.Title>
   <Form.Item name="endpoint_id" label={t`Endpoint ID`} rules={[{ required: true, message: t`Enter an endpoint ID` }]}><Input maxLength={128} /></Form.Item>
   <Form.Item name="provider" label={t`Provider`} rules={[{ required: true }]}><Select options={channel === 'sms' ? [{ value: 'twilio', label: t`Twilio` }] : [{ value: 'fcm', label: t`FCM` }, { value: 'apns', label: t`APNs` }, { value: 'webpush', label: t`Web Push` }]} /></Form.Item>
   <Form.Item name="platform" label={t`Platform`} rules={[{ required: true }]}><Select options={channel === 'sms' ? [{ value: 'phone', label: t`Phone` }] : [{ value: 'android', label: t`Android` }, { value: 'ios', label: t`iOS` }, { value: 'web', label: t`Web` }]} /></Form.Item>
   <Form.Item name="address" label={channel === 'sms' ? t`Phone number in international format` : t`Device token or browser subscription`} rules={[{ required: true, message: t`Enter the endpoint address` }]}><Input.TextArea rows={3} /></Form.Item>
   <Form.Item name="locale" label={t`Language code`}><Input maxLength={35} /></Form.Item>
   <Form.Item name="marketing_consent" valuePropName="checked"><Checkbox>{t`I confirm this customer has given marketing consent for this channel`}</Checkbox></Form.Item>
   <Typography.Paragraph type="secondary">{t`Saving with this box unchecked revokes marketing consent for this channel. Endpoint registration alone does not grant consent.`}</Typography.Paragraph>
   <Button type="primary" htmlType="submit" loading={mutation.isPending}>{t`Save customer endpoint`}</Button>
  </Form>}
  {mutation.isError && <ActionableError error={mutation.error} />}
 </Space>
}
