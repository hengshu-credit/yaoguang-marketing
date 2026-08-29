import React from 'react'
import { Handle, Position, useConnection, type NodeProps } from '@xyflow/react'
import { MessageSquareText, Smartphone } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { BaseNode } from './BaseNode'
import { getNodeDescription, nodeTypeColors } from './constants'
import type { AutomationFlowNode, Structural } from '../utils/flowConverter'
import type { ChannelNodeConfig } from '../../../services/api/automation'

export const ChannelNode: React.FC<NodeProps<AutomationFlowNode>> = ({ data, selected }) => {
  const { t } = useLingui()
  const nodeType = data.nodeType === 'push' ? 'push' : 'sms'
  const config = data.config as Structural<ChannelNodeConfig>
  const connection = useConnection()
  const color = nodeTypeColors[nodeType]
  const label = nodeType === 'sms' ? t`SMS` : t`Push`
  const icon = nodeType === 'sms' ? <MessageSquareText size={16} color={selected ? undefined : color} /> : <Smartphone size={16} color={selected ? undefined : color} />
  return (
    <>
      <Handle
        type="target"
        position={Position.Top}
        style={{ background: data.isOrphan ? '#f97316' : '#3b82f6', width: connection.inProgress ? 16 : 10, height: connection.inProgress ? 16 : 10 }}
      />
      <BaseNode
        type={nodeType}
        label={label}
        description={getNodeDescription(data.config)}
        icon={icon}
        selected={selected}
        isOrphan={data.isOrphan}
        onDelete={data.onDelete}
      >
        {config.template_id ? <div>{config.template_id}</div> : <div className="text-orange-500">{t`Select`}</div>}
      </BaseNode>
      <Handle type="source" position={Position.Bottom} style={{ background: data.isOrphan ? '#f97316' : '#3b82f6', width: 10, height: 10 }} />
    </>
  )
}
