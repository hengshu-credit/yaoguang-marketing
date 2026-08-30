import { Button, Tag, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { NodeType } from '../../services/api/automation'

interface AccessibleAutomationNode {
  id: string
  data: {
    label: string
    nodeType: NodeType
  }
}

interface AutomationNodeListProps {
  nodes: AccessibleAutomationNode[]
  selectedNodeId?: string
  orphanNodeIds: Set<string>
  onSelect: (nodeId: string) => void
}

export function AutomationNodeList({ nodes, selectedNodeId, orphanNodeIds, onSelect }: AutomationNodeListProps) {
  const { t } = useLingui()
  return (
    <details className="automation-keyboard-node-list">
      <summary>{t`Keyboard node list`}</summary>
      <Typography.Paragraph type="secondary" style={{ margin: '6px 0' }}>
        {t`Select a node here to configure it without using the canvas.`}
      </Typography.Paragraph>
      <ol>
        {nodes.map((node, index) => {
          const orphan = orphanNodeIds.has(node.id)
          const selected = selectedNodeId === node.id
          const connectionStatus = orphan ? t`Not connected` : t`Connected`
          return (
            <li key={node.id}>
              <Button
                type={selected ? 'primary' : 'text'}
                block
                aria-current={selected ? 'step' : undefined}
                aria-label={`${index + 1}. ${node.data.label}, ${node.data.nodeType}, ${connectionStatus}`}
                onClick={() => onSelect(node.id)}
              >
                <span>{index + 1}. {node.data.label}</span>
                <Tag color={orphan ? 'orange' : 'green'}>{connectionStatus}</Tag>
              </Button>
            </li>
          )
        })}
      </ol>
    </details>
  )
}
