import type { ReactNode } from 'react'
import { useLingui } from '@lingui/react/macro'
import { DataAnalyticsTabs, type DataAnalyticsTabKey } from './WorkspaceSectionTabs'

interface DataAnalyticsPageShellProps {
  workspaceId: string
  activeKey: DataAnalyticsTabKey
  actions?: ReactNode
  toolbar?: ReactNode
  children: ReactNode
  className?: string
}

export function DataAnalyticsPageShell({
  workspaceId,
  activeKey,
  actions,
  toolbar,
  children,
  className = 'p-4 md:p-6'
}: DataAnalyticsPageShellProps) {
  const { t } = useLingui()
  const descriptions: Record<DataAnalyticsTabKey, string> = {
    marketing: t`Track message performance, engagement and delivery trends across marketing channels.`,
    dashboard: t`Understand website traffic, acquisition quality and visitor behaviour over time.`,
    live: t`Monitor active sessions, locations, pages and acquisition sources in near real time.`,
    explore: t`Build a focused report by combining dimensions, metrics, filters and comparisons.`,
    goals: t`Measure the actions that matter and compare conversion performance across segments.`,
    filters: t`Map incoming traffic to channels and normalize source dimensions as sessions arrive.`,
    annotations: t`Record launches, campaigns and incidents so changes in charts retain their business context.`
  }
  const description = descriptions[activeKey]

  return (
    <div className={className}>
      <header
        data-testid="analytics-header"
        className="mb-5 flex flex-wrap items-start justify-between gap-3"
      >
        <div className="min-w-0">
          <p className="m-0 max-w-3xl text-sm text-gray-500">{description}</p>
        </div>
        {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
      </header>

      <div data-testid="analytics-tabs">
        <DataAnalyticsTabs workspaceId={workspaceId} activeKey={activeKey} />
      </div>

      {toolbar ? (
        <div data-testid="analytics-toolbar" className="mb-4">
          {toolbar}
        </div>
      ) : null}

      <main data-testid="analytics-content">{children}</main>
    </div>
  )
}
