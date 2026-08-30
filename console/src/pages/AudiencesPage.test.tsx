import { App } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

beforeAll(() => {
  vi.stubGlobal('ResizeObserver', ResizeObserverStub)
  vi.stubGlobal('matchMedia', () => ({ matches: false, addListener() {}, removeListener() {}, addEventListener() {}, removeEventListener() {} }))
})

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ workspaceId: 'ws1' }),
  Link: ({ children }: { children: ReactNode }) => <a href="/lists">{children}</a>
}))

vi.mock('../services/api/list', () => ({
  listsApi: { list: vi.fn().mockResolvedValue({ lists: [{ id: 'list-1', name: '高意向名单' }] }) }
}))
vi.mock('../services/api/segment', () => ({
  listSegments: vi.fn().mockResolvedValue({ segments: [{ id: 'segment-1', name: '近30天活跃' }] })
}))
vi.mock('../services/api/marketing', () => ({
  audienceApi: {
    list: vi.fn().mockResolvedValue({ items: [{ id: 'audience-1', name: '存量客户', kind: 'static', active_version: 1, active_build_id: 'build-1' }], total: 1 }),
    preview: vi.fn(), create: vi.fn(), build: vi.fn(), delete: vi.fn()
  },
  importJobApi: {
    list: vi.fn().mockResolvedValue({ items: [{ id: 'job-1', status: 'completed', filename: 'customers.csv', counters: { total: 1000, pending: 0, processing: 0, succeeded: 998, failed: 2 } }], total: 1 }),
    get: vi.fn(), upload: vi.fn(), cancel: vi.fn(), downloadErrors: vi.fn()
  }
}))

import { AudiencesPage } from './AudiencesPage'

const renderPage = () => render(
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <App><AudiencesPage /></App>
  </QueryClientProvider>
)

describe('AudiencesPage business workflows', () => {
  it('lets a non-technical user compose more than one audience source', async () => {
    const user = userEvent.setup()
    renderPage()
    expect(await screen.findByText('创建可复用客群')).toBeInTheDocument()
    await user.click(screen.getByText('添加条件'))
    expect(screen.getByText('组合方式')).toBeInTheDocument()
    expect(screen.getByText('条件 1')).toBeInTheDocument()
    expect(screen.getByText('条件 2')).toBeInTheDocument()
    expect(screen.getByText('满足任一条件（并集）')).toBeInTheDocument()
  })

  it('shows a close-safe five-step import flow with history and no manual chunk button', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('tab', { name: '批量导入' }))
    expect(await screen.findByText('字段识别')).toBeInTheDocument()
    expect(screen.getByText('后台处理')).toBeInTheDocument()
    expect(screen.getByText('页面关闭后继续')).toBeInTheDocument()
    expect(screen.getByText('最近导入任务')).toBeInTheDocument()
    expect(await screen.findByText('customers.csv')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '处理下一批' })).not.toBeInTheDocument()
  })
})
