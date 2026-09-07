import { render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { SubjectStatesPanel } from './SubjectStatesPanel'
vi.mock('@lingui/react/macro', () => { const t = (strings: TemplateStringsArray) => strings.join(''); return { useLingui: () => ({ t }) } })
vi.mock('../../services/api/subject_states', () => ({ subjectStatesApi: { list: vi.fn().mockResolvedValue({ states: [{ state_key: 'verified', value: 'true', source: 'rule', valid_from: '2026-09-07T00:00:00Z' }, { state_key: 'member', value: 'true', source: 'manual', reason: 'Reviewed', valid_from: '2026-09-07T00:00:00Z' }] }) } }))
describe('SubjectStatesPanel', () => { it('shows simultaneous independent states and their actual sources', async () => { render(<SubjectStatesPanel workspaceId="w" subjectType="contract" subjectId="A" />); await waitFor(() => expect(screen.getByText('verified')).toBeTruthy()); expect(screen.getByText('member')).toBeTruthy(); expect(screen.getAllByText('Satisfied')).toHaveLength(2); expect(screen.getByText('Reviewed')).toBeTruthy() }) })
