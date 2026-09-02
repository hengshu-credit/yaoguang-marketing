import '../../__tests__/resizeObserverMock'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nProvider } from '@lingui/react'
import { App } from 'antd'
import { i18n } from '../../i18n'
import { CustomerDrawer } from './CustomerDrawer'

const { timelineList, journeyList, deliveryList, updateListMemberships, audienceListAll, matchCustomer } = vi.hoisted(() => ({
  timelineList: vi.fn(),
  journeyList: vi.fn(),
  deliveryList: vi.fn(),
  updateListMemberships: vi.fn(),
  audienceListAll: vi.fn(),
  matchCustomer: vi.fn()
}))

vi.mock('../../services/api/customer', async () => {
  const actual = await vi.importActual('../../services/api/customer')
  return {
    ...actual,
    customerApi: {
      update: vi.fn(),
      updateListMemberships,
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

vi.mock('../../services/api/marketing', () => ({
  audienceApi: { listAll: audienceListAll, matchCustomer }
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
      <I18nProvider i18n={i18n}><App>{children}</App></I18nProvider>
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
    updateListMemberships.mockResolvedValue({
      request_id: 'request-1', customers: 1, lists: 1, changed: 1, unchanged: 0
    })
    audienceListAll.mockResolvedValue([{
      id: 'audience-1', name: 'High value customers', kind: 'dynamic', active_version: 4
    }])
    matchCustomer.mockResolvedValue({
      audience_id: 'audience-1', name: 'High value customers', kind: 'dynamic',
      audience_version: 4, matches: true
    })
  })

  it('renders the Customer 360 foundation without consent information', async () => {
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
    expect(screen.queryByRole('heading', { name: 'Consent' })).not.toBeInTheDocument()
    expect(screen.queryByText(/marketing \/ email/)).not.toBeInTheDocument()
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

  it('opens list membership adjustment for one customer from Customer 360', async () => {
    render(
      <CustomerDrawer workspaceId="ws1" customerId="customer-1" open onClose={vi.fn()} canWrite />,
      { wrapper }
    )

    expect(await screen.findByText(/Newsletter · active/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Adjust list memberships' }))
    expect(await screen.findByText('Operation')).toBeInTheDocument()
    expect(screen.getByText('This operation applies to 1 selected customers.')).toBeInTheDocument()
  })

  it('calculates every dynamic audience on entry and reveals matches as each result arrives', async () => {
    let resolveTestAudience: (value: unknown) => void = () => undefined
    let resolveDormantAudience: (value: unknown) => void = () => undefined
    audienceListAll.mockResolvedValue([
      { id: 'audience-test', name: 'Test', kind: 'dynamic', active_version: 2 },
      { id: 'audience-dormant', name: 'Dormant', kind: 'dynamic', active_version: 5 },
      { id: 'audience-static', name: 'Static', kind: 'static', active_version: 1 }
    ])
    matchCustomer.mockImplementation((_workspaceId: string, audienceId: string) => new Promise((resolve) => {
      if (audienceId === 'audience-test') resolveTestAudience = resolve
      if (audienceId === 'audience-dormant') resolveDormantAudience = resolve
    }))

    render(
      <CustomerDrawer workspaceId="ws1" customerId="customer-1" open onClose={vi.fn()} />,
      { wrapper }
    )

    expect(await screen.findByText('Calculating dynamic audience memberships (0/2)')).toBeInTheDocument()
    expect(matchCustomer).toHaveBeenCalledTimes(2)
    expect(matchCustomer).not.toHaveBeenCalledWith('ws1', 'audience-static', 'customer-1')

    await act(async () => resolveTestAudience({
      audience_id: 'audience-test', name: 'Test', kind: 'dynamic', audience_version: 2, matches: true
    }))
    expect(await screen.findByText('Test · v2')).toBeInTheDocument()
    expect(screen.getByText('Calculating dynamic audience memberships (1/2)')).toBeInTheDocument()
    expect(screen.queryByText(/Dormant ·/)).not.toBeInTheDocument()

    await act(async () => resolveDormantAudience({
      audience_id: 'audience-dormant', name: 'Dormant', kind: 'dynamic', audience_version: 5, matches: false
    }))
    expect(await screen.findByText('Calculation complete: 1 matching dynamic audience')).toBeInTheDocument()
    expect(screen.queryByText(/Calculating dynamic audience memberships/)).not.toBeInTheDocument()

    const dynamicHeading = screen.getByRole('heading', { name: 'Dynamic audience memberships' })
    const listHeading = screen.getByRole('heading', { name: 'List memberships' })
    expect(dynamicHeading.compareDocumentPosition(listHeading) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('recalculates dynamic audiences whenever Customer 360 is reopened', async () => {
    const { rerender } = render(
      <CustomerDrawer workspaceId="ws1" customerId="customer-1" open onClose={vi.fn()} />,
      { wrapper }
    )
    expect(await screen.findByText('Calculation complete: 1 matching dynamic audience')).toBeInTheDocument()

    rerender(<CustomerDrawer workspaceId="ws1" customerId="customer-1" open={false} onClose={vi.fn()} />)
    rerender(<CustomerDrawer workspaceId="ws1" customerId="customer-1" open onClose={vi.fn()} />)

    await waitFor(() => expect(audienceListAll).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(matchCustomer).toHaveBeenCalledTimes(2))
  })
})
