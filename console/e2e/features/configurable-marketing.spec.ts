import { test, expect } from '@playwright/test'
import { readFileSync } from 'node:fs'

// The opt-in Go HTTP integration fixture provides a disposable real workspace.
// No business endpoint is mocked: configuration, ingestion and analysis all use PostgreSQL.
const fixturePath = process.env.CONFIGURABLE_BROWSER_FIXTURE
test.skip(!fixturePath, 'Start TestConfigurableMarketingHTTPIntegration with CONFIGURABLE_BROWSER_FIXTURE first')
const fixture = fixturePath ? JSON.parse(readFileSync(fixturePath, 'utf8')) as { auth_token: string; workspace_id: string; customer_id: string } : undefined

test('configure repeated events, drag and persist the funnel, calculate real occurrences', async ({ page, request }, testInfo) => {
  const { workspace_id, customer_id, auth_token } = fixture!
  const headers = { Authorization: `Bearer ${auth_token}` }
  const code = `browser.login.${Date.now()}`
  const name = `Browser sign in ${Date.now()}`
  const post = async (action: string, data: unknown) => {
    const response = await request.post(`/api/${action}`, { headers, data })
    expect(response.ok(), `${action}: ${await response.text()}`).toBeTruthy()
    return response.json()
  }
  const errors: string[] = []
  page.on('pageerror', error => { if (!error.message.includes('WebSocket closed without opened')) errors.push(error.message) })
  await page.addInitScript(token => localStorage.setItem('auth_token', token), auth_token)
  await page.route('**/config.js?*', async route => {
    const response = await route.fetch()
    await route.fulfill({ response, body: `${await response.text()}\nwindow.API_ENDPOINT=window.location.origin;` })
  })
  await page.goto(`/console/workspace/${workspace_id}/settings/business`)
  const language = page.getByRole('button', { name: /^(EN|ZH-CN)$/, exact: true })
  await expect(language).toBeVisible()
  if (await language.textContent() !== 'EN') {
    await language.click()
    await page.getByText('English', { exact: true }).click()
  }
  await page.getByRole('button', { name: 'New definition', exact: true }).click()
  await page.getByRole('textbox', { name: 'Definition key', exact: true }).fill(code)
  await page.getByRole('textbox', { name: 'Definition name', exact: true }).fill(name)
  await page.getByRole('combobox', { name: 'Definition status', exact: true }).click()
  await page.getByTitle('Active', { exact: true }).last().click()
  await page.getByRole('tab', { name: 'Report data fields', exact: true }).click()
  await page.getByRole('button', { name: 'Add field', exact: true }).click()
  await page.getByRole('textbox', { name: 'Field key field_1', exact: true }).fill('device')
  await page.getByRole('textbox', { name: 'Field key field_1', exact: true }).press('Tab')
  await page.getByRole('switch', { name: 'Required device', exact: true }).click()
  const event = { report_id: `ui-${Date.now()}`, event_code: code, customer_attributes: { customer_id }, data: { device: 'mobile' }, triggered_at: new Date(Date.now() - 60000).toISOString() }
  await page.getByRole('tab', { name: 'Sample validation', exact: true }).click()
  await page.getByRole('textbox', { name: 'Sample JSON', exact: true }).fill(JSON.stringify({ ...event, data: {} }))
  await page.getByRole('button', { name: 'Validate sample', exact: true }).click()
  await expect(page.getByText('Sample validation failed', { exact: true })).toBeVisible()
  await page.getByRole('textbox', { name: 'Sample JSON', exact: true }).fill(JSON.stringify(event))
  await page.getByRole('button', { name: 'Validate sample', exact: true }).click()
  await expect(page.getByText('Sample is valid', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Save definition', exact: true }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)

  const first = await post('events.track', { workspace_id, source: 'browser-test', event })
  const canonicalCustomerId = first.customer_id as string
  expect(canonicalCustomerId).toMatch(/^[0-9a-f-]{36}$/)
  const retry = await post('events.track', { workspace_id, source: 'browser-test', event })
  const next = await post('events.track', { workspace_id, source: 'browser-test', event: { ...event, report_id: `${event.report_id}-2`, triggered_at: new Date(Date.now() - 30000).toISOString() } })
  expect(retry.duplicate).toBe(true)
  expect(retry.event_id).toBe(first.event_id)
  expect(next.event_id).not.toBe(first.event_id)

  await page.goto(`/console/workspace/${workspace_id}/funnels`)
  await page.getByRole('button', { name, exact: true }).click()
  await page.getByRole('button', { name, exact: true }).click()
  await page.getByRole('textbox', { name: 'Step name', exact: true }).nth(0).fill('First sign in')
  await page.getByRole('textbox', { name: 'Step name', exact: true }).nth(1).fill('Return sign in')
  const handle = page.getByRole('button', { name: 'Drag step', exact: true }).first()
  await handle.focus(); await page.keyboard.press('Space')
  await expect(handle).toHaveAttribute('aria-pressed', 'true')
  await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => resolve())))
  await page.keyboard.press('ArrowDown')
  await expect.poll(async () => {
    const text = await page.getByText(/was moved over droppable area/).textContent()
    const ids = text?.match(/Draggable item (\S+) was moved over droppable area (\S+)\./)
    return !!ids && ids[1] !== ids[2]
  }).toBe(true)
  await page.keyboard.press('Space')
  await expect(page.getByRole('textbox', { name: 'Step name', exact: true }).first()).toHaveValue('Return sign in')
  await page.getByRole('button', { name: 'Move step up', exact: true }).nth(1).click()
  await page.getByRole('textbox', { name: 'Funnel key', exact: true }).fill(code)
  await page.getByRole('textbox', { name: 'Funnel name', exact: true }).fill(name)
  const saved = page.waitForResponse(response => response.url().includes('/api/businessDefinitions.save') && response.request().method() === 'POST')
  await page.getByRole('button', { name: 'Save funnel', exact: true }).click()
  expect((await saved).ok()).toBe(true)
  await page.reload()
  await page.getByRole('combobox', { name: 'Saved funnels', exact: true }).click()
  await page.locator('.ant-select-dropdown:visible').getByTitle(name, { exact: true }).click()
  await expect(page.getByRole('textbox', { name: 'Step name', exact: true }).first()).toHaveValue('First sign in')
  const started = page.waitForResponse(response => response.url().includes('/api/funnels.start'))
  await page.getByRole('button', { name: 'Calculate funnel', exact: true }).click()
  const startedResponse = await started
  expect(startedResponse.status()).toBe(202)
  const job = await startedResponse.json()
  await expect(page.getByRole('columnheader', { name: 'Occurrences', exact: true })).toBeVisible()
  const rows = page.getByRole('row')
  await expect(rows.filter({ has: page.getByRole('cell', { name: 'First sign in', exact: true }) })).toContainText('2')
  await expect(rows.filter({ has: page.getByRole('cell', { name: 'Return sign in', exact: true }) })).toContainText('100.0%')
  const completed = await request.get(`/api/funnels.get?workspace_id=${workspace_id}&id=${job.id}`, { headers })
  const computed = await completed.json()
  expect(computed.result.steps.map((step: { objects: number; occurrences: number }) => [step.objects, step.occurrences])).toEqual([[1, 2], [1, 1]])
  await page.screenshot({ path: testInfo.outputPath('funnel-english.png'), fullPage: true })
  await page.getByRole('button', { name: 'EN', exact: true }).click()
  await page.getByText('简体中文', { exact: true }).click()
  await expect(page.getByRole('button', { name: '计算漏斗', exact: true })).toBeVisible()
  await page.getByRole('combobox', { name: '已保存漏斗', exact: true }).click()
  await page.locator('.ant-select-dropdown:visible').getByTitle(name, { exact: true }).click()
  await page.getByRole('button', { name: '计算漏斗', exact: true }).click()
  await expect(page.getByRole('cell', { name: 'First sign in', exact: true })).toBeVisible()
  await page.screenshot({ path: testInfo.outputPath('funnel-chinese.png'), fullPage: true })
  const stateKeys = [`${code}.active`, `${code}.engaged`]
  for (const stateKey of stateKeys) {
    await post('businessDefinitions.save', { workspace_id, definition: { kind: 'state', key: stateKey, name: stateKey, status: 'active', schema: { type: 'object' }, settings: { subject_type: 'customer', rule: { kind: 'event', event: code } } } })
    await post('subjectStates.set', { workspace_id, subject_type: 'customer', subject_id: canonicalCustomerId, override: { state_key: stateKey, value: 'true', reason: 'Browser overlap verification' } })
  }
  await page.getByText('状态覆盖', { exact: true }).click()
  await page.getByRole('combobox', { name: '所选状态', exact: true }).click()
  for (const stateKey of stateKeys) await page.getByTitle(stateKey, { exact: true }).click()
  await page.keyboard.press('Escape')
  const coverage = page.waitForResponse(response => response.url().includes('/api/subjectStates.coverage'))
  await page.getByRole('button', { name: '分析状态覆盖', exact: true }).click()
  const coverageResponse = await coverage
  expect(coverageResponse.ok()).toBe(true)
  const counts = await coverageResponse.json()
  expect(counts.intersection).toBe(1)
  expect(counts.union).toBe(1)
  expect(counts.states.map((state: { satisfied: number }) => state.satisfied)).toEqual([1, 1])
  await expect(page.getByText('全部所选状态均满足', { exact: true })).toBeVisible()
  await page.getByRole('heading', { name: '状态覆盖与交集', exact: true }).click()
  await page.screenshot({ path: testInfo.outputPath('state-coverage-chinese.png'), fullPage: true })
  expect(errors).toEqual([])
})
