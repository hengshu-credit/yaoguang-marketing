import { Alert, Form, Modal, Select } from 'antd'
import { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { automationApi, type AutomationAudienceRunResult } from '../../services/api/automation'

interface AutomationOption {
  id: string
  name: string
  status: string
}

interface AudienceOption {
  id: string
  name: string
}

interface AutomationAudienceRunModalProps {
  open: boolean
  workspaceId: string
  automations: AutomationOption[]
  audiences: AudienceOption[]
  onClose: () => void
}

export function AutomationAudienceRunModal({
  open,
  workspaceId,
  automations,
  audiences,
  onClose
}: AutomationAudienceRunModalProps) {
  const { t } = useLingui()
  const [form] = Form.useForm<{ automation_id: string; audience_id: string }>()
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<AutomationAudienceRunResult>()
  const [error, setError] = useState<string>()

  const close = () => {
    form.resetFields()
    setResult(undefined)
    setError(undefined)
    onClose()
  }

  const start = async () => {
    const values = await form.validateFields()
    setLoading(true)
    setError(undefined)
    try {
      setResult(await automationApi.startAudience(workspaceId, values.automation_id, values.audience_id))
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : t`Failed to start audience run`)
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal
      open={open}
      title={t`Start automation from audience`}
      okText={t`Start audience run`}
      cancelText={t`Close`}
      confirmLoading={loading}
      onOk={() => void start()}
      onCancel={close}
    >
      <Alert
        type="info"
        showIcon
        className="mb-4"
        description={t`The latest audience rule is evaluated now. Candidates enter the journey once, and every message node checks the same rule again against current customer data.`}
      />
      <Form form={form} layout="vertical">
        <Form.Item name="automation_id" label={t`Live automation`} rules={[{ required: true }]}>
          <Select options={automations.filter((item) => item.status === 'live').map((item) => ({ value: item.id, label: item.name }))} />
        </Form.Item>
        <Form.Item name="audience_id" label={t`Dynamic audience`} rules={[{ required: true }]}>
          <Select options={audiences.map((item) => ({ value: item.id, label: item.name }))} />
        </Form.Item>
      </Form>
      {result && <Alert type="success" showIcon title={t`${result.candidate_count} candidates, ${result.enrolled_count} enrolled`} />}
      {error && <Alert type="error" showIcon title={error} />}
    </Modal>
  )
}
