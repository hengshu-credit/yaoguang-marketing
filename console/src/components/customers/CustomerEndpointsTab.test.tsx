import { describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { CustomerEndpointsTab } from './CustomerEndpointsTab'
import { customerEndpointsApi } from '../../services/api/customer_endpoints'
vi.mock('../../services/api/customer_endpoints', () => ({ customerEndpointsApi: { list: vi.fn(), save: vi.fn() } }))
describe('CustomerEndpointsTab', () => {
 it('saves a Customer ID endpoint without email and requires active consent selection', async () => {
  vi.mocked(customerEndpointsApi.list).mockResolvedValue({ endpoints: [] })
  vi.mocked(customerEndpointsApi.save).mockResolvedValue({ saved: true })
  render(<QueryClientProvider client={new QueryClient()}><App><CustomerEndpointsTab workspaceId="ws" customerId="customer-id" canWrite /></App></QueryClientProvider>)
  const checkbox = await screen.findByRole('checkbox')
  expect(checkbox).not.toBeChecked()
  const user = userEvent.setup()
  await act(async () => { fireEvent.change(screen.getByLabelText('Endpoint ID'), { target: { value: 'customer-phone' } }) })
  await act(async () => { fireEvent.change(screen.getByLabelText('Phone number in international format'), { target: { value: '+15557654321' } }) })
  await act(async () => { await user.click(checkbox) })
  await act(async () => { await user.click(screen.getByRole('button', { name: 'Save customer endpoint' })) })
  expect(customerEndpointsApi.save).toHaveBeenCalledWith('ws', 'customer-id', expect.objectContaining({ endpoint_id: 'customer-phone', channel: 'sms', address: '+15557654321' }), true)
 })
})
