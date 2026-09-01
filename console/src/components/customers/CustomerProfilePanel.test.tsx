import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { I18nProvider } from '@lingui/react'
import { describe, expect, it, vi } from 'vitest'
import { i18n } from '../../i18n'
import type { Customer } from '../../services/api/customer'
import { CustomerProfilePanel } from './CustomerProfilePanel'

const customer: Customer = {
  customer_id: 'customer-1',
  customer_no: 'U0001202608301000000811111111111141118111111111111111',
  external_user_id: 'crm-42',
  version: 3,
  profile: {
    customer_id: 'customer-1',
    version: 3,
    status: 'active',
    attributes: { tier: 'gold' },
    created_at: '2026-08-30T00:00:00Z',
    updated_at: '2026-08-30T00:00:00Z'
  },
  identities: [],
  tags: ['vip'],
  list_memberships: [],
  audience_memberships: [],
  consents: [],
  created_at: '2026-08-30T00:00:00Z',
  updated_at: '2026-08-30T00:00:00Z'
}

function renderPanel(onUpdate = vi.fn(async () => undefined), canWrite = true) {
  render(
    <I18nProvider i18n={i18n}>
      <CustomerProfilePanel customer={customer} canWrite={canWrite} saving={false} onUpdate={onUpdate} />
    </I18nProvider>
  )
  return onUpdate
}

describe('CustomerProfilePanel', () => {
  it('renders profile attribute names and values side by side', () => {
    renderPanel()

    const attributeName = screen.getByText('tier')
    const attributeValue = screen.getByText('gold')
    const attributeRow = attributeName.closest('.group')

    expect(attributeRow).toHaveClass('grid', 'grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]')
    expect(attributeName).toHaveClass('text-left')
    expect(attributeValue).toHaveClass('text-right')
  })

  it('updates an existing profile field inline', async () => {
    const onUpdate = renderPanel()

    fireEvent.click(screen.getByRole('button', { name: 'Edit Status' }))
    fireEvent.change(screen.getByDisplayValue('active'), { target: { value: 'inactive' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Status' }))

    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith({ profile: { status: 'inactive' } }))
  })

  it('adds a typed profile attribute inline', async () => {
    const onUpdate = renderPanel()

    fireEvent.click(screen.getByRole('button', { name: 'Add profile attribute' }))
    fireEvent.change(screen.getByPlaceholderText('Attribute name'), { target: { value: 'score' } })
    fireEvent.change(screen.getByPlaceholderText('Value (text or JSON)'), { target: { value: '88' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add' }))

    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith({
      profile: { attributes: { merge: { score: 88 } } }
    }))
  })

  it('removes a profile attribute through the unset patch', async () => {
    const onUpdate = renderPanel()

    fireEvent.click(screen.getByRole('button', { name: 'Remove tier' }))
    fireEvent.click(await screen.findByRole('button', { name: 'OK' }))

    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith({
      profile: { attributes: { unset: ['tier'] } }
    }))
  })

  it('hides every editing entry point without Customer write permission', () => {
    renderPanel(vi.fn(async () => undefined), false)

    expect(screen.queryByRole('button', { name: /^(Edit|Remove|Add profile attribute)/ })).not.toBeInTheDocument()
  })
})
