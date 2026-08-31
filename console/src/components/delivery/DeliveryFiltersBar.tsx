import { useMemo, useState } from 'react'
import { Button, Input, Popover, Select, Space, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { DeliveryStatus } from '../../services/api/delivery'
import { deliveryStatusOptions, type DeliveryFilters } from './deliveryFilters'

type DeliveryFilterKey = keyof DeliveryFilters | 'delivery_time'

interface DeliveryFilterOption {
  key: DeliveryFilterKey
  label: string
  placeholder?: string
  options?: Array<{ value: string; label: string }>
}

interface DeliveryFiltersBarProps {
  filters: DeliveryFilters
  statusLabels: Record<DeliveryStatus, string>
  onChange: (filters: DeliveryFilters) => void
}

export function DeliveryFiltersBar({ filters, statusLabels, onChange }: DeliveryFiltersBarProps) {
  const { t } = useLingui()
  const [draft, setDraft] = useState<DeliveryFilters>(filters)
  const [openFilter, setOpenFilter] = useState<DeliveryFilterKey>()
  const [filterError, setFilterError] = useState<string>()

  const statusFilter: DeliveryFilterOption = useMemo(() => ({
    key: 'status',
    label: t`Status`,
    options: deliveryStatusOptions.map((status) => ({ value: status, label: statusLabels[status] }))
  }), [statusLabels, t])

  const businessFilters: DeliveryFilterOption[] = useMemo(() => [
    {
      key: 'channel',
      label: t`Channel`,
      options: [
        { value: 'email', label: t`Email` },
        { value: 'sms', label: t`SMS` },
        { value: 'push', label: t`Push` },
        { value: 'whatsapp', label: 'WhatsApp' },
        { value: 'telegram', label: 'Telegram' },
        { value: 'in_app', label: t`In-App` },
        { value: 'webhook', label: 'Webhook' }
      ]
    },
    {
      key: 'source_type',
      label: t`Source type`,
      options: [
        { value: 'automation', label: t`Automation Journey` },
        { value: 'campaign', label: t`Marketing Campaign` },
        { value: 'broadcast', label: t`Email Broadcast` },
        { value: 'api', label: 'API' },
        { value: 'legacy', label: t`Legacy delivery` }
      ]
    },
    { key: 'source_id', label: t`Source ID`, placeholder: t`Source ID` },
    { key: 'customer_id', label: t`Customer`, placeholder: t`Customer ID or number` },
    { key: 'provider', label: t`Provider`, placeholder: t`Provider name` },
    { key: 'delivery_time', label: t`Delivery time` }
  ], [t])

  const isFilterActive = (key: DeliveryFilterKey) => key === 'delivery_time'
    ? Boolean(filters.from || filters.to)
    : Boolean(filters[key])

  const displayFilterValue = (option: DeliveryFilterOption) => {
    if (option.key === 'delivery_time') {
      if (filters.from && filters.to) return `${filters.from.replace('T', ' ')} – ${filters.to.replace('T', ' ')}`
      if (filters.from) return `${t`From`} ${filters.from.replace('T', ' ')}`
      return `${t`To`} ${filters.to?.replace('T', ' ')}`
    }
    const value = filters[option.key]
    return option.options?.find((item) => item.value === value)?.label ?? value
  }

  const applyFilter = (key: DeliveryFilterKey) => {
    if (key === 'delivery_time' && draft.from && draft.to && new Date(draft.from) >= new Date(draft.to)) {
      setFilterError(t`End time must be after start time`)
      return
    }

    const next = { ...filters }
    if (key === 'delivery_time') {
      next.from = draft.from || undefined
      next.to = draft.to || undefined
    } else {
      const value = draft[key]
      if (typeof value === 'string' && value.trim()) next[key] = value.trim() as never
      else delete next[key]
    }
    onChange(next)
    setFilterError(undefined)
    setOpenFilter(undefined)
  }

  const clearFilter = (key: DeliveryFilterKey) => {
    const nextFilters = { ...filters }
    const nextDraft = { ...draft }
    if (key === 'delivery_time') {
      delete nextFilters.from
      delete nextFilters.to
      delete nextDraft.from
      delete nextDraft.to
    } else {
      delete nextFilters[key]
      delete nextDraft[key]
    }
    onChange(nextFilters)
    setDraft(nextDraft)
    setFilterError(undefined)
    setOpenFilter(undefined)
  }

  const renderFilterInput = (option: DeliveryFilterOption) => {
    if (option.key === 'delivery_time') {
      return (
        <Space orientation="vertical" size="small" style={{ width: '100%' }}>
          <label>
            <Typography.Text>{t`From`}</Typography.Text>
            <Input type="datetime-local" value={draft.from} onChange={(event) => setDraft((current) => ({ ...current, from: event.target.value || undefined }))} />
          </label>
          <label>
            <Typography.Text>{t`To`}</Typography.Text>
            <Input type="datetime-local" value={draft.to} onChange={(event) => setDraft((current) => ({ ...current, to: event.target.value || undefined }))} />
          </label>
        </Space>
      )
    }

    if (option.options) {
      return (
        <Select
          allowClear
          value={draft[option.key] as string | undefined}
          onChange={(value) => setDraft((current) => ({ ...current, [option.key]: value }))}
          options={option.options}
          placeholder={t`Select a value`}
          style={{ width: '100%' }}
        />
      )
    }

    return (
      <Input
        value={draft[option.key] as string | undefined}
        onChange={(event) => setDraft((current) => ({ ...current, [option.key]: event.target.value }))}
        placeholder={option.placeholder}
      />
    )
  }

  const renderFilterButton = (option: DeliveryFilterOption) => {
    const active = isFilterActive(option.key)
    return (
      <Popover
        key={option.key}
        trigger="click"
        placement="bottomLeft"
        open={openFilter === option.key}
        onOpenChange={(open) => {
          if (open) {
            setDraft(filters)
            setFilterError(undefined)
          }
          setOpenFilter(open ? option.key : undefined)
        }}
        content={
          <Space orientation="vertical" size="small" style={{ width: option.key === 'delivery_time' ? 320 : 240 }}>
            {renderFilterInput(option)}
            {filterError && <Typography.Text type="danger" role="alert">{filterError}</Typography.Text>}
            <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
              {active && <Button danger size="small" onClick={() => clearFilter(option.key)}>{t`Clear`}</Button>}
              <Button type="primary" size="small" onClick={() => applyFilter(option.key)}>{t`Apply`}</Button>
            </Space>
          </Space>
        }
      >
        <Button type={active ? 'primary' : 'default'} size="small">
          {active ? `${option.label}: ${displayFilterValue(option)}` : option.label}
        </Button>
      </Popover>
    )
  }

  return (
    <Space orientation="vertical" size="small" style={{ width: '100%' }}>
      <Space wrap>
        <Typography.Text strong>{t`Status`}:</Typography.Text>
        {renderFilterButton(statusFilter)}
      </Space>
      <Space wrap>
        <Typography.Text strong>{t`Filters`}:</Typography.Text>
        {businessFilters.map(renderFilterButton)}
        {Object.values(filters).some(Boolean) && (
          <Button size="small" onClick={() => { onChange({}); setDraft({}); setOpenFilter(undefined) }}>{t`Clear all`}</Button>
        )}
      </Space>
    </Space>
  )
}
