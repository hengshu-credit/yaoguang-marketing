import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { StateCoveragePanel } from './StateCoveragePanel'
import { subjectStatesApi } from '../../services/api/subject_states'
vi.mock('@lingui/react/macro', () => { const t = (s: TemplateStringsArray) => s.join(''); return { useLingui: () => ({ t }) } })
vi.mock('../business/useBusinessCatalogue', () => ({ useBusinessCatalogue: () => ({ definitions: [{ kind: 'state', key: 'X', name: 'Independent X', settings: { subject_type: 'customer' } }, { kind: 'state', key: 'Y', name: 'Independent Y', settings: { subject_type: 'customer' } }] }) }))
vi.mock('../../services/api/subject_states', () => ({ subjectStatesApi: { coverage: vi.fn().mockResolvedValue({ total: 3, intersection: 1, union: 2, states: [{ state_key: 'X', satisfied: 2, false: 0, unknown: 1 }, { state_key: 'Y', satisfied: 1, false: 1, unknown: 1 }] }) } }))
it('shows independent state counts and keeps unknown subjects in the cohort', async () => {
 render(<StateCoveragePanel workspaceId="w" />)
 fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Selected states' }))
 fireEvent.click(await screen.findByText('Independent X'))
 fireEvent.click(screen.getByText('Independent Y'))
 fireEvent.click(screen.getByRole('button', { name: 'Analyze state coverage' }))
 await waitFor(() => expect(subjectStatesApi.coverage).toHaveBeenCalled())
 expect(await screen.findByText('Total subjects')).toBeTruthy()
 expect(screen.getByText('All selected states satisfied')).toBeTruthy()
 expect(screen.getByRole('columnheader', { name: 'Unknown' })).toBeTruthy()
})
