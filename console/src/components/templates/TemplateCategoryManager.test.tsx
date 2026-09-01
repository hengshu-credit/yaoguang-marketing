import { App as AntApp } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { templateCategoriesApi } from '../../services/api/templateCategories'
import TemplateCategoryManager from './TemplateCategoryManager'

vi.mock('../../services/api/templateCategories', () => ({
  templateCategoriesApi: { list: vi.fn(), create: vi.fn(), update: vi.fn(), delete: vi.fn() }
}))

it('creates a Workspace template category from the management drawer', async () => {
  vi.mocked(templateCategoriesApi.list).mockResolvedValue({ categories: [] })
  vi.mocked(templateCategoriesApi.create).mockResolvedValue({ category: {
    id: 'vip', name: 'VIP', purpose: 'marketing', sort_order: 100, is_system: false,
    is_active: true, usage_count: 0, created_at: '', updated_at: ''
  } })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const user = userEvent.setup()
  render(<QueryClientProvider client={client}><AntApp><TemplateCategoryManager workspaceId="ws1" canWrite /></AntApp></QueryClientProvider>)

  await user.click(screen.getByRole('button', { name: /Manage categories/ }))
  await user.click(await screen.findByRole('button', { name: /Add category/ }))
  await user.type(screen.getByLabelText('Category ID'), 'vip')
  await user.type(screen.getByLabelText('Name'), 'VIP')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  expect(vi.mocked(templateCategoriesApi.create).mock.calls[0][0]).toEqual(expect.objectContaining({
    workspace_id: 'ws1', id: 'vip', name: 'VIP', purpose: 'transactional'
  }))
})
