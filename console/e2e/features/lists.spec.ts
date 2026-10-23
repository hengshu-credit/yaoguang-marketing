import type { Locator, Page } from '@playwright/test'
import { test, expect, requestCapture } from '../fixtures/auth'
import { waitForLoading, waitForSuccessMessage } from '../fixtures/test-utils'
import { API_PATTERNS } from '../fixtures/request-capture'
import { testListData } from '../fixtures/form-data'
import { logCapturedRequests } from '../fixtures/payload-assertions'

const WORKSPACE_ID = 'test-workspace'

// antd 6 renamed `.ant-drawer-content` out of existence; the drawer's `role="dialog"`
// node now carries the title as its accessible name, so every drawer is addressed by
// role + name instead of by class.
const createListDrawer = (page: Page) =>
  page.getByRole('dialog', { name: 'Create New List', exact: true })

const editListDrawer = (page: Page) => page.getByRole('dialog', { name: 'Edit List', exact: true })

/** Open the create drawer from the unified audience page. */
async function openCreateDrawer(page: Page): Promise<Locator> {
  await page.getByRole('button', { name: 'Create list', exact: true }).click()
  const drawer = createListDrawer(page)
  await expect(drawer).toBeVisible()
  return drawer
}

/** Open the edit drawer belonging to the named audience row. */
async function openEditDrawer(page: Page, listName = 'Newsletter'): Promise<Locator> {
  const row = page.getByRole('row').filter({ has: page.getByRole('link', { name: listName, exact: true }) })
  await row.getByRole('button', { name: 'Edit list', exact: true }).click()

  const drawer = editListDrawer(page)
  await expect(drawer).toBeVisible()
  return drawer
}

