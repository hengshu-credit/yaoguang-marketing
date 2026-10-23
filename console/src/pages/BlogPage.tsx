import { useState } from 'react'
import { Alert, Button, Empty, App, Skeleton } from 'antd'
import { useParams, useNavigate, useSearch } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { PlusOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { PostsTable } from '../components/blog/PostsTable'
import { BlogSidebar } from '../components/blog/BlogSidebar'
import { CategoryDrawer } from '../components/blog/CategoryDrawer'
import { DeleteCategoryModal } from '../components/blog/DeleteCategoryModal'
import { blogCategoriesApi, BlogCategory } from '../services/api/blog'
import { useAuth } from '../contexts/AuthContext'
import BlogPageHeader from '../components/blog/BlogPageHeader'

interface BlogSearch {
  category_id?: string
}

export function BlogPage() {
  const { t } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId/blog' })
  const navigate = useNavigate({ from: '/console/workspace/$workspaceId/blog' })
  const search = useSearch({ from: '/console/workspace/$workspaceId/blog' }) as BlogSearch
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const { workspaces } = useAuth()

  // All hooks must be called before any conditional returns
  const [categoryDrawerOpen, setCategoryDrawerOpen] = useState(false)
  const [editingCategory, setEditingCategory] = useState<BlogCategory | null>(null)
  const [deleteCategoryModalOpen, setDeleteCategoryModalOpen] = useState(false)
  const [categoryToDelete, setCategoryToDelete] = useState<BlogCategory | null>(null)
  const { data: categoriesData, isPending, error, refetch } = useQuery({
    queryKey: ['blog-categories', workspaceId],
    queryFn: () => blogCategoriesApi.list(workspaceId)
  })

  const categories = categoriesData?.categories ?? []
  const hasCategories = categories.length > 0
  const activeCategoryId = search.category_id || null

  const deleteCategoryMutation = useMutation({
    mutationFn: (id: string) => blogCategoriesApi.delete(workspaceId, { id }),
    onSuccess: () => {
      message.success(t`Category deleted successfully`)
      queryClient.invalidateQueries({ queryKey: ['blog-categories', workspaceId] })
      setDeleteCategoryModalOpen(false)
      setCategoryToDelete(null)
      // Navigate to all posts if the deleted category was active
      if (activeCategoryId === categoryToDelete?.id) {
        navigate({
          search: (prev) => ({ ...prev, category_id: undefined })
        })
      }
    },
    onError: (error: unknown) => {
      const errorMsg = error instanceof Error ? error.message : t`Failed to delete category`
      message.error(errorMsg)
    }
  })

  // Get the current workspace - conditional check AFTER all hooks
  const workspace = workspaces.find((w) => w.id === workspaceId)

  if (!workspace) {
    return null // Or handle the case where workspace is not found
  }

  const handleCategoryChange = (categoryId: string | null) => {
    navigate({
      search: (prev) => ({ ...prev, category_id: categoryId || undefined })
    })
  }

  const handleNewCategory = () => {
    setEditingCategory(null)
    setCategoryDrawerOpen(true)
  }

  const handleEditCategory = (category: BlogCategory) => {
    setEditingCategory(category)
    setCategoryDrawerOpen(true)
  }

  const handleDeleteCategory = (category: BlogCategory) => {
    setCategoryToDelete(category)
    setDeleteCategoryModalOpen(true)
  }

  const handleCategoryDrawerClose = () => {
    setCategoryDrawerOpen(false)
    setEditingCategory(null)
  }

  return (
    <div>
      <BlogPageHeader workspaceId={workspaceId} />

      <div className="grid items-start gap-6 lg:grid-cols-[240px_minmax(0,1fr)]">
        <aside className="min-w-0 overflow-hidden rounded-lg border" style={{ backgroundColor: 'var(--console-surface, #FFFFFF)', borderColor: 'var(--console-border, #DCE3ED)' }}>
          <BlogSidebar
            workspaceId={workspaceId}
            activeCategoryId={activeCategoryId}
            onCategoryChange={handleCategoryChange}
            onNewCategory={handleNewCategory}
            onEditCategory={handleEditCategory}
            onDeleteCategory={handleDeleteCategory}
          />
        </aside>
        <div className="min-w-0">
          {isPending ? <Skeleton active /> : error ? (
            <Alert type="error" showIcon title={error.message} action={<Button onClick={() => void refetch()}>{t`Retry`}</Button>} />
          ) : !hasCategories ? (
            <Empty description={t`No categories yet`} className="py-16">
              <Button type="primary" icon={<PlusOutlined />} onClick={handleNewCategory}>
                {t`Create Your First Category`}
              </Button>
            </Empty>
          ) : (
            <PostsTable />
          )}
        </div>

        <CategoryDrawer
          open={categoryDrawerOpen}
          onClose={handleCategoryDrawerClose}
          category={editingCategory}
          workspaceId={workspaceId}
        />
        <DeleteCategoryModal
          open={deleteCategoryModalOpen}
          category={categoryToDelete}
          onConfirm={() => categoryToDelete && deleteCategoryMutation.mutate(categoryToDelete.id)}
          onCancel={() => {
            setDeleteCategoryModalOpen(false)
            setCategoryToDelete(null)
          }}
          loading={deleteCategoryMutation.isPending}
        />

      </div>
    </div>
  )
}
