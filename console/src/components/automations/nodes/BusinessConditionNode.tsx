import { Handle, Position, type NodeProps } from '@xyflow/react'
import { ListChecks } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { BaseNode } from './BaseNode'
import type { AutomationFlowNode } from '../utils/flowConverter'

export function BusinessConditionNode({ data, selected }: NodeProps<AutomationFlowNode>) {
  const { t } = useLingui()
  const label = data.config.mode === 'exit_if' ? t`Exit when matched` : data.config.mode === 'continue_if' ? t`Continue only when matched` : t`Wait until matched`
  return <><Handle type="target" position={Position.Top} /><BaseNode type="business_condition" label={t`Event or state condition`} icon={<ListChecks size={16} />} selected={selected} isOrphan={data.isOrphan} onDelete={data.onDelete}><div>{label}</div><div className="text-xs text-gray-500">{String(data.config.subject_type ?? '')}</div></BaseNode><Handle type="source" position={Position.Bottom} /></>
}
