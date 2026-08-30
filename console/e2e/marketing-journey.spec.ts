import type { Page, Route } from '@playwright/test'
import { test, expect } from './fixtures/auth'

const WORKSPACE_ID = 'test-workspace'
const NOW = '2026-08-30T08:00:00Z'

const json = (route: Route, body: unknown, status = 200) =>
  route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })

async function routeJourneyConsole(page: Page) {
  const automation = {
    id: 'journey-welcome',
    workspace_id: WORKSPACE_ID,
    name: 'New customer welcome',
    status: 'draft',
    list_id: '',
    root_node_id: 'trigger-1',
    trigger: { event_kind: 'custom_event', custom_event_name: 'customer.created', frequency: 'every_time' },
    nodes: [
      { id: 'trigger-1', automation_id: 'journey-welcome', type: 'trigger', config: {}, position: { x: 0, y: 0 }, created_at: NOW }
    ],
    stats: { enrolled: 12, completed: 9, exited: 1, failed: 0 },
    created_at: NOW,
    updated_at: NOW
  }
  const customer = {
    customer_id: '11111111-1111-4111-8111-111111111111',
    customer_no: 'U00012026083016000008aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
    external_user_id: 'core-banking-10001',
    version: 3,
    profile: {
      customer_id: '11111111-1111-4111-8111-111111111111',
      status: 'active',
      language: 'zh-CN',
      timezone: 'Asia/Shanghai',
      attributes: { tier: 'gold' },
      version: 3,
      created_at: NOW,
      updated_at: NOW
    },
    identities: [{ id: 'identity-1', type: 'email', display_hint: 'm***@example.com', verified: true, primary: true, enabled: true, created_at: NOW, updated_at: NOW }],
    tags: ['high-value'],
    list_memberships: [{ list_id: 'vip-list', status: 'active', created_at: NOW, updated_at: NOW }],
    audience_memberships: [{ audience_id: 'audience-1', name: 'High value', kind: 'dynamic', audience_version: 4, build_id: 'build-4', created_at: NOW }],
    consents: [{ id: 'consent-1', purpose: 'marketing', channel: 'email', status: 'granted', valid_from: NOW, created_at: NOW, updated_at: NOW }],
    created_at: NOW,
    updated_at: NOW
  }
  const instance = {
    id: '22222222-2222-4222-8222-222222222222',
    automation_id: automation.id,
    automation_name: automation.name,
    customer_id: customer.customer_id,
    customer_no: customer.customer_no,
    external_user_id: customer.external_user_id,
    contact_email: 'masked@example.com',
    frequency: 'every_time',
    origin_event_id: '33333333-3333-4333-8333-333333333333',
    entry_decision: 'enrolled',
    status: 'active',
    current_node_id: 'message-1',
    waiting_reason: 'Waiting for scheduled send',
    next_scheduled_at: '2026-08-31T08:00:00Z',
    started_at: NOW
  }
  const delivery = {
    id: '44444444-4444-4444-8444-444444444444',
    effect_key: 'a'.repeat(64),
    source_type: 'automation',
    source_id: automation.id,
    source_version: '1',
    customer_id: customer.customer_id,
    channel: 'email',
    node_or_phase: 'message-1',
    occurrence: instance.origin_event_id,
    variant: 'control',
    status: 'suppressed',
    suppression_reason: 'frequency_cap:trigger',
    created_at: '2026-08-30T08:01:00Z',
    updated_at: '2026-08-30T08:01:00Z'
  }

  await page.route('https://localapi.notifuse.com:4000/**', async (route) => {
    const request = route.request()
    const url = request.url()
    if (url.includes('/api/automations.list')) return json(route, { automations: [automation], total: 1 })
    if (url.includes('/api/automations.preflight')) {
      return json(route, { preflight: {
        workspace_id: WORKSPACE_ID,
        automation_id: automation.id,
        issues: [{ code: 'frequency_policy_missing', severity: 'warning', title: 'No email frequency cap', description: 'Configure a cap before activation.' }],
        blocking_count: 0,
        warning_count: 1,
        summary_hash: 'preflight-hash.1',
        generated_at: NOW,
        expires_at: '2026-08-30T08:05:00Z'
      } })
    }
    if (url.includes('/api/automations.activate')) return json(route, { automation: { ...automation, status: 'live' } })
    if (url.includes('/api/customers.list')) return json(route, { customers: [customer] })
    if (url.includes('/api/customers.get')) return json(route, { customer })
    if (url.includes('/api/journeys.instances')) return json(route, { instances: [instance], total: 1, limit: 50, offset: 0 })
    if (url.includes('/api/journeys.trace')) {
      return json(route, { trace: {
        instance,
        entry_decisions: [{ id: 'decision-1', automation_id: automation.id, customer_id: customer.customer_id, origin_event_id: instance.origin_event_id, decision: 'enrolled', decided_at: NOW }],
        events: [{ id: 'event-1', node_id: 'trigger-1', event_type: 'enrolled', status: 'active', occurred_at: NOW }],
        deliveries: [{ intent: delivery, attempts: [], receipts: [] }]
      } })
    }
    if (url.includes('/api/deliveries.list')) return json(route, { deliveries: [delivery], total: 1 })
    return route.fallback()
  })

  return { automation, customer, instance }
}

test.describe('Marketing journey console contracts', () => {
  test('requires preflight warning confirmation and forwards its evidence hash', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const { automation } = await routeJourneyConsole(page)
    let activationBody: Record<string, unknown> | undefined
    page.on('request', (request) => {
      if (request.url().includes('/api/automations.activate')) activationBody = request.postDataJSON()
    })

    await page.goto(`/console/workspace/${WORKSPACE_ID}/automations`)
    await expect(page.getByText(automation.name)).toBeVisible()
    await page.getByRole('button', { name: 'Activate', exact: true }).click()

    const preflight = page.getByRole('region', { name: 'Activation preflight' })
    await expect(preflight).toContainText('No email frequency cap')
    await expect(preflight.getByRole('button', { name: 'Activate journey' })).toBeDisabled()
    await preflight.getByRole('checkbox').check()
    await page.screenshot({ path: '../docs/operations/evidence/b3-journey-preflight.png', fullPage: true })
    await preflight.getByRole('button', { name: 'Activate journey' }).click()

    await expect.poll(() => activationBody).toMatchObject({
      workspace_id: WORKSPACE_ID,
      automation_id: automation.id,
      preflight_hash: 'preflight-hash.1',
      confirm_warnings: true
    })
  })

  test('connects Customer 360 to Journey trace and frequency-suppressed delivery', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const { customer, instance } = await routeJourneyConsole(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/customers`)
    await page.getByText(customer.customer_no).click()
    const customer360 = page.getByRole('dialog', { name: 'Customer 360' })
    await expect(customer360).toContainText(customer.external_user_id)
    await customer360.getByRole('tab', { name: 'Journeys' }).click()
    await expect(customer360).toContainText('Waiting for scheduled send')
    await customer360.getByRole('button', { name: 'View trace' }).click()

    const trace = page.getByRole('dialog', { name: 'Journey trace' })
    await expect(trace).toContainText(instance.id)
    await expect(trace).toContainText('Customer entered the journey')
    await expect(trace).toContainText('EMAIL')
    await expect(trace).toContainText('frequency')
    await expect(trace.getByRole('link', { name: 'Fix this node' }).first()).toHaveAttribute('href', /automation_id=journey-welcome/)
    await page.screenshot({ path: '../docs/operations/evidence/b3-customer-journey-trace.png', fullPage: true })
  })
})
