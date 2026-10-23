import { createContext } from 'react'
import type { UserPermissions } from '../services/api/types'

export interface WorkspacePermissionsSnapshot {
  workspaceId: string
  userId: string
  permissions: UserPermissions | null
  loading: boolean
}

// The persistent workspace layout owns this result across child-route changes.
export const WorkspacePermissionsContext = createContext<WorkspacePermissionsSnapshot | null>(null)
