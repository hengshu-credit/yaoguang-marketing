import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ExecutionObservations } from './ExecutionObservations'
import { executionsApi } from '../../services/api/executions'
vi.mock('../../services/api/executions', () => ({ executionsApi: { observations: vi.fn() } }))
describe('ExecutionObservations', () => {
 it('shows repeated occurrences separately from distinct customers and attributed conversions', async () => {
  vi.mocked(executionsApi.observations).mockResolvedValue({ execution_id: 'run', plan: { subject_type: 'loan', window_seconds: 3600 }, watermark: '2026-09-07T00:00:00Z', cohort_customers: 1, events: [{ key: 'borrow', goal: true, observed: { occurrences: 2, customers: 1, objects: 1 }, attributed: { occurrences: 0, customers: 0, objects: 0 }, event_ids: ['event-1', 'event-2'] }], states: [] })
  render(<QueryClientProvider client={new QueryClient()}><ExecutionObservations workspaceId="workspace" executionId="run" /></QueryClientProvider>)
  expect(await screen.findByText('borrow')).toBeInTheDocument()
  expect(screen.getAllByText('Observed occurrences')).toHaveLength(2)
  expect(screen.getByText('Attributed occurrences')).toBeInTheDocument()
  expect(screen.getByRole('cell', { name: '2' })).toBeInTheDocument()
  expect(screen.getByRole('cell', { name: '0' })).toBeInTheDocument()
 })
})
