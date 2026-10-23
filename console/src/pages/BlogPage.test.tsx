import { App } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { BlogPage } from './BlogPage'

const mocks = vi.hoisted(() => ({ categories: vi.fn(), posts: vi.fn(), editor: vi.fn() }))
vi.mock('../contexts/AuthContext', () => ({ useAuth: () => ({ workspaces: [{ id: 'ws1', settings: {} }] }) }))
vi.mock('@tanstack/react-router', () => ({ useParams: () => ({ workspaceId: 'ws1' }), useSearch: () => ({}), useNavigate: () => vi.fn() }))
vi.mock('../services/api/blog', () => ({ blogCategoriesApi: { list: mocks.categories }, blogPostsApi: { list: mocks.posts } }))
vi.mock('../components/blog/BlogSidebar', () => ({ BlogSidebar: () => <nav>Categories</nav> }))
vi.mock('../components/blog/BlogPageHeader', () => ({ default: () => <h1>Blog Content</h1> }))
vi.mock('../components/blog/PostsTable', () => ({ PostsTable: () => <div>Posts table</div> }))
vi.mock('../components/blog/PostDrawer', () => ({ PostDrawer: () => { mocks.editor(); return null } }))
vi.mock('../components/blog/CategoryDrawer', () => ({ CategoryDrawer: () => null }))
vi.mock('../components/blog/DeleteCategoryModal', () => ({ DeleteCategoryModal: () => null }))
const show = () => render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><App><BlogPage /></App></QueryClientProvider>)

describe('BlogPage entry', () => {
 beforeEach(() => vi.clearAllMocks())
 it('keeps loading separate from an empty category list and does not mount the editor', () => {
  mocks.categories.mockReturnValue(new Promise(() => {}))
  show()
  expect(screen.queryByText('No categories yet')).not.toBeInTheDocument()
  expect(mocks.editor).not.toHaveBeenCalled()
 })
 it('loads the post table without a duplicate existence query or a closed editor', async () => {
  mocks.categories.mockResolvedValue({ categories: [{ id: 'news' }] })
  mocks.posts.mockResolvedValue({ posts: [], total_count: 0 })
  show()
  expect(await screen.findByText('Posts table')).toBeVisible()
  expect(mocks.posts).not.toHaveBeenCalled()
  expect(mocks.editor).not.toHaveBeenCalled()
 })
 it('reports a category request failure instead of claiming the workspace is empty', async () => {
  mocks.categories.mockRejectedValue(new Error('Categories unavailable'))
  show()
  expect(await screen.findByRole('alert')).toHaveTextContent('Categories unavailable')
  expect(screen.queryByText('No categories yet')).not.toBeInTheDocument()
 })
})
