export type FilterType = 'string' | 'number' | 'date' | 'boolean'

export interface FilterField {
  key: string
  label: string
  type: FilterType
  options?: { label: string; value: string | number | boolean }[]
}

export interface FilterValue {
  field: string
  value: string | number | boolean | Date
  label: string
}

export interface FilterProps {
  fields: FilterField[]
  activeFilters: FilterValue[]
  className?: string
}

export interface FilterInputProps {
  field: FilterField
  value?: string | number | boolean | Date
  onChange: (value: string | number | boolean | Date) => void
  className?: string
}

export interface ActiveFiltersProps {
  filters: FilterValue[]
  onRemove: (field: string) => void
  className?: string
}
