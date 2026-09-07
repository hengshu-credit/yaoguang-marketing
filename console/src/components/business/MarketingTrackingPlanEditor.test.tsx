import { useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { MarketingTrackingPlan } from '../../services/api/marketing_tracking'
import { MarketingTrackingPlanEditor } from './MarketingTrackingPlanEditor'

vi.mock('./useBusinessCatalogue', () => ({ useBusinessCatalogue: () => ({ definitions: [{ kind: 'event', key: 'login', name: 'Login', status: 'active', settings: { subject_type: 'customer' } }, { kind: 'object_type', key: 'contract', name: 'Contract', status: 'active', settings: {} }], loading: false, displayName: (item: { name: string }) => item.name }) }))
function Example() {
  const [value, setValue] = useState<MarketingTrackingPlan | null>()
  return <><MarketingTrackingPlanEditor workspaceId="ws" value={value} onChange={setValue} /><output data-testid="plan">{JSON.stringify(value)}</output></>
}
describe('MarketingTrackingPlanEditor', () => {
  it('keeps a task tracking plan optional and enables a specific observation window', () => {
    render(<Example />)
    expect(screen.queryByRole('combobox', { name: 'Tracked events' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Configure conversion tracking' }))
    expect(screen.getByRole('combobox', { name: 'Tracked events' })).toBeVisible()
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Observation window in seconds' }), { target: { value: '3600' } })
    expect(JSON.parse(screen.getByTestId('plan').textContent!).window_seconds).toBe(3600)
    fireEvent.click(screen.getByRole('button', { name: 'Remove task tracking' }))
    expect(screen.getByTestId('plan')).toHaveTextContent('null')
  })
})
