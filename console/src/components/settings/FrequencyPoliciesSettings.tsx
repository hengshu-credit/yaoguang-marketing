import { useCallback, useEffect, useState } from 'react'
import { Alert, message, Skeleton, Space, Typography } from 'antd'
import { FrequencyPolicyForm } from '../frequency/FrequencyPolicyForm'
import { frequencyPolicyApi, type FrequencyPolicy, type FrequencyPolicyScope, type SaveFrequencyPolicyRequest } from '../../services/api/frequency_policy'

const { Title, Paragraph } = Typography

export function FrequencyPoliciesSettings({ workspaceId }: { workspaceId: string }) {
  const [policies, setPolicies] = useState<FrequencyPolicy[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState<FrequencyPolicyScope>()
  const load = useCallback(async () => {
    setLoading(true)
    try { setPolicies((await frequencyPolicyApi.list(workspaceId)).policies) }
    catch (error) { message.error(error instanceof Error ? error.message : '频控策略加载失败') }
    finally { setLoading(false) }
  }, [workspaceId])
  useEffect(() => { void load() }, [load])

  const save = async (scope: FrequencyPolicyScope, request: Omit<SaveFrequencyPolicyRequest, 'workspace_id'>) => {
    setSaving(scope)
    try {
      await frequencyPolicyApi.save({ ...request, workspace_id: workspaceId })
      message.success('频控策略已保存为新版本')
      await load()
    } catch (error) { message.error(error instanceof Error ? error.message : '频控策略保存失败') }
    finally { setSaving(undefined) }
  }

  if (loading) return <Skeleton active />
  return (
    <Space orientation="vertical" size="large" style={{ width: '100%' }}>
      <div><Title level={2}>触达频控</Title><Paragraph type="secondary">按客户和渠道限制营销消息次数。任意一层达到上限都会阻止本次触达，基础设施异常时会延后，不会放行。</Paragraph></div>
      <Alert type="info" showIcon title="三个层级彼此独立" description="建议同时配置活动级保护和 Workspace 全量底线。Automation 的入场频次仍在自动化旅程中配置。" />
      {(['campaign', 'trigger', 'workspace_global'] as FrequencyPolicyScope[]).map((scope) => (
        <FrequencyPolicyForm key={scope} scope={scope} policy={policies.find((item) => item.scope === scope)} saving={saving === scope} onSave={(request) => save(scope, request)} />
      ))}
    </Space>
  )
}
