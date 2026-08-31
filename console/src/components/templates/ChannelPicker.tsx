import { useMemo, useState } from 'react'
import { Tag } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { ChannelDefinition } from '../../services/api/channels'

interface ChannelPickerProps {
  definitions: ChannelDefinition[]
  country?: string
  value?: string
  onSelect: (channelId: string) => void
}

const ChannelPicker: React.FC<ChannelPickerProps> = ({ definitions, country, value, onSelect }) => {
  const { t } = useLingui()
  const [scope, setScope] = useState(country ? 'recommended' : 'all')
  const [search, setSearch] = useState('')
  const normalizedCountry = country?.trim().toUpperCase()
  const visible = useMemo(() => {
    const query = search.trim().toLocaleLowerCase()
    return definitions.filter((definition) => {
      if (
        scope === 'recommended' &&
        normalizedCountry &&
        !definition.recommended_in?.includes(normalizedCountry)
      ) {
        return false
      }
      return !query || definition.label_key.toLocaleLowerCase().includes(query)
    })
  }, [definitions, normalizedCountry, scope, search])

  return (
    <div>
      <div role="tablist" aria-label={t`Channel scope`} className="mb-4 flex gap-2 border-b border-gray-200">
        {normalizedCountry && (
          <button
            type="button"
            role="tab"
            aria-selected={scope === 'recommended'}
            className={`border-b-2 px-3 py-2 ${scope === 'recommended' ? 'border-blue-600 text-blue-600' : 'border-transparent'}`}
            onClick={() => setScope('recommended')}
          >
            {t`Recommended for ${normalizedCountry}`}
          </button>
        )}
        <button
          type="button"
          role="tab"
          aria-selected={scope === 'all'}
          className={`border-b-2 px-3 py-2 ${scope === 'all' ? 'border-blue-600 text-blue-600' : 'border-transparent'}`}
          onClick={() => setScope('all')}
        >
          {t`All channels`}
        </button>
      </div>
      <input
        type="search"
        value={search}
        placeholder={t`Search channels`}
        onChange={(event) => setSearch(event.target.value)}
        className="mb-4 w-full rounded-md border border-gray-300 px-3 py-2"
      />
      {visible.length === 0 ? (
        <div className="rounded-md border border-dashed border-gray-300 p-8 text-center text-gray-500">
          {t`No channels match`}
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {visible.map((definition) => (
            <button
              key={definition.id}
              type="button"
              aria-pressed={value === definition.id}
              className={`min-h-20 rounded-md border p-4 text-left ${value === definition.id ? 'border-blue-600 bg-blue-50' : 'border-gray-200 bg-white'}`}
              onClick={() => onSelect(definition.id)}
            >
              <span className="flex w-full flex-col gap-2">
                <strong>{definition.label_key}</strong>
                <span className="flex flex-wrap gap-1">
                  {definition.delivery_modes.includes('native') && <Tag color="blue">{t`Native`}</Tag>}
                  {definition.delivery_modes.includes('signed_webhook') && (
                    <Tag color="purple">{t`Signed Webhook`}</Tag>
                  )}
                </span>
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

export default ChannelPicker
