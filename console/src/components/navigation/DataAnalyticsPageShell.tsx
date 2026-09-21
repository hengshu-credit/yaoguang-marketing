import type { ReactNode } from 'react'
import { useLingui } from '@lingui/react/macro'
import { DataAnalyticsTabs, type DataAnalyticsTabKey } from './WorkspaceSectionTabs'
import { WorkspacePageHeader } from './WorkspacePageHeader'

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
  className = ''
}: DataAnalyticsPageShellProps) {
  const { t } = useLingui()
  const descriptions: Record<DataAnalyticsTabKey, string> = {
    funnels: t`Build conversion funnels from events for customers and configured business objects.`,
    marketing: t`Track message performance, engagement and delivery trends across marketing channels.`,
    dashboard: t`Understand website traffic, acquisition quality and visitor behaviour over time.`,
    live: t`Monitor active sessions, locations, pages and acquisition sources in near real time.`,
    explore: t`Build a focused report by combining dimensions, metrics, filters and comparisons.`,
    goals: t`Measure the actions that matter and compare conversion performance across segments.`,
    filters: t`Map incoming traffic to channels and normalize source dimensions as sessions arrive.`,
    annotations: t`Record launches, campaigns and incidents so changes in charts retain their business context.`
  }
  const description = descriptions[activeKey]
  const titles: Record<DataAnalyticsTabKey,string> = {funnels:t`Conversion funnels`,marketing:t`Marketing Overview`,dashboard:t`Website Overview`,live:t`Live Visitors`,explore:t`Multidimensional Analysis`,goals:t`Conversion Goals`,filters:t`Attribution Rules`,annotations:t`Analytics Annotations`}

  return (
    <div className={className}>
      <WorkspacePageHeader testId="analytics-header" title={titles[activeKey]} description={description} actions={actions} />

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