test.describe('Lists Feature', () => {
  test.describe('Page Load & Empty State', () => {
    test('loads the unified lists page and shows its empty state', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
      await waitForLoading(page)

      await expect(page).toHaveURL(`/console/workspace/${WORKSPACE_ID}/lists`)
      await expect(page.getByText('No audiences yet')).toBeVisible()
      await expect(page.getByRole('button', { name: 'Create list', exact: true })).toBeVisible()
    })

    test('loads lists page with data', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
      await waitForLoading(page)

      await expect(page.locator('.audience-row')).toHaveCount(3)
      await expect(page.locator('.audience-row > td:first-child a')).toHaveText([
        'Newsletter',
        'Marketing Updates',
        'Beta Testers'
      ])
    })
  })

  test.describe('CRUD Operations', () => {
    test('opens create list form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await expect(drawer.getByLabel('Name', { exact: true })).toBeVisible()
      await expect(drawer.getByLabel('List ID', { exact: true })).toBeVisible()
    })

    test('fills and submits list form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByLabel('Name', { exact: true }).fill('Test Newsletter List')

      // The ID is derived from the name: lowercased, non-alphanumerics stripped
      await expect(drawer.getByLabel('List ID', { exact: true })).toHaveValue('testnewsletterlist')

      await drawer
        .getByLabel('Description', { exact: true })
        .fill('A test newsletter list with all fields')

      await drawer.getByRole('button', { name: 'Create', exact: true }).click()

      await waitForSuccessMessage(page)
      await expect(drawer).toBeHidden()
    })

    test('views list details', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
      await waitForLoading(page)

      const details = page.getByRole('row').filter({ has: page.getByRole('link', { name: 'Newsletter', exact: true }) })
      await expect(details).toContainText('Monthly newsletter subscribers')
      await expect(details.getByRole('link', { name: 'Newsletter' })).toHaveAttribute(
        'href', `/console/workspace/${WORKSPACE_ID}/lists/list/list-1`
      )
    })
  })

  test.describe('List Configuration', () => {
    test('shows double opt-in setting', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      const doubleOptIn = drawer.getByRole('switch', { name: 'Double Opt-in' })
      await expect(doubleOptIn).toBeVisible()
      await expect(doubleOptIn).not.toBeChecked()

      // Turning it on is what pulls in the confirmation template field
      await doubleOptIn.click()
      await expect(doubleOptIn).toBeChecked()
      await expect(drawer.getByPlaceholder('Select confirmation email template')).toBeVisible()
    })

    // The template picker only has something to show when the workspace has
    // templates, so this case runs against the populated fixture.
    test('shows template selection options', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)
      await drawer.getByRole('switch', { name: 'Double Opt-in' }).click()

      // The template field opens a picker listing the workspace templates
      await drawer.getByPlaceholder('Select confirmation email template').click()

      const picker = page.getByRole('dialog', { name: 'Select Template', exact: true })
      await expect(picker).toBeVisible()
      await expect(picker.getByText('Welcome Email')).toBeVisible()
      await expect(picker.getByRole('button', { name: 'Select', exact: true })).toHaveCount(3)
    })
  })

  test.describe('List Statistics', () => {
    test('displays subscriber counts', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
      await waitForLoading(page)

      const stats = page.getByLabel('Customer status counts for Newsletter', { exact: true }).locator('.ant-statistic')
      await expect(stats.filter({ hasText: 'Active' })).toContainText('150')
      await expect(stats.filter({ hasText: 'Pending' })).toContainText('25')
      await expect(stats.filter({ hasText: 'Unsub' })).toContainText('10')
    })
  })

  test.describe('Edit Form Prefill', () => {
    test('edit list drawer shows existing list name', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page)

      await expect(drawer.getByLabel('Name', { exact: true })).toHaveValue('Newsletter')
    })

    test('edit list preserves list ID', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page)

      const idInput = drawer.getByLabel('List ID', { exact: true })
      await expect(idInput).toHaveValue('list-1')
      // The ID is the list's key, so editing an existing list cannot change it
      await expect(idInput).toBeDisabled()
    })

    test('edit list preserves description', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page)

      await expect(drawer.getByLabel('Description', { exact: true })).toHaveValue(
        'Monthly newsletter subscribers'
      )
    })

    test('edit list preserves double opt-in setting', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page)

      // The first mocked list is both public and double opt-in
      await expect(drawer.getByRole('switch', { name: 'Double Opt-in' })).toBeChecked()
      await expect(drawer.getByRole('switch', { name: 'Public' })).toBeChecked()
    })
  })

  test.describe('Form Validation', () => {
    test('requires list name', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByRole('button', { name: 'Create', exact: true }).click()

      await expect(drawer.getByText('Please enter a list name')).toBeVisible()
      await expect(drawer.getByText('Please enter a list ID')).toBeVisible()
      expect(requestCapture.getRequestCount(API_PATTERNS.LIST_CREATE)).toBe(0)
    })

    test('validates list ID format', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByLabel('Name', { exact: true }).fill('Test List')

      const idInput = drawer.getByLabel('List ID', { exact: true })
      await idInput.fill('invalid@id!')

      await drawer.getByRole('button', { name: 'Create', exact: true }).click()

      await expect(drawer.getByText('ID must be alphanumeric')).toBeVisible()
      expect(requestCapture.getRequestCount(API_PATTERNS.LIST_CREATE)).toBe(0)
    })
  })

  test.describe('Navigation', () => {
    test('navigates to audiences from sidebar', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      // Start at dashboard
      await page.goto(`/console/workspace/${WORKSPACE_ID}/`)
      await waitForLoading(page)

      await page.getByRole('link', { name: 'Audiences & Lists', exact: true }).click()
      await expect(page).toHaveURL(`/console/workspace/${WORKSPACE_ID}/lists`)
    })

    test('can close create form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByRole('button', { name: 'Close', exact: true }).click()

      await expect(drawer).toBeHidden()
    })
  })

  test.describe('Full Form Submission with Payload Verification', () => {
    // Uses the populated fixture: double opt-in requires picking a real template.
    test('creates list with all fields and verifies payload', async ({
      authenticatedPageWithData
    }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByLabel('Name', { exact: true }).fill(testListData.name)
      await drawer.getByLabel('Description', { exact: true }).fill(testListData.description || '')

      const publicSwitch = drawer.getByRole('switch', { name: 'Public' })
      if (testListData.is_public) {
        await publicSwitch.click()
      }
      await expect(publicSwitch).toBeChecked({ checked: !!testListData.is_public })

      const doubleOptInSwitch = drawer.getByRole('switch', { name: 'Double Opt-in' })
      if (testListData.is_double_optin) {
        await doubleOptInSwitch.click()
      }
      await expect(doubleOptInSwitch).toBeChecked({ checked: !!testListData.is_double_optin })

      // Double opt-in makes the confirmation template a required field
      await drawer.getByPlaceholder('Select confirmation email template').click()
      const picker = page.getByRole('dialog', { name: 'Select Template', exact: true })
      await picker
        .getByRole('listitem')
        .filter({ hasText: 'Welcome Email' })
        .getByRole('button', { name: 'Select', exact: true })
        .click()
      await expect(drawer.getByPlaceholder('Select confirmation email template')).toHaveValue(
        'Welcome Email'
      )

      await drawer.getByRole('button', { name: 'Create', exact: true }).click()

      await expect
        .poll(() => requestCapture.getRequestCount(API_PATTERNS.LIST_CREATE))
        .toBeGreaterThan(0)
      logCapturedRequests(requestCapture)

      const request = requestCapture.getLastRequest(API_PATTERNS.LIST_CREATE)
      expect(request?.body, 'List create body should not be empty').toBeTruthy()
      const body = request!.body as Record<string, unknown>

      expect(body.workspace_id).toBe(WORKSPACE_ID)
      expect(body.name).toBe(testListData.name)
      expect(body.id).toBe('e2etestlist')
      expect(body.description).toBe(testListData.description)
      expect(body.is_double_optin).toBe(testListData.is_double_optin)
      expect(body.is_public).toBe(testListData.is_public)
      expect(body.double_optin_template).toEqual({ id: 'tpl-1', version: 1 })
    })

    test('verifies list configuration settings in payload', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByLabel('Name', { exact: true }).fill('Configuration Test List')

      // Both toggles left at their defaults: the payload must spell them out as false
      await drawer.getByRole('button', { name: 'Create', exact: true }).click()

      await expect
        .poll(() => requestCapture.getRequestCount(API_PATTERNS.LIST_CREATE))
        .toBeGreaterThan(0)

      const request = requestCapture.getLastRequest(API_PATTERNS.LIST_CREATE)
      expect(request?.body, 'List create body should not be empty').toBeTruthy()
      const body = request!.body as Record<string, unknown>

      expect(body.name).toBe('Configuration Test List')
      expect(body.id).toBe('configurationtestlist')
      expect(body.is_double_optin).toBe(false)
      expect(body.is_public).toBe(false)
    })
  })
})
