import { Background, Controls, ReactFlow } from '@xyflow/react'
import { useLingui } from '@lingui/react/macro'
import { Typography } from 'antd'
import type { FunnelDefinition } from '../../services/api/funnels'
import { buildFunnelGraph } from './funnel-graph'
import '@xyflow/react/dist/style.css'
export function FunnelPathOverview({ definition }: { definition: FunnelDefinition }) {
 const { t } = useLingui()
 const graph = buildFunnelGraph(definition)
 const nodes = graph.nodes.map(node => ({ ...node, data: { ...node.data, label: `${node.data.label}${node.data.mode ? ` · ${node.data.mode === 'all' ? t`All branches` : t`Any branch`}` : ''}${node.data.optional ? ` · ${t`Optional`}` : ''}` } }))
 return <><Typography.Paragraph type="secondary">{t`Branches share the same entry cohort. OR branches may overlap; their union is counted once per object.`}</Typography.Paragraph><div style={{ height: 320 }}><ReactFlow nodes={nodes} edges={graph.edges} fitView nodesConnectable={false} nodesDraggable={false}><Background /><Controls showInteractive={false} /></ReactFlow></div></>
}
