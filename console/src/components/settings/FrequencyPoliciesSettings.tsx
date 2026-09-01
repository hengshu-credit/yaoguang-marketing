import { useCallback, useEffect, useState } from 'react'
import { Alert, message, Skeleton, Space, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { FrequencyPolicyForm } from '../frequency/FrequencyPolicyForm'
import { frequencyPolicyApi, type FrequencyPolicy, type FrequencyPolicyScope, type SaveFrequencyPolicyRequest } from '../../services/api/frequency_policy'
import { WorkspacePageTitle } from '../navigation/WorkspacePageTitle'

const { Paragraph } = Typography

export function FrequencyPoliciesSettings({ workspaceId }: { workspaceId: string }) {
  const { t } = useLingui()
  const [policies, setPolicies] = useState<FrequencyPolicy[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState<FrequencyPolicyScope>()
  const load = useCallback(async () => {
    setLoading(true)
    try { setPolicies((await frequencyPolicyApi.list(workspaceId)).policies) }
    catch (error) { message.error(error instanceof Error ? error.message : t`Failed to load frequency policies`) }
    finally { setLoading(false) }
  }, [t, workspaceId])
  useEffect(() => { void load() }, [load])

  const save = async (scope: FrequencyPolicyScope, request: Omit<SaveFrequencyPolicyRequest, 'workspace_id'>) => {
    setSaving(scope)
    try {
      await frequencyPolicyApi.save({ ...request, workspace_id: workspaceId })
      message.success(t`Frequency policy saved as a new version`)
      await load()
    } catch (error) { message.error(error instanceof Error ? error.message : t`Failed to save frequency policy`) }
    finally { setSaving(undefined) }
  }

  if (loading) return <Skeleton active />
  return (
    <Space orientation="vertical" size="large" style={{ width: '100%' }}>
      <div>
        <WorkspacePageTitle style={{ marginBottom: 8 }}>{t`Message frequency control`}</WorkspacePageTitle>
        <Paragraph type="secondary">
          {t`Limit marketing messages per customer and channel.`} {t`Reaching any level blocks this delivery; infrastructure failures defer it instead of allowing it.`}
        </Paragraph>
      </div>
      <Alert
        type="info"
        showIcon
        title={t`The three levels are independent`}
        description={t`Configure both campaign protection and a Workspace-wide safety limit. Automation entry frequency remains configured in the Journey.`}
      />
      {(['campaign', 'trigger', 'workspace_global'] as FrequencyPolicyScope[]).map((scope) => (
        <FrequencyPolicyForm key={scope} scope={scope} policy={policies.find((item) => item.scope === scope)} saving={saving === scope} onSave={(request) => save(scope, request)} />
      ))}
    </Space>
  )
}
