import { useParams } from '@tanstack/react-router'
import { useWorkspacePermissions } from '../contexts/AuthContext'
import { DataAnalyticsPageShell } from '../components/navigation/DataAnalyticsPageShell'
import { FunnelsPage } from './FunnelsPage'

export function WorkspaceFunnelsPage() {
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId/funnels' })
  const { permissions, loading } = useWorkspacePermissions(workspaceId)
  return <DataAnalyticsPageShell workspaceId={workspaceId} activeKey="funnels"><FunnelsPage key={workspaceId} workspaceId={workspaceId} canManage={!loading && permissions?.workspace?.write === true} /></DataAnalyticsPageShell>
}
