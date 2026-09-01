import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { templateCategoriesApi } from '../../services/api/templateCategories'
import TemplateCategorySelect from './TemplateCategorySelect'

vi.mock('../../services/api/templateCategories', () => ({ templateCategoriesApi: { list: vi.fn() } }))

it('shows active categories and keeps the current inactive category selectable', async () => {
  vi.mocked(templateCategoriesApi.list).mockResolvedValue({ categories: [
    { id: 'marketing', name: 'Marketing', purpose: 'marketing', sort_order: 10, is_system: true, is_active: true, usage_count: 2, created_at: '', updated_at: '' },
    { id: 'legacy', name: 'Legacy', purpose: 'transactional', sort_order: 20, is_system: false, is_active: false, usage_count: 1, created_at: '', updated_at: '' }
  ] })
  const onChange = vi.fn()
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><TemplateCategorySelect workspaceId="ws1" value="legacy" onChange={onChange} /></QueryClientProvider>)

  await userEvent.click(await screen.findByRole('combobox'))
  expect(await screen.findAllByText('Legacy')).not.toHaveLength(0)
  await userEvent.click(screen.getByText('Marketing'))
  expect(onChange.mock.calls[0][0]).toBe('marketing')
})
