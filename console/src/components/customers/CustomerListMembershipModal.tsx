import { useEffect, useMemo } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Alert, App, Form, Modal, Radio, Select, Space, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import {
  customerApi,
  type CustomerListMembershipAction,
  type CustomerListMembershipStatus,
  type CustomerListMembershipUpdateResult
} from '../../services/api/customer'
import { listsApi } from '../../services/api/list'

interface CustomerListMembershipModalProps {
  workspaceId: string
  customerIds: string[]
  currentListIds?: string[]
  open: boolean
  onClose: () => void
  onSuccess: (result: CustomerListMembershipUpdateResult) => void
}

interface MembershipFormValues {
  action: CustomerListMembershipAction
  list_ids: string[]
  status?: CustomerListMembershipStatus
}

export function CustomerListMembershipModal({
  workspaceId,
  customerIds,
  currentListIds,
  open,
  onClose,
  onSuccess
}: CustomerListMembershipModalProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [form] = Form.useForm<MembershipFormValues>()
  const action = Form.useWatch('action', form) ?? 'add'
  const listsQuery = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId }),
    enabled: open
  })

  useEffect(() => {
    if (open) {
      form.setFieldsValue({ action: 'add', list_ids: [], status: 'active' })
    }
  }, [form, open])

  const listOptions = useMemo(() => {
    const current = currentListIds ? new Set(currentListIds) : undefined
    return (listsQuery.data?.lists ?? [])
      .filter((list) => action === 'add' || !current || current.has(list.id))
      .map((list) => ({ value: list.id, label: list.name }))
  }, [action, currentListIds, listsQuery.data?.lists])

  const mutation = useMutation({
    mutationFn: (values: MembershipFormValues) => customerApi.updateListMemberships({
      workspace_id: workspaceId,
      customer_ids: customerIds,
      list_ids: values.list_ids,
      action: values.action,
      status: values.action === 'remove' ? undefined : values.status
    }),
    onSuccess: (result) => {
      message.success(t`Updated ${result.changed} list memberships; ${result.unchanged} unchanged`)
      onSuccess(result)
      form.resetFields()
    }
  })

  const close = () => {
    if (mutation.isPending) return
    mutation.reset()
    form.resetFields()
    onClose()
  }

  const submit = () => {
    const fields: Array<keyof MembershipFormValues> = action === 'remove'
      ? ['action', 'list_ids']
      : ['action', 'list_ids', 'status']
    void form.validateFields(fields)
      .then((values) => mutation.mutate({ ...values, action }))
      .catch(() => undefined)
  }

  return (
    <Modal
      title={t`Adjust list memberships`}
      open={open}
      onCancel={close}
      onOk={submit}
      okText={t`Apply changes`}
      cancelText={t`Cancel`}
      confirmLoading={mutation.isPending}
      okButtonProps={{ disabled: customerIds.length === 0 || listsQuery.isLoading || listsQuery.isError }}
      destroyOnHidden
    >
      <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
        <Typography.Text type="secondary">
          {t`This operation applies to ${customerIds.length} selected customers.`}
        </Typography.Text>
        {listsQuery.isError ? (
          <Alert
            type="error"
            showIcon
            title={t`Unable to load lists`}
            description={listsQuery.error instanceof Error ? listsQuery.error.message : undefined}
          />
        ) : null}
        {mutation.isError ? (
          <Alert
            type="error"
            showIcon
            title={t`Unable to update list memberships`}
            description={mutation.error instanceof Error ? mutation.error.message : undefined}
          />
        ) : null}
        <Form<MembershipFormValues>
          form={form}
          layout="vertical"
          preserve={false}
        >
          <Form.Item name="action" label={t`Operation`} rules={[{ required: true }]}>
            <Radio.Group
              optionType="button"
              buttonStyle="solid"
              options={[
                { value: 'add', label: t`Add to lists` },
                { value: 'remove', label: t`Remove from lists` },
                { value: 'set_status', label: t`Change status` }
              ]}
              onChange={() => form.setFieldValue('list_ids', [])}
            />
          </Form.Item>
          <Form.Item
            name="list_ids"
            label={t`Target lists`}
            rules={[{ required: true, message: t`Select at least one list` }]}
          >
            <Select
              aria-label={t`Target lists`}
              mode="multiple"
              loading={listsQuery.isLoading}
              options={listOptions}
              placeholder={t`Select lists`}
              optionFilterProp="label"
            />
          </Form.Item>
          {action !== 'remove' ? (
            <Form.Item name="status" label={t`Membership status`} rules={[{ required: true }]}>
              <Select
                aria-label={t`Membership status`}
                options={[
                  { value: 'active', label: t`Active` },
                  { value: 'pending', label: t`Pending` },
                  { value: 'unsubscribed', label: t`Unsubscribed` },
                  { value: 'bounced', label: t`Bounced` },
                  { value: 'complained', label: t`Complained` }
                ]}
              />
            </Form.Item>
          ) : null}
          <Typography.Text type="secondary">
            {action === 'add'
              ? t`Existing memberships are left unchanged. Use Change status to update them explicitly.`
              : t`Customers without the selected memberships are skipped.`}
          </Typography.Text>
        </Form>
      </Space>
    </Modal>
  )
}
