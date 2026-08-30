import React from 'react'
import { Typography, Empty, Form, Input } from 'antd'
import { X } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import type { Node } from '@xyflow/react'
import {
  TriggerConfigForm,
  DelayConfigForm,
  EmailConfigForm,
  ChannelConfigForm,
  ABTestConfigForm,
  AddToListConfigForm,
  RemoveFromListConfigForm,
  FilterConfigForm,
  WebhookConfigForm,
  ListStatusBranchConfigForm,
  type TriggerConfig
} from './config'
import { useAutomation } from './context'
import type { AutomationNodeData, Structural } from './utils/flowConverter'
import type {
  DelayNodeConfig,
  EmailNodeConfig,
  ChannelNodeConfig,
  ABTestNodeConfig,
  AddToListNodeConfig,
  RemoveFromListNodeConfig,
  FilterNodeConfig,
  WebhookNodeConfig,
  ListStatusBranchNodeConfig
} from '../../services/api/automation'

const { Title } = Typography

// The per-type forms declare `onChange` against their own concrete config interface. An interface
// carries no implicit index signature, so a `(config: Record<string, unknown>) => void` handler is
// assignable to none of them. `Structural<T>` re-expresses each config as an anonymous object type
// with the same members: it accepts the interface on the way in (parameters are contravariant) and
// satisfies `Record<string, unknown>` on the way back out into the node's config bag.
type NodeConfigUpdate =
  | Structural<TriggerConfig>
  | Structural<DelayNodeConfig>
  | Structural<EmailNodeConfig>
  | Structural<ChannelNodeConfig>
  | Structural<ABTestNodeConfig>
  | Structural<AddToListNodeConfig>
  | Structural<RemoveFromListNodeConfig>
  | Structural<FilterNodeConfig>
  | Structural<WebhookNodeConfig>
  | Structural<ListStatusBranchNodeConfig>

interface NodeConfigPanelProps {
  selectedNode: Node<AutomationNodeData> | null
  onNodeUpdate: (nodeId: string, data: Partial<AutomationNodeData>) => void
  workspaceId: string
  onClose?: () => void
}

export const NodeConfigPanel: React.FC<NodeConfigPanelProps> = ({
  selectedNode,
  onNodeUpdate,
  workspaceId,
  onClose
}) => {
  const { t } = useLingui()
  const { workspace } = useAutomation()

  if (!selectedNode) {
    return null
  }

  const { nodeType, config } = selectedNode.data
  const nodeLabel = (() => {
    switch (nodeType) {
      case 'trigger': return t`Trigger`
      case 'delay': return t`Delay`
      case 'email': return t`Email`
      case 'sms': return t`SMS`
      case 'push': return t`Push`
      case 'ab_test': return t`A/B Test`
      case 'add_to_list': return t`Add to List`
      case 'remove_from_list': return t`Remove from List`
      case 'filter': return t`Filter`
      case 'webhook': return t`Webhook`
      case 'list_status_branch': return t`List Status`
      default: return selectedNode.data.label
    }
  })()

  const writeConfig = (newConfig: Record<string, unknown>) => {
    onNodeUpdate(selectedNode.id, {
      ...selectedNode.data,
      config: newConfig
    })
  }

  const handleConfigChange = (newConfig: NodeConfigUpdate) => {
    writeConfig(newConfig)
  }

  const renderConfigForm = () => {
    switch (nodeType) {
      case 'trigger':
        return (
          <TriggerConfigForm
            config={config as Structural<TriggerConfig>}
            onChange={handleConfigChange}
            workspaceId={workspaceId}
            workspace={workspace}
          />
        )
      case 'delay':
        return (
          <DelayConfigForm
            config={config as Structural<DelayNodeConfig>}
            onChange={handleConfigChange}
          />
        )
      case 'email':
        return (
          <EmailConfigForm
            config={config as Structural<EmailNodeConfig>}
            onChange={handleConfigChange}
            workspaceId={workspaceId}
            workspace={workspace}
          />
        )
      case 'sms':
      case 'push':
        return (
          <ChannelConfigForm
            nodeType={nodeType}
            config={config as Structural<ChannelNodeConfig>}
            onChange={handleConfigChange}
            workspaceId={workspaceId}
            workspace={workspace}
          />
        )
      case 'ab_test':
        return (
          <ABTestConfigForm
            config={config as Structural<ABTestNodeConfig>}
            onChange={handleConfigChange}
          />
        )
      case 'add_to_list':
        return (
          <AddToListConfigForm
            config={config as Structural<AddToListNodeConfig>}
            onChange={handleConfigChange}
          />
        )
      case 'remove_from_list':
        return (
          <RemoveFromListConfigForm
            config={config as Structural<RemoveFromListNodeConfig>}
            onChange={handleConfigChange}
          />
        )
      case 'filter':
        return (
          <FilterConfigForm
            config={config as Structural<FilterNodeConfig>}
            onChange={handleConfigChange}
          />
        )
      case 'webhook':
        return (
          <WebhookConfigForm
            config={config as Structural<WebhookNodeConfig>}
            onChange={handleConfigChange}
          />
        )
      case 'list_status_branch':
        return (
          <ListStatusBranchConfigForm
            config={config as Structural<ListStatusBranchNodeConfig>}
            onChange={handleConfigChange}
          />
        )
      default:
        return (
          <Empty
            description={t`Configuration for ${nodeType} is not available in Phase 2`}
            image={Empty.PRESENTED_IMAGE_SIMPLE}
          />
        )
    }
  }

  return (
    <div className="bg-white h-full flex flex-col">
      <div className="p-3 border-b border-gray-200 flex items-center justify-between flex-shrink-0">
        <Title level={5} style={{ margin: 0, fontSize: '14px' }}>
          {t`Configure ${nodeLabel}`}
        </Title>
        {onClose && (
          <button
            type="button"
            aria-label={t`Close node configuration`}
            onClick={onClose}
            className="p-1 hover:bg-gray-100 rounded text-gray-500 hover:text-gray-700 cursor-pointer"
          >
            <X size={16} />
          </button>
        )}
      </div>
      {/* key by node id so switching between same-type nodes remounts the form
          and resets any internal form state (defense-in-depth for stale config) */}
      <div key={selectedNode.id} className="p-3 overflow-y-auto flex-1">
        {/* Every node type takes a description, so it is edited here rather than repeated in each
            of the per-type forms below. It is stored in the node's config like any other setting. */}
        <Form layout="vertical" className="nodrag">
          {/* htmlFor/id by hand: a Form.Item with no `name` generates no field id, so without
              these the label is not associated with the input for assistive tech. */}
          <Form.Item label={t`Description`} htmlFor="node-description">
            <Input
              id="node-description"
              value={(config as { description?: string }).description || ''}
              // undefined rather than '': clearing the box should drop the key instead of storing a
              // blank. Emptiness is judged on the trimmed value — a description of only spaces
              // renders as nothing on the canvas, so persisting one would be invisible dead weight
              // — while the value itself is stored as typed.
              onChange={(e) =>
                writeConfig({
                  ...config,
                  description: e.target.value.trim() ? e.target.value : undefined
                })
              }
              placeholder={t`e.g., Welcome — day 1`}
              maxLength={100}
            />
          </Form.Item>
        </Form>
        {renderConfigForm()}
      </div>
    </div>
  )
}
