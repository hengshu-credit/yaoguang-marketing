import type { Page } from '@playwright/test'
import { test, expect } from './fixtures/auth'

const WORKSPACE_ID = 'test-workspace'

async function assertNoDocumentOverflow(page: Page) {
  await expect.poll(() => page.evaluate(() => ({
    client: document.documentElement.clientWidth,
    scroll: document.documentElement.scrollWidth
  }))).toEqual(await page.evaluate(() => ({
    client: document.documentElement.clientWidth,
    scroll: document.documentElement.clientWidth
  })))
}

for (const viewport of [
  { name: 'phone', width: 375, height: 812 },
  { name: 'tablet', width: 768, height: 1024 }
]) {
  test.describe(`Marketing mobile console - ${viewport.name}`, () => {
    test.use({ viewport: { width: viewport.width, height: viewport.height } })

    test('keeps the nine domain-first entries reachable without document overflow', async ({ authenticatedPage }) => {
      const page = authenticatedPage
      await page.goto(`/console/workspace/${WORKSPACE_ID}/customers`)
      await expect(page.getByRole('heading', { name: 'Customers' })).toBeVisible()
      await assertNoDocumentOverflow(page)

      if (viewport.width < 768) await page.getByTestId('workspace-mobile-nav-toggle').click()
      const nav = page.locator('.workspace-sider-nav')
      await expect(nav.locator('.ant-menu-item')).toHaveCount(9)
      for (const name of ['Customers', 'Audiences', 'Marketing Campaigns', 'Automation Journeys', 'Delivery Center']) {
        await expect(nav.getByRole('link', { name })).toBeVisible()
      }

      if (viewport.width < 768) {
        const toggleBox = await page.getByTestId('workspace-mobile-nav-toggle').boundingBox()
        expect(toggleBox?.width).toBeGreaterThanOrEqual(44)
        expect(toggleBox?.height).toBeGreaterThanOrEqual(44)
        await page.getByTestId('workspace-mobile-nav-mask').click()
      }

      for (const target of [
        { path: 'audiences', visible: '客群' },
        { path: 'broadcasts', visible: 'No broadcasts found' },
        { path: 'automations', visible: 'Automations' },
        { path: 'deliveries', visible: 'Delivery Center' }
      ]) {
        await page.goto(`/console/workspace/${WORKSPACE_ID}/${target.path}`)
        await expect(page.getByText(target.visible, { exact: true }).first()).toBeVisible()
        await assertNoDocumentOverflow(page)
      }
      if (viewport.width === 375) {
        await page.screenshot({ path: '../docs/operations/evidence/b3-mobile-375.png', fullPage: true })
      }
    })
  })
}
