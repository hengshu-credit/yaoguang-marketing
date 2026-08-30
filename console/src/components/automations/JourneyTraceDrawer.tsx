import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Alert, Button, Collapse, Descriptions, Drawer, Empty, Result, Space, Spin, Tag, Timeline, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { automationApi, type JourneyTrace } from '../../services/api/automation'
import { deliveryExplanation, deliveryStatusLabels } from '../delivery/deliveryPresentation'

interface JourneyTraceDrawerProps {
  workspaceId: string
  journeyInstanceId?: string
  open: boolean
  onClose: () => void
  onOpenCustomer?: (customerId: string) => void
  onFixNode?: (automationId: string, nodeId: string) => void
}

interface TraceMoment {
  key: string
  occurredAt: string
  title: string
  reason?: string
  status?: string
  nodeId?: string
  delivery?: JourneyTrace['deliveries'][number]
}

export function JourneyTraceDrawer({
  workspaceId,
  journeyInstanceId,
  open,
  onClose,
  onOpenCustomer,
  onFixNode
}: JourneyTraceDrawerProps) {
  const { t } = useLingui()
  const query = useQuery({
    queryKey: ['journey-trace', workspaceId, journeyInstanceId],
    queryFn: () => automationApi.getJourneyTrace(workspaceId, journeyInstanceId!),
    enabled: open && Boolean(journeyInstanceId)
  })
  const trace = query.data
  const moments = useMemo<TraceMoment[]>(() => {
    if (!trace) return []
    return [
      ...trace.entry_decisions.map((decision) => ({
        key: `decision-${decision.id}`,
        occurredAt: decision.decided_at,
        title: decision.decision === 'enrolled' ? t`Customer entered the journey` : t`Journey entry was not allowed`,
        reason: decision.reason,
        status: decision.decision
      })),
      ...trace.events.map((event) => ({
        key: `event-${event.id}`,
        occurredAt: event.occurred_at,
        title: event.event_type,
        reason: event.reason,
        status: event.status,
        nodeId: event.node_id
      })),
      ...trace.deliveries.map((delivery) => ({
        key: `delivery-${delivery.intent.id}`,
        occurredAt: delivery.intent.created_at,
        title: `${delivery.intent.channel.toUpperCase()} · ${deliveryStatusLabels[delivery.intent.status]}`,
        reason: deliveryExplanation(delivery.intent),
        status: delivery.intent.status,
        nodeId: delivery.intent.node_or_phase,
        delivery
      }))
    ].sort((left, right) => new Date(left.occurredAt).getTime() - new Date(right.occurredAt).getTime())
  }, [trace, t])

  return (
    <Drawer
      title={t`Journey trace`}
      open={open}
      onClose={onClose}
      size="large"
      styles={{ wrapper: { maxWidth: '100vw' } }}
      destroyOnHidden
    >
      {query.isLoading && <div style={{ display: 'grid', placeItems: 'center', minHeight: 200 }}><Spin /></div>}
      {query.isError && <Result status="error" title={t`Unable to load journey trace`} subTitle={query.error instanceof Error ? query.error.message : t`Please try again`} extra={<Button onClick={() => query.refetch()}>{t`Retry`}</Button>} />}
      {trace && (
        <Space orientation="vertical" size="large" style={{ width: '100%' }}>
          <Descriptions bordered size="small" column={1}>
            <Descriptions.Item label={t`Journey`}>{trace.instance.automation_name}</Descriptions.Item>
            <Descriptions.Item label={t`Instance trace ID`}><Typography.Text copyable>{trace.instance.id}</Typography.Text></Descriptions.Item>
            <Descriptions.Item label={t`Customer`}>
              <Button type="link" style={{ padding: 0 }} onClick={() => onOpenCustomer?.(trace.instance.customer_id)}>{trace.instance.customer_no}</Button>
            </Descriptions.Item>
            <Descriptions.Item label={t`Status`}><Tag>{trace.instance.status}</Tag></Descriptions.Item>
            {trace.instance.origin_event_id && <Descriptions.Item label={t`Origin event`}><Typography.Text copyable>{trace.instance.origin_event_id}</Typography.Text></Descriptions.Item>}
          </Descriptions>

          {trace.instance.status === 'active' && trace.instance.next_scheduled_at && (
            <Alert type="info" showIcon title={t`This journey is waiting by design`} description={t`It will continue at ${new Date(trace.instance.next_scheduled_at).toLocaleString()}.`} />
          )}

          {moments.length === 0 ? <Empty description={t`No trace events yet`} /> : (
            <Timeline
              items={moments.map((moment) => ({
                color: moment.status === 'failed' || moment.status === 'terminal_failed' ? 'red' : moment.status === 'suppressed' ? 'orange' : 'blue',
                content: (
                  <Space orientation="vertical" size={2} style={{ width: '100%' }}>
                    <Space wrap><Typography.Text strong>{moment.title}</Typography.Text>{moment.status && <Tag>{moment.status}</Tag>}</Space>
                    {moment.reason && <Typography.Text>{moment.reason}</Typography.Text>}
                    <Typography.Text type="secondary">{new Date(moment.occurredAt).toLocaleString()}</Typography.Text>
                    {moment.nodeId && (onFixNode ? (
                      <Button size="small" onClick={() => onFixNode(trace.instance.automation_id, moment.nodeId!)}>{t`Fix this node`}</Button>
                    ) : (
                      <Button
                        size="small"
                        href={`/console/workspace/${encodeURIComponent(workspaceId)}/automations?automation_id=${encodeURIComponent(trace.instance.automation_id)}&node_id=${encodeURIComponent(moment.nodeId)}`}
                      >
                        {t`Fix this node`}
                      </Button>
                    ))}
                  </Space>
                )
              }))}
            />
          )}

          <Collapse
            ghost
            items={[{
              key: 'diagnostics',
              label: t`Diagnostic data`,
              children: <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{JSON.stringify(trace, null, 2)}</pre>
            }]}
          />
        </Space>
      )}
    </Drawer>
  )
}
