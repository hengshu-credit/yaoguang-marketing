import '../../__tests__/resizeObserverMock'
import { App } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { I18nProvider } from '@lingui/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { i18n } from '../../i18n'
import { CustomerImportPanel } from './CustomerImportPanel'

const { listLists, listImports, uploadImport } = vi.hoisted(() => ({
  listLists: vi.fn(),
  listImports: vi.fn(),
  uploadImport: vi.fn()
}))

vi.mock('../../services/api/list', () => ({
  listsApi: { list: listLists }
}))

vi.mock('../../services/api/marketing', () => ({
  importJobApi: {
    list: listImports,
    upload: uploadImport,
    get: vi.fn(),
    cancel: vi.fn(),
    downloadErrors: vi.fn()
  }
}))

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <App>{children}</App>
      </I18nProvider>
    </QueryClientProvider>
  )
}

describe('CustomerImportPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listImports.mockResolvedValue({ items: [], total: 0, limit: 50, offset: 0 })
    uploadImport.mockResolvedValue({
      id: 'job-1',
      filename: 'customers.csv',
      status: 'staged',
      list_ids: [],
      counters: { total: 1, pending: 1, processing: 0, succeeded: 0, failed: 0 }
    })
  })

  it('still imports customers without list binding when lists cannot be loaded', async () => {
    listLists.mockRejectedValue(new Error('forbidden'))
    const { container } = render(<CustomerImportPanel workspaceId="ws1" canWrite />, { wrapper })

    expect(await screen.findByText('Lists could not be loaded. You can still import without list binding.')).toBeInTheDocument()
    const fileInput = container.querySelector<HTMLInputElement>('input[type="file"]')
    expect(fileInput).not.toBeNull()
    expect(fileInput).not.toBeDisabled()

    const file = new File(['external_user_id\ncustomer-1\n'], 'customers.csv', { type: 'text/csv' })
    fireEvent.change(fileInput!, { target: { files: [file] } })

    await waitFor(() => expect(uploadImport).toHaveBeenCalledWith('ws1', file, []))
  })

  it('does not request or expose list selection without list permission', async () => {
    const { container } = render(
      <CustomerImportPanel workspaceId="ws1" canWrite canBindLists={false} />,
      { wrapper }
    )

    await screen.findByText('Recent imports')
    expect(listLists).not.toHaveBeenCalled()
    expect(screen.queryByLabelText('Target lists')).not.toBeInTheDocument()
    const fileInput = container.querySelector<HTMLInputElement>('input[type="file"]')
    expect(fileInput).not.toBeNull()
    expect(fileInput).not.toBeDisabled()
  })
})
