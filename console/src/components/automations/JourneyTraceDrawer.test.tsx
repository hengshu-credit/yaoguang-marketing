import '../../__tests__/resizeObserverMock'
import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nProvider } from '@lingui/react'
import { i18n } from '../../i18n'
import { JourneyTraceDrawer } from './JourneyTraceDrawer'

const { getJourneyTrace } = vi.hoisted(() => ({ getJourneyTrace: vi.fn(async () => ({
  instance: {
    id: 'instance-1',
    automation_id: 'automation-1',
    automation_name: '授信通过关怀',
    customer_id: 'customer-1',
    customer_no: 'U0001',
    status: 'active',
    current_node_id: 'delay-1',
    waiting_reason: '等待 2 天后发送短信',
    started_at: '2026-08-30T08:00:00Z',
    entry_decision: 'enrolled',
    frequency: 'every_time'
  },
  entry_decisions: [
    {
      id: 'decision-1',
      automation_id: 'automation-1',
      customer_id: 'customer-1',
      decision: 'enrolled',
      reason: '事件满足进入条件',
      decided_at: '2026-08-30T08:00:00Z'
    }
  ],
  events: [
    {
      id: 'trace-event-1',
      node_id: 'delay-1',
      event_type: 'node.waiting',
      status: 'waiting',
      reason: '等待 2 天后发送短信',
      occurred_at: '2026-08-30T08:01:00Z'
    }
  ],
  deliveries: [
    {
      intent: {
        id: 'delivery-1',
        effect_key: 'effect-1',
        source_type: 'automation',
        source_id: 'automation-1',
        source_version: '1',
        customer_id: 'customer-1',
        channel: 'sms',
        status: 'suppressed',
        suppression_reason: 'frequency_policy',
        created_at: '2026-08-30T08:02:00Z',
        updated_at: '2026-08-30T08:02:00Z'
      }
    }
  ]
})) }))

vi.mock('../../services/api/automation', () => ({ automationApi: { getJourneyTrace } }))

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>{children}</I18nProvider>
    </QueryClientProvider>
  )
}

describe('JourneyTraceDrawer', () => {
  it('explains waiting and suppression without exposing queue implementation terms', async () => {
    const onFixNode = vi.fn()
    render(
      <JourneyTraceDrawer
        workspaceId="ws1"
        journeyInstanceId="instance-1"
        open
        onClose={vi.fn()}
        onOpenCustomer={vi.fn()}
        onFixNode={onFixNode}
      />,
      { wrapper }
    )

    expect(await screen.findByText('等待 2 天后发送短信')).toBeInTheDocument()
    expect(screen.getByText('Blocked by message frequency policy')).toBeInTheDocument()
    expect(screen.queryByText(/queue|lease/i)).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Fix this node' }))
    expect(onFixNode).toHaveBeenCalledWith('automation-1', 'delay-1')
    fireEvent.click(screen.getByText('Diagnostic data'))
    expect(await screen.findByText(/trace-event-1/)).toBeInTheDocument()
  })
})
