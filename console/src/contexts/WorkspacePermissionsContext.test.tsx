import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AuthContext, useWorkspacePermissions } from './AuthContext'
import { WorkspacePermissionsContext } from './WorkspacePermissionsContext'
import { createEmptyPermissions, createFullPermissions } from '../services/api/permissions'
import { workspaceService } from '../services/api/workspace'
import type { ReactNode } from 'react'

vi.mock('../services/api/auth', () => ({ authService: {}, isRootUser: () => false }))
vi.mock('../services/api/workspace', () => ({ workspaceService: { getMembers: vi.fn() } }))

const auth = { user: { id: 'u1', email: 'member@example.com' }, workspaces: [], isAuthenticated: true, loading: false, signin: vi.fn(), signout: vi.fn(), refreshWorkspaces: vi.fn() }
const allowed = createFullPermissions()
const denied = createEmptyPermissions()
function Consumer({ workspaceId = 'ws1' }: { workspaceId?: string }) {
  const { permissions, loading } = useWorkspacePermissions(workspaceId)
  return <output>{loading ? 'Loading permissions' : permissions?.templates?.write ? 'Allowed' : 'Denied'}</output>
}
function Scope({ children, workspaceId = 'ws1', userId = 'u1', permissions = allowed }: { children: ReactNode; workspaceId?: string; userId?: string; permissions?: typeof allowed }) {
  return <AuthContext.Provider value={auth}><WorkspacePermissionsContext.Provider value={{ workspaceId, userId, permissions, loading: false }}>{children}</WorkspacePermissionsContext.Provider></AuthContext.Provider>
}

describe('shared workspace permissions', () => {
  beforeEach(() => { vi.clearAllMocks(); vi.mocked(workspaceService.getMembers).mockReturnValue(new Promise(() => {})) })
  it('keeps permissions ready when a tab unmounts and another tab mounts', () => {
    const { rerender } = render(<Scope><Consumer key="templates" /></Scope>)
    expect(screen.getByText('Allowed')).toBeVisible()
    rerender(<Scope><Consumer key="blog" /></Scope>)
    expect(screen.queryByText('Loading permissions')).not.toBeInTheDocument()
    expect(screen.getByText('Allowed')).toBeVisible()
    expect(workspaceService.getMembers).not.toHaveBeenCalled()
  })
  it('immediately reflects a changed permission result', () => {
    const { rerender } = render(<Scope><Consumer /></Scope>)
    rerender(<Scope permissions={denied}><Consumer /></Scope>)
    expect(screen.getByText('Denied')).toBeVisible()
  })
  it.each([{ workspaceId: 'ws2', userId: 'u1' }, { workspaceId: 'ws1', userId: 'u2' }])('does not reuse another workspace or account result: %j', scope => {
    render(<Scope {...scope}><Consumer /></Scope>)
    expect(screen.getByText('Loading permissions')).toBeVisible()
    expect(screen.queryByText('Allowed')).not.toBeInTheDocument()
    expect(workspaceService.getMembers).toHaveBeenCalledWith('ws1')
  })
  it('still resolves permissions for consumers outside a workspace layout', async () => {
    vi.mocked(workspaceService.getMembers).mockResolvedValue({ members: [{ user_id: 'u1', permissions: allowed }] } as never)
    render(<AuthContext.Provider value={auth}><Consumer /></AuthContext.Provider>)
    await waitFor(() => expect(screen.getByText('Allowed')).toBeVisible())
    expect(workspaceService.getMembers).toHaveBeenCalledTimes(1)
  })
})
