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
  const descriptors: Record<DataAnalyticsTabKey, { title: string; description: string }> = {
    marketing: {
      title: t`Marketing Overview`,
      description: t`Track message performance, engagement and delivery trends across marketing channels.`
    },
    dashboard: {
      title: t`Website Overview`,
      description: t`Understand website traffic, acquisition quality and visitor behaviour over time.`
    },
    live: {
      title: t`Live Visitors`,
      description: t`Monitor active sessions, locations, pages and acquisition sources in near real time.`
    },
    explore: {
      title: t`Multidimensional Analysis`,
      description: t`Build a focused report by combining dimensions, metrics, filters and comparisons.`
    },
    goals: {
      title: t`Conversion Goals`,
      description: t`Measure the actions that matter and compare conversion performance across segments.`
    },
    filters: {
      title: t`Attribution Rules`,
      description: t`Map incoming traffic to channels and normalize source dimensions as sessions arrive.`
    },
    annotations: {
      title: t`Analytics Annotations`,
      description: t`Record launches, campaigns and incidents so changes in charts retain their business context.`
    }
  }
  const descriptor = descriptors[activeKey]

  return (
    <div className={className}>
      <header
        data-testid="analytics-header"
        className="mb-5 flex flex-wrap items-start justify-between gap-3"
      >
        <div className="min-w-0">
          <h1 className="m-0 text-2xl font-medium text-gray-900">{descriptor.title}</h1>
          <p className="mb-0 mt-1 max-w-3xl text-sm text-gray-500">{descriptor.description}</p>
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
