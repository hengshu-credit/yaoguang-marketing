import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { PostsTable } from './PostsTable'

const mocks = vi.hoisted(() => ({ posts: vi.fn().mockResolvedValue({ posts: [], total_count: 0 }), editor: vi.fn() }))
vi.mock('../../contexts/AuthContext', () => ({
 useAuth: () => ({ workspaces: [{ id: 'ws1', settings: {} }] }),
 useWorkspacePermissions: () => ({ permissions: { workspace: { read: true, write: true } } }),
}))
vi.mock('@tanstack/react-router', () => ({ useParams: () => ({ workspaceId: 'ws1' }), useSearch: () => ({}), useNavigate: () => vi.fn() }))
vi.mock('../../services/api/blog', () => ({ blogPostsApi: { list: mocks.posts }, blogCategoriesApi: { list: vi.fn().mockResolvedValue({ categories: [] }) } }))
vi.mock('./PostDrawer', () => ({ PostDrawer: () => { mocks.editor(); return <div>Article editor</div> } }))
vi.mock('./CategoryDrawer', () => ({ CategoryDrawer: () => null }))
vi.mock('./DeleteCategoryModal', () => ({ DeleteCategoryModal: () => null }))
vi.mock('./DeletePostModal', () => ({ DeletePostModal: () => null }))
vi.mock('./PublishModal', () => ({ PublishModal: () => null }))

it('fetches the article list once and mounts the editor only after New Post', async () => {
 render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><PostsTable /></QueryClientProvider>)
 expect(await screen.findByText('No posts yet')).toBeVisible()
 expect(mocks.posts).toHaveBeenCalledTimes(1)
 expect(mocks.editor).not.toHaveBeenCalled()
 await userEvent.click(screen.getByRole('button', { name: /New Post/ }))
 expect(await screen.findByText('Article editor')).toBeVisible()
 expect(mocks.posts).toHaveBeenCalledTimes(1)
})
