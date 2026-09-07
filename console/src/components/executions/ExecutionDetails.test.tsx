import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App, ConfigProvider } from 'antd'
import { ExecutionDetails } from './ExecutionDetails'
import { executionsApi, type ExecutionDetailsResponse } from '../../services/api/executions'
vi.mock('../../services/api/executions', () => ({ executionsApi: { get: vi.fn(), list: vi.fn(), asset: vi.fn() } }))
const fixture = (id: string, product: string, subject: string): ExecutionDetailsResponse => ({ snapshot: { id, execution_id: id, source_id: 'campaign', kind: 'campaign', configuration: { filters: { product } }, materials: { templates: [{ id: 'template', name: 'Saved template', email: { subject } }], assets: [] }, dependencies: {}, content_hash: 'hash', completeness: 'complete', missing: [], created_at: '2026-09-07T00:00:00Z' }, evidence: [] })
function view(id: string, client: QueryClient) { return <QueryClientProvider client={client}><ConfigProvider theme={{ token: { motion: false } }}><App><ExecutionDetails workspaceId="workspace" executionId={id} /></App></ConfigProvider></QueryClientProvider> }
describe('ExecutionDetails', () => {
 beforeEach(() => vi.clearAllMocks())
 it('switches immutable configuration and templates together when run changes', async () => {
  vi.mocked(executionsApi.get).mockImplementation(async (_workspace, id) => id === 'run-a' ? fixture(id, 'A', 'Original content') : fixture(id, 'B', 'Changed content'))
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const rendered = render(view('run-a', client))
  expect(await screen.findByText(/"product": "A"/)).toBeInTheDocument()
  const user = userEvent.setup()
  await act(async () => { await user.click(screen.getByRole('tab', { name: 'Materials' })) })
  await act(async () => { await user.click(screen.getByText('Saved template')) })
  expect(await screen.findByText('Original content')).toBeInTheDocument()
  rendered.rerender(view('run-b', client))
  expect(await screen.findByText(/"product": "B"/)).toBeInTheDocument()
  await act(async () => { await user.click(screen.getByRole('tab', { name: 'Materials' })) })
  await act(async () => { await user.click(screen.getByText('Saved template')) })
  expect(await screen.findByText('Changed content')).toBeInTheDocument()
  expect(screen.queryByText('Original content')).not.toBeInTheDocument()
 })
 it('states missing historical content instead of loading current templates', async () => {
  vi.mocked(executionsApi.get).mockRejectedValue(Object.assign(new Error('Missing'), { status: 404 }))
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(view('old-run', client))
  expect(await screen.findByText('This historical execution has no configuration snapshot')).toBeInTheDocument()
 })
})
