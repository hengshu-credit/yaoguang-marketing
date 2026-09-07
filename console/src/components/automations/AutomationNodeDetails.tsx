import { Descriptions, Empty, Input, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { AutomationNode } from '../../services/api/automation'

// Read saved values directly, including unknown fields, so viewing a live task
// cannot apply editor defaults or silently hide newer node configuration.
export function AutomationNodeDetails({ node, listNames = {} }: {
  node: AutomationNode
  listNames?: Record<string, string>
}) {
  const { t } = useLingui()
  const labels: Record<string, string> = {
    description: t`Description`, duration: t`Duration`, unit: t`Unit`,
    template_id: t`Template`, template_version: t`Version`, integration_id: t`Integration`,
    list_id: t`List`, status: t`Status`, url: t`Webhook URL`, secret: t`Authorization Secret`,
    event_kind: t`Trigger`, frequency: t`Frequency`, conditions: t`Conditions`,
    updated_fields: t`Updated Fields`, segment_id: t`Segment`, custom_event_name: t`Event Name`,
    tag: t`Tag`, paths: t`Paths`, variants: t`Variants`, name: t`Name`, weight: t`Weight`,
    next_node_id: t`Next node`, continue_node_id: t`Continue node`, exit_node_id: t`Exit node`,
    language: t`Language`, data: t`Data`, entry_guard: t`Entry guard`
  }
  const renderValue = (key: string, value: unknown): React.ReactNode => {
    if (value === null || value === undefined || value === '') return <Typography.Text type="secondary">—</Typography.Text>
    if (key === 'secret' && typeof value === 'string') return <Input.Password readOnly value={value} aria-label={t`Authorization Secret`} />
    if (Array.isArray(value)) {
      if (!value.length) return '[]'
      return <ol style={{ margin: 0, paddingLeft: 20 }}>{value.map((item, i) => <li key={i}>{renderValue('', item)}</li>)}</ol>
    }
    if (typeof value === 'object') {
      return <Descriptions size="small" column={1} layout="vertical" items={Object.entries(value).map(([field, child]) => ({
        key: field, label: labels[field] || field, children: renderValue(field, child)
      }))} />
    }
    if (key === 'list_id' && listNames[String(value)]) return <>{listNames[String(value)]} <Typography.Text type="secondary">({String(value)})</Typography.Text></>
    const units: Record<string, string> = { minutes: t`Minutes`, hours: t`Hours`, days: t`Days`, once: t`Once per contact`, every_time: t`Every time` }
    if ((key === 'unit' || key === 'frequency') && units[String(value)]) return units[String(value)]
    return <span style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{String(value)}</span>
  }
  const fields = Object.entries(node.config ?? {}).filter(([, value]) => value !== undefined)
  return <>
    <Typography.Paragraph type="secondary">{t`Read-only`}</Typography.Paragraph>
    <Descriptions size="small" column={1} items={[
      { key: 'id', label: t`Node ID`, children: node.id },
      { key: 'type', label: t`Type`, children: node.type },
      ...(node.next_node_id ? [{ key: 'next', label: t`Next node`, children: node.next_node_id }] : [])
    ]} />
    {fields.length ? <Descriptions className="mt-4" size="small" column={1} layout="vertical" bordered items={fields.map(([key, value]) => ({
      key, label: labels[key] || key, children: renderValue(key, value)
    }))} /> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t`No configuration`} />}
  </>
}
