import '../../__tests__/resizeObserverMock'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nProvider } from '@lingui/react'
import { i18n } from '../../i18n'
import { CustomerDrawer } from './CustomerDrawer'

const { timelineList, journeyList, deliveryList } = vi.hoisted(() => ({
  timelineList: vi.fn(),
  journeyList: vi.fn(),
  deliveryList: vi.fn()
}))

vi.mock('../../services/api/customer', async () => {
  const actual = await vi.importActual('../../services/api/customer')
  return {
    ...actual,
    customerApi: {
      get: vi.fn(async () => ({
        customer_id: 'customer-1',
        customer_no: 'U0001202608301000000811111111111141118111111111111111',
        external_user_id: 'crm-42',
        version: 3,
        profile: { customer_id: 'customer-1', status: 'active', attributes: { tier: 'gold' } },
        identities: [
          {
            id: 'identity-1',
            type: 'email',
            display_hint: 'a***@example.com',
            value_ciphertext: 'must-never-render',
            verified: true,
            primary: true,
            enabled: true
          }
        ],
        tags: ['vip'],
        list_memberships: [{ list_id: 'newsletter', status: 'active' }],
        audience_memberships: [{
          audience_id: 'audience-1',
          name: 'High value customers',
          kind: 'dynamic',
          audience_version: 4,
          build_id: 'build-1',
          created_at: '2026-08-30T00:00:00Z'
        }],
        consents: [
          {
            id: 'consent-1',
            purpose: 'marketing',
            channel: 'email',
            status: 'granted',
            valid_from: '2026-08-30T00:00:00Z'
          }
        ],
        created_at: '2026-08-30T00:00:00Z',
        updated_at: '2026-08-30T00:00:00Z'
      }))
    }
  }
})

vi.mock('../../services/api/list', () => ({
  listsApi: { list: vi.fn(async () => ({ lists: [{ id: 'newsletter', name: 'Newsletter' }] })) }
}))

vi.mock('../../services/api/contact_timeline', () => ({
  contactTimelineApi: { list: timelineList }
}))

vi.mock('../../services/api/automation', () => ({
  automationApi: { listJourneyInstances: journeyList }
}))

vi.mock('../../services/api/delivery', () => ({
  deliveryApi: { list: deliveryList }
}))

vi.mock('../automations/JourneyTraceDrawer', () => ({
  JourneyTraceDrawer: ({ open, journeyInstanceId }: { open: boolean; journeyInstanceId?: string }) =>
    open ? <div>trace:{journeyInstanceId}</div> : null
}))

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>{children}</I18nProvider>
    </QueryClientProvider>
  )
}

describe('CustomerDrawer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    timelineList.mockResolvedValue({
      timeline: [
        {
          id: 'event-1',
          customer_id: 'customer-1',
          operation: 'insert',
          entity_type: 'custom_event',
          kind: 'loan.approved',
          changes: {},
          created_at: '2026-08-30T08:00:00Z',
          db_created_at: '2026-08-30T08:00:00Z'
        }
      ]
    })
    journeyList.mockResolvedValue({
      instances: [
        {
          id: 'instance-1',
          automation_id: 'automation-1',
          automation_name: '授信通过关怀',
          customer_id: 'customer-1',
          customer_no: 'U0001',
          status: 'active',
          frequency: 'every_time',
          entry_decision: 'enrolled',
          waiting_reason: '等待 2 天后发送短信',
          started_at: '2026-08-30T08:00:00Z'
        }
      ],
      total: 1
    })
    deliveryList.mockResolvedValue({
      deliveries: [
        {
          id: 'delivery-1',
          effect_key: 'effect-1',
          source_type: 'automation',
          source_id: 'automation-1',
          source_version: '1',
          customer_id: 'customer-1',
          channel: 'sms',
          status: 'suppressed',
          suppression_reason: 'frequency_policy',
          created_at: '2026-08-30T08:00:00Z',
          updated_at: '2026-08-30T08:00:00Z'
        }
      ],
      total: 1
    })
  })

  it('renders the Customer 360 foundation with masked identities and consent', async () => {
    render(
      <CustomerDrawer
        workspaceId="ws1"
        customerId="customer-1"
        open
        onClose={vi.fn()}
      />,
      { wrapper }
    )

    expect((await screen.findAllByText('crm-42')).length).toBeGreaterThan(0)
    expect(screen.getByText('vip')).toBeInTheDocument()
    expect(screen.getAllByText(/a\*\*\*@example\.com/)).toHaveLength(2)
    expect(screen.getByText(/marketing \/ email/)).toBeInTheDocument()
    expect(await screen.findByText(/Newsletter · active/)).toBeInTheDocument()
    expect(screen.getByText(/High value customers · v4/)).toBeInTheDocument()
    expect(screen.queryByText('must-never-render')).not.toBeInTheDocument()
    expect(screen.getByTestId('customer-360-layout')).toHaveClass('md:flex-row')
  })

  it('loads timeline, journeys and deliveries only when their Customer 360 tabs are opened', async () => {
    render(
      <CustomerDrawer workspaceId="ws1" customerId="customer-1" open onClose={vi.fn()} />,
      { wrapper }
    )

    await screen.findAllByText('crm-42')
    expect(timelineList).not.toHaveBeenCalled()
    expect(journeyList).not.toHaveBeenCalled()
    expect(deliveryList).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('tab', { name: 'Activity timeline' }))
    expect(await screen.findByText('loan.approved')).toBeInTheDocument()
    expect(timelineList).toHaveBeenCalledWith(
      expect.objectContaining({ workspace_id: 'ws1', customer_id: 'customer-1' })
    )

    fireEvent.click(screen.getByRole('tab', { name: 'Journeys' }))
    expect(await screen.findByText('授信通过关怀')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'View trace' }))
    expect(screen.getByText('trace:instance-1')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('tab', { name: 'Deliveries' }))
    expect(await screen.findByText('Blocked by message frequency policy')).toBeInTheDocument()
    await waitFor(() => {
      expect(deliveryList).toHaveBeenCalledWith(
        expect.objectContaining({ workspace_id: 'ws1', customer_id: 'customer-1' })
      )
    })
  })
})
