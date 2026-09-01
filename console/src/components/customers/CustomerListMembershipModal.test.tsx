import '../../__tests__/resizeObserverMock'
import { App } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { I18nProvider } from '@lingui/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { i18n } from '../../i18n'
import { CustomerListMembershipModal } from './CustomerListMembershipModal'

const { listLists, updateListMemberships } = vi.hoisted(() => ({
  listLists: vi.fn(),
  updateListMemberships: vi.fn()
}))

vi.mock('../../services/api/list', () => ({
  listsApi: { list: listLists }
}))

vi.mock('../../services/api/customer', () => ({
  customerApi: { updateListMemberships }
}))

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return (
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <App>{children}</App>
      </I18nProvider>
    </QueryClientProvider>
  )
}

describe('CustomerListMembershipModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listLists.mockResolvedValue({
      lists: [
        { id: 'newsletter', name: 'Newsletter', is_double_optin: false, is_public: false, created_at: '', updated_at: '' },
        { id: 'vip', name: 'VIP', is_double_optin: false, is_public: false, created_at: '', updated_at: '' }
      ]
    })
    updateListMemberships.mockResolvedValue({
      request_id: 'request-1', customers: 2, lists: 2, changed: 3, unchanged: 1
    })
  })

  it('adds selected customers to multiple lists with an explicit default status', async () => {
    const onSuccess = vi.fn()
    render(
      <CustomerListMembershipModal
        workspaceId="ws1"
        customerIds={['customer-1', 'customer-2']}
        open
        onClose={vi.fn()}
        onSuccess={onSuccess}
      />,
      { wrapper }
    )

    expect(await screen.findByText('Adjust list memberships')).toBeInTheDocument()
    fireEvent.mouseDown(screen.getByLabelText('Target lists'))
    fireEvent.click(await screen.findByText('Newsletter'))
    fireEvent.click(await screen.findByText('VIP'))
    fireEvent.click(screen.getByRole('button', { name: 'Apply changes' }))

    await waitFor(() => {
      expect(updateListMemberships).toHaveBeenCalledWith({
        workspace_id: 'ws1',
        customer_ids: ['customer-1', 'customer-2'],
        list_ids: ['newsletter', 'vip'],
        action: 'add',
        status: 'active'
      })
    })
    expect(onSuccess).toHaveBeenCalledWith({
      request_id: 'request-1', customers: 2, lists: 2, changed: 3, unchanged: 1
    })
  })

  it('removes one customer only from a current membership', async () => {
    render(
      <CustomerListMembershipModal
        workspaceId="ws1"
        customerIds={['customer-1']}
        currentListIds={['newsletter']}
        open
        onClose={vi.fn()}
        onSuccess={vi.fn()}
      />,
      { wrapper }
    )

    await screen.findByText('Adjust list memberships')
    const remove = screen.getByRole('radio', { name: 'Remove from lists' })
    fireEvent.click(remove)
    expect(remove).toBeChecked()
    fireEvent.mouseDown(screen.getByLabelText('Target lists'))
    fireEvent.click(await screen.findByText('Newsletter', { selector: '.ant-select-item-option-content' }))
    expect(screen.queryByRole('option', { name: 'VIP' })).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Membership status')).not.toBeInTheDocument()
    const apply = screen.getByRole('button', { name: 'Apply changes' })
    expect(apply).not.toBeDisabled()
    fireEvent.click(apply)

    await waitFor(() => {
      expect(updateListMemberships).toHaveBeenCalledWith({
        workspace_id: 'ws1',
        customer_ids: ['customer-1'],
        list_ids: ['newsletter'],
        action: 'remove',
        status: undefined
      })
    })
  })

  it('changes membership status explicitly without adding missing memberships', async () => {
    render(
      <CustomerListMembershipModal
        workspaceId="ws1"
        customerIds={['customer-1', 'customer-2']}
        open
        onClose={vi.fn()}
        onSuccess={vi.fn()}
      />,
      { wrapper }
    )

    await screen.findByText('Adjust list memberships')
    fireEvent.click(screen.getByRole('radio', { name: 'Change status' }))
    fireEvent.mouseDown(screen.getByLabelText('Target lists'))
    fireEvent.click(await screen.findByText('VIP', { selector: '.ant-select-item-option-content' }))
    fireEvent.mouseDown(screen.getByLabelText('Membership status'))
    fireEvent.click(await screen.findByText('Unsubscribed', { selector: '.ant-select-item-option-content' }))
    fireEvent.click(screen.getByRole('button', { name: 'Apply changes' }))

    await waitFor(() => {
      expect(updateListMemberships).toHaveBeenCalledWith({
        workspace_id: 'ws1',
        customer_ids: ['customer-1', 'customer-2'],
        list_ids: ['vip'],
        action: 'set_status',
        status: 'unsubscribed'
      })
    })
  })
})
