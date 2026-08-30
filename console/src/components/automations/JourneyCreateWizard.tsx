import { useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import type { Edge } from '@xyflow/react'
import {
  Alert,
  Button,
  Card,
  Collapse,
  InputNumber,
  Space,
  Spin,
  Steps,
  Switch,
  Typography
} from 'antd'
import {
  BellOutlined,
  CalendarOutlined,
  GiftOutlined,
  NotificationOutlined,
  RetweetOutlined,
  ThunderboltOutlined
} from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import type { AutomationFlowNode } from './utils/flowConverter'
import type { JourneyEntryGuard, TimelineTriggerConfig } from '../../services/api/automation'
import type { FrequencyPolicy, SaveFrequencyPolicyRequest } from '../../services/api/frequency_policy'
import { frequencyPolicyApi } from '../../services/api/frequency_policy'
import { useAutomation } from './context'
import { TriggerConfigForm, type TriggerConfig } from './config/TriggerConfigForm'
import { AutomationFlowEditor } from './AutomationFlowEditor'
import { OptionSelector } from '../ui/OptionSelector'
import { FrequencyPolicyForm } from '../frequency/FrequencyPolicyForm'
import { JourneyPreflightPanel } from './JourneyPreflightPanel'

const { Paragraph, Text, Title } = Typography
const HOUR_IN_NANOSECONDS = 60 * 60 * 1_000_000_000

type JourneyBlueprintKey =
  | 'event_welcome'
  | 'scheduled_reminder'
  | 'churn_winback'
  | 'birthday_care'
  | 'audience_notification'
  | 'blank'

interface JourneyBlueprint {
  key: JourneyBlueprintKey
  title: string
  description: string
  icon: ReactNode
  suggestedName: string
  trigger?: Partial<TimelineTriggerConfig>
}

interface JourneyCreateWizardProps {
  workspaceId: string
  automationId: string
  onActivated?: () => void
  onFixIssue?: (nodeId?: string) => void
}

interface StoredWizardDraft {
  step: number
  blueprint?: JourneyBlueprintKey
  name?: string
  nodes?: AutomationFlowNode[]
  edges?: Edge[]
}

export function JourneyCreateWizard({
  workspaceId,
  automationId,
  onActivated,
  onFixIssue
}: JourneyCreateWizardProps) {
  const { t } = useLingui()
  const {
    name,
    setName,
    automation,
    workspace,
    canvasState,
    markAsChanged,
    focusNode,
    save,
    isSaving
  } = useAutomation()
  const [step, setStep] = useState(0)
  const [blueprintKey, setBlueprintKey] = useState<JourneyBlueprintKey>()
  const [frequencyPolicy, setFrequencyPolicy] = useState<FrequencyPolicy>()
  const [frequencySaving, setFrequencySaving] = useState(false)
  const [preparingPreflight, setPreparingPreflight] = useState(false)
  const restoredRef = useRef(false)
  const storageKey = `yaoguang:journey-draft:${workspaceId}:${automationId}`

  const blueprints = useMemo<JourneyBlueprint[]>(
    () => [
      {
        key: 'event_welcome',
        title: t`Event welcome`,
        description: t`Welcome a customer after profile creation or a business signup event.`,
        icon: <ThunderboltOutlined />,
        suggestedName: t`New customer welcome journey`,
        trigger: { event_kind: 'contact.created', frequency: 'once' }
      },
      {
        key: 'scheduled_reminder',
        title: t`Scheduled reminder`,
        description: t`Receive a scheduled_reminder event from your scheduler and continue the journey.`,
        icon: <CalendarOutlined />,
        suggestedName: t`Scheduled reminder journey`,
        trigger: {
          event_kind: 'custom_event',
          custom_event_name: 'scheduled_reminder',
          frequency: 'every_time'
        }
      },
      {
        key: 'churn_winback',
        title: t`Churn win-back`,
        description: t`Re-engage customers when the business reports a churn-risk event.`,
        icon: <RetweetOutlined />,
        suggestedName: t`Churn win-back journey`,
        trigger: {
          event_kind: 'custom_event',
          custom_event_name: 'customer_churn_risk',
          frequency: 'every_time'
        }
      },
      {
        key: 'birthday_care',
        title: t`Birthday care`,
        description: t`Start when the business scheduler reports a customer birthday event.`,
        icon: <GiftOutlined />,
        suggestedName: t`Birthday care journey`,
        trigger: {
          event_kind: 'custom_event',
          custom_event_name: 'customer_birthday',
          frequency: 'every_time'
        }
      },
      {
        key: 'audience_notification',
        title: t`Audience notification`,
        description: t`Notify customers whenever they join the selected audience segment.`,
        icon: <NotificationOutlined />,
        suggestedName: t`Audience notification journey`,
        trigger: { event_kind: 'segment.joined', frequency: 'once' }
      },
      {
        key: 'blank',
        title: t`Blank journey`,
        description: t`Start with an empty trigger and build the flow yourself.`,
        icon: <BellOutlined />,
        suggestedName: t`Untitled journey`,
        trigger: { frequency: 'once' }
      }
    ],
    [t]
  )

  const triggerNode = canvasState.nodes.find((node) => node.data.nodeType === 'trigger')
  const triggerConfig = (triggerNode?.data.config ?? { frequency: 'once' }) as TriggerConfig

  const updateTrigger = (config: TriggerConfig) => {
    if (!triggerNode) return
    canvasState.setNodes((nodes) =>
      nodes.map((node) =>
        node.id === triggerNode.id
          ? { ...node, data: { ...node.data, config: { ...config } } }
          : node
      )
    )
    markAsChanged()
  }

  const selectBlueprint = (blueprint: JourneyBlueprint) => {
    setBlueprintKey(blueprint.key)
    if (!name.trim()) setName(blueprint.suggestedName)
    updateTrigger({ ...triggerConfig, ...blueprint.trigger })
  }

  useEffect(() => {
    if (restoredRef.current || canvasState.nodes.length === 0) return
    restoredRef.current = true
    const raw = localStorage.getItem(storageKey)
    if (!raw) return
    try {
      const draft = JSON.parse(raw) as StoredWizardDraft
      setStep(Math.min(Math.max(draft.step ?? 0, 0), 4))
      setBlueprintKey(draft.blueprint)
      if (draft.name) setName(draft.name)
      if (draft.nodes?.length) canvasState.setNodes(draft.nodes)
      if (draft.edges) canvasState.setEdges(draft.edges)
    } catch {
      localStorage.removeItem(storageKey)
    }
  }, [canvasState, setName, storageKey])

  useEffect(() => {
    if (!restoredRef.current || canvasState.nodes.length === 0 || automation) return
    const draft: StoredWizardDraft = {
      step,
      blueprint: blueprintKey,
      name,
      nodes: canvasState.nodes as AutomationFlowNode[],
      edges: canvasState.edges
    }
    localStorage.setItem(storageKey, JSON.stringify(draft))
  }, [automation, blueprintKey, canvasState.edges, canvasState.nodes, name, step, storageKey])

  useEffect(() => {
    if (step !== 4) return
    let active = true
    void frequencyPolicyApi
      .list(workspaceId)
      .then(({ policies }) => {
        if (active) {
          setFrequencyPolicy(
            policies.find((policy) => policy.scope === 'trigger' && policy.scope_ref === automationId)
          )
        }
      })
      .catch(() => undefined)
    return () => {
      active = false
    }
  }, [automationId, step, workspaceId])

  const saveFrequencyPolicy = async (
    request: Omit<SaveFrequencyPolicyRequest, 'workspace_id'>
  ) => {
    setFrequencySaving(true)
    try {
      const response = await frequencyPolicyApi.save({
        ...request,
        workspace_id: workspaceId,
        scope_ref: automationId
      })
      setFrequencyPolicy(response.policy)
    } finally {
      setFrequencySaving(false)
    }
  }

  const goNext = async () => {
    if (step === 0 && !blueprintKey) return
    if (step < 4) {
      setStep(step + 1)
      return
    }

    setPreparingPreflight(true)
    try {
      const saved = await save({ close: false })
      if (saved) {
        localStorage.removeItem(storageKey)
        setStep(5)
      }
    } finally {
      setPreparingPreflight(false)
    }
  }

  const entryGuard: JourneyEntryGuard = triggerConfig.entry_guard ?? { enabled: false }
  const updateEntryGuard = (guard: JourneyEntryGuard) =>
    updateTrigger({ ...triggerConfig, entry_guard: guard })

  const content = (() => {
    switch (step) {
      case 0:
        return (
          <div>
            <Title level={4}>{t`Choose a goal and starting template`}</Title>
            <Paragraph type="secondary">
              {t`Templates provide understandable defaults. Every trigger and flow node remains editable.`}
            </Paragraph>
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
              {blueprints.map((blueprint) => (
                <button
                  key={blueprint.key}
                  type="button"
                  aria-pressed={blueprintKey === blueprint.key}
                  className={`rounded border bg-white p-0 text-left ${
                    blueprintKey === blueprint.key
                      ? 'border-indigo-500 ring-2 ring-indigo-100'
                      : 'border-gray-200'
                  }`}
                  onClick={() => selectBlueprint(blueprint)}
                >
                  <Card variant="borderless">
                    <Space align="start">
                      <span className="text-xl text-indigo-500">{blueprint.icon}</span>
                      <div>
                        <Text strong>{blueprint.title}</Text>
                        <Paragraph type="secondary" style={{ margin: '4px 0 0' }}>
                          {blueprint.description}
                        </Paragraph>
                      </div>
                    </Space>
                  </Card>
                </button>
              ))}
            </div>
          </div>
        )
      case 1:
        return (
          <div className="mx-auto max-w-3xl">
            <Title level={4}>{t`Configure the trigger`}</Title>
            <TriggerConfigForm
              config={triggerConfig}
              onChange={updateTrigger}
              workspaceId={workspaceId}
              workspace={workspace}
              showFrequency={false}
            />
          </div>
        )
      case 2:
        return (
          <div className="mx-auto max-w-3xl">
            <Title level={4}>{t`Entry frequency and protection`}</Title>
            <Paragraph type="secondary">
              {t`Entry frequency decides when a customer creates a Journey instance.`}
            </Paragraph>
            <OptionSelector<'once' | 'every_time'>
              value={triggerConfig.frequency ?? 'once'}
              onChange={(frequency) => updateTrigger({ ...triggerConfig, frequency })}
              options={[
                {
                  value: 'once',
                  label: t`Once per contact`,
                  description: t`Each contact enters the automation only once`
                },
                {
                  value: 'every_time',
                  label: t`Every time`,
                  description: t`Contact re-enters each time the event occurs`
                }
              ]}
            />
            <Collapse
              className="mt-4"
              items={[
                {
                  key: 'entry-guard',
                  label: entryGuard.enabled
                    ? t`Entry protection: On`
                    : t`Entry protection: Off`,
                  extra: (
                    <Switch
                      aria-label={t`Entry protection`}
                      checked={entryGuard.enabled}
                      onClick={(_, event) => event.stopPropagation()}
                      onChange={(enabled) =>
                        updateEntryGuard({
                          ...entryGuard,
                          enabled,
                          cooldown: enabled
                            ? entryGuard.cooldown || HOUR_IN_NANOSECONDS
                            : entryGuard.cooldown
                        })
                      }
                    />
                  ),
                  children: (
                    <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
                      <Space wrap align="end">
                        <label>
                          <Text>{t`Cooldown (hours)`}</Text>
                          <InputNumber
                            className="mt-1 block"
                            min={0}
                            value={Math.round((entryGuard.cooldown ?? 0) / HOUR_IN_NANOSECONDS)}
                            onChange={(hours) =>
                              updateEntryGuard({
                                ...entryGuard,
                                cooldown: Math.max(0, hours ?? 0) * HOUR_IN_NANOSECONDS
                              })
                            }
                          />
                        </label>
                        <label>
                          <Text>{t`Maximum concurrent instances`}</Text>
                          <InputNumber
                            className="mt-1 block"
                            min={0}
                            value={entryGuard.max_concurrent ?? 0}
                            onChange={(maxConcurrent) =>
                              updateEntryGuard({
                                ...entryGuard,
                                max_concurrent: Math.max(0, maxConcurrent ?? 0)
                              })
                            }
                          />
                        </label>
                      </Space>
                    </Space>
                  )
                }
              ]}
            />
            <Alert
              className="mt-3"
              type="info"
              showIcon
              title={t`Entry protection only limits Journey entry and is not message frequency control.`}
            />
          </div>
        )
      case 3:
        return (
          <div style={{ height: 'calc(100vh - 300px)', minHeight: 420 }}>
            <AutomationFlowEditor />
          </div>
        )
      case 4:
        return (
          <div className="mx-auto max-w-3xl">
            <Title level={4}>{t`Message frequency control`}</Title>
            <Alert
              className="mb-4"
              type="info"
              showIcon
              title={t`This policy does not change whether a customer enters the Journey.`}
              description={t`It only decides whether each message created by this Journey may be delivered.`}
            />
            <FrequencyPolicyForm
              scope="trigger"
              policy={frequencyPolicy}
              defaultScopeRef={automationId}
              fixedScopeRef={automationId}
              saving={frequencySaving}
              onSave={saveFrequencyPolicy}
            />
          </div>
        )
      default:
        return (
          <div className="mx-auto max-w-4xl">
            <JourneyPreflightPanel
              workspaceId={workspaceId}
              automationId={automationId}
              onFixIssue={(issue) => {
                if (issue.node_id) {
                  focusNode(issue.node_id)
                  setStep(3)
                } else {
                  setStep(1)
                }
                onFixIssue?.(issue.node_id)
              }}
              onActivated={() => {
                localStorage.removeItem(storageKey)
                onActivated?.()
              }}
            />
          </div>
        )
    }
  })()

  const stepItems = [
    t`Goal and template`,
    t`Trigger`,
    t`Entry`,
    t`Flow content`,
    t`Message frequency`,
    t`Activation check`
  ].map((title) => ({ title }))

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="border-b border-gray-200 bg-white px-6 py-4">
        <Steps current={step} items={stepItems} responsive />
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-6">{content}</div>
      {step < 5 && (
        <div className="flex items-center justify-between border-t border-gray-200 bg-white px-6 py-3">
          <Text type="secondary">{t`Your draft is kept when you close this wizard.`}</Text>
          <Space>
            <Button disabled={step === 0} onClick={() => setStep(Math.max(0, step - 1))}>
              {t`Back`}
            </Button>
            <Button
              type="primary"
              disabled={step === 0 && !blueprintKey}
              loading={preparingPreflight || isSaving}
              onClick={() => void goNext()}
            >
              {step === 4 ? t`Save draft and check` : t`Next`}
            </Button>
          </Space>
        </div>
      )}
      {preparingPreflight && <Spin fullscreen description={t`Saving journey draft...`} />}
    </div>
  )
}
