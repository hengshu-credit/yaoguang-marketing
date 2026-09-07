import { useEffect, useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { businessDefinitionDisplayName } from './builtin-event-labels'
import { businessApi, type BusinessDefinition, type BusinessDefinitionKind } from '../../services/api/business'

const catalogueKinds: BusinessDefinitionKind[] = ['customer_attribute', 'object_type', 'object_relation', 'event', 'event_mapping', 'state']

export function useBusinessCatalogue(workspaceId: string, refresh = 0) {
  const { i18n } = useLingui()
  const [result, setResult] = useState<{ workspaceId: string; refresh: number; definitions: BusinessDefinition[]; error?: Error }>()
  useEffect(() => {
    let current = true
    const loadKind = async (kind: BusinessDefinitionKind) => {
      const items: BusinessDefinition[] = []
      let total = 1
      while (current && items.length < total) {
        const result = await businessApi.list(workspaceId, kind, { limit: 100, offset: items.length })
        items.push(...(result.definitions ?? [])); total = result.total
        if (!result.definitions?.length) break
      }
      return items
    }
    Promise.all(catalogueKinds.map(loadKind)).then((lists) => { if (current) setResult({ workspaceId, refresh, definitions: lists.flat() }) }).catch((cause) => { if (current) setResult({ workspaceId, refresh, definitions: [], error: cause instanceof Error ? cause : new Error(String(cause)) }) })
    return () => { current = false }
  }, [workspaceId, refresh])
  const current = result?.workspaceId === workspaceId && result.refresh === refresh
  return { definitions: current ? result.definitions : [], displayName: (definition: BusinessDefinition) => businessDefinitionDisplayName(definition, i18n), loading: !current, error: current ? result.error : undefined }
}
