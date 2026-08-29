import React from 'react'
import { Form, Input, Select } from 'antd'
import { useLingui } from '@lingui/react/macro'
import TemplateSelectorInput from '../../templates/TemplateSelectorInput'
import type { ChannelNodeConfig, NodeType } from '../../../services/api/automation'
import type { Workspace } from '../../../services/api/types'

interface ChannelConfigFormProps {
  nodeType: Extract<NodeType, 'sms' | 'push'>
  config: ChannelNodeConfig
  onChange: (config: ChannelNodeConfig) => void
  workspaceId: string
  workspace: Workspace
}

export const ChannelConfigForm: React.FC<ChannelConfigFormProps> = ({
  nodeType,
  config,
  onChange,
  workspaceId,
  workspace
}) => {
  const { t } = useLingui()
  const integrations = React.useMemo(
    () => workspace.integrations?.filter((integration) => integration.type === nodeType) || [],
    [workspace.integrations, nodeType]
  )
  const languages = workspace.settings.languages || []
  const channelName = nodeType === 'sms' ? t`SMS` : t`Push`

  return (
    <Form layout="vertical" className="nodrag">
      <Form.Item label={`${channelName} ${t`Template`}`} required>
        <TemplateSelectorInput
          value={config.template_id || null}
          onChange={(templateId) => onChange({ ...config, template_id: templateId || '' })}
          workspaceId={workspaceId}
          channel={nodeType}
          placeholder={t`Select a template`}
        />
      </Form.Item>
      <Form.Item label={`${channelName} ${t`Integration`}`} required>
        <Select
          value={config.integration_id || undefined}
          placeholder={t`Select an integration`}
          onChange={(integration_id) => onChange({ ...config, integration_id })}
          options={integrations.map((integration) => ({ label: integration.name, value: integration.id }))}
        />
      </Form.Item>
      <Form.Item
        label={t`Endpoint ID`}
        extra={t`Optional. Leave blank to use the contact's most recently active endpoint.`}
      >
        <Input
          value={config.endpoint_id}
          onChange={(event) => onChange({ ...config, endpoint_id: event.target.value || undefined })}
        />
      </Form.Item>
      {languages.length > 0 && (
        <Form.Item label={t`Language`} extra={t`Optional. Contact and endpoint locale are used by default.`}>
          <Select
            allowClear
            value={config.language || undefined}
            onChange={(language) => onChange({ ...config, language })}
            options={languages.map((language) => ({ label: language, value: language }))}
          />
        </Form.Item>
      )}
    </Form>
  )
}
