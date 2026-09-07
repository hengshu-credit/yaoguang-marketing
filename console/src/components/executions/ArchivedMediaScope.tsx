import { useContext, type ReactNode } from 'react'
import { ArchivedMedia } from './executionMediaContext'
import { useQuery } from '@tanstack/react-query'
import { executionsApi } from '../../services/api/executions'
import { ActionableError } from '../errors/ActionableError'
import { record } from './executionPresentation'
import { archivedMediaDataUrls } from './executionMediaPreview'
export function ArchivedMediaScope({ workspaceId, executionId, materials, children }: { workspaceId: string; executionId: string; materials: unknown; children: ReactNode }) {
 const parent = useContext(ArchivedMedia)
 const rawAssets = record(materials).assets
 const assets = Array.isArray(rawAssets) ? rawAssets.map(record).filter((asset) => typeof asset.source_url === 'string' && typeof asset.hash === 'string') : []
 const query = useQuery({ queryKey: ['execution-preview-media', workspaceId, executionId, assets.map((asset) => [asset.source_url, asset.hash])], queryFn: async () => archivedMediaDataUrls(new Map(await Promise.all(assets.map(async (asset): Promise<[string, Blob]> => [String(asset.source_url), await executionsApi.asset(workspaceId, executionId, String(asset.hash))])))), enabled: assets.length > 0, staleTime: Infinity })
 return <ArchivedMedia.Provider value={{ ...parent, ...query.data }}>{query.isError && <ActionableError error={query.error} onRetry={() => void query.refetch()} />}{children}</ArchivedMedia.Provider>
}
