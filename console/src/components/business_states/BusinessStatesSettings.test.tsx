import { fireEvent, render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { BusinessStatesSettings } from './BusinessStatesSettings'
vi.mock('@lingui/react/macro', () => { const t = (strings: TemplateStringsArray) => strings.join(''); return { useLingui: () => ({ t }) } })
vi.mock('../business/useBusinessCatalogue', () => ({ useBusinessCatalogue: () => ({ definitions: [], loading: false }) }))
vi.mock('../../services/api/business', () => ({ businessApi: { save: vi.fn() } }))
describe('BusinessStatesSettings permissions', () => { it('prevents saving without management permission even when identifiers are populated', () => { render(<BusinessStatesSettings workspaceId="w" canManage={false} />); fireEvent.change(screen.getByLabelText('State key'), { target: { value: 'member' } }); fireEvent.change(screen.getByLabelText('State name'), { target: { value: 'Member' } }); expect(screen.getByRole('button', { name: 'Save state' })).toBeDisabled() }) })
