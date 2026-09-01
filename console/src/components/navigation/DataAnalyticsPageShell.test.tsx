import { render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

vi.mock('./WorkspaceSectionTabs', () => ({
  DataAnalyticsTabs: () => <div>analytics tabs</div>
}))

import { DataAnalyticsPageShell } from './DataAnalyticsPageShell'

describe('DataAnalyticsPageShell', () => {
  it.each([
    ['marketing', 'Marketing Overview', 'Track message performance, engagement and delivery trends across marketing channels.'],
    ['dashboard', 'Website Overview', 'Understand website traffic, acquisition quality and visitor behaviour over time.'],
    ['live', 'Live Visitors', 'Monitor active sessions, locations, pages and acquisition sources in near real time.'],
    ['explore', 'Multidimensional Analysis', 'Build a focused report by combining dimensions, metrics, filters and comparisons.'],
    ['goals', 'Conversion Goals', 'Measure the actions that matter and compare conversion performance across segments.'],
    ['filters', 'Attribution Rules', 'Map incoming traffic to channels and normalize source dimensions as sessions arrive.'],
    ['annotations', 'Analytics Annotations', 'Record launches, campaigns and incidents so changes in charts retain their business context.']
  ] as const)('keeps only the description above the %s tab', (activeKey, title, description) => {
    render(
      <DataAnalyticsPageShell workspaceId="ws1" activeKey={activeKey}>
        content
      </DataAnalyticsPageShell>
    )

    const header = screen.getByTestId('analytics-header')
    expect(within(header).queryByRole('heading')).not.toBeInTheDocument()
    expect(header).not.toHaveTextContent(title)
    expect(within(header).getByText(description)).toBeInTheDocument()
  })

  it('keeps header, tabs, toolbar and content in one stable order', () => {
    const { container } = render(
      <DataAnalyticsPageShell workspaceId="ws1" activeKey="marketing" toolbar={<div>toolbar</div>}>
        content
      </DataAnalyticsPageShell>
    )

    expect(
      Array.from(container.firstElementChild?.children ?? []).map((node) =>
        node.getAttribute('data-testid')
      )
    ).toEqual(['analytics-header', 'analytics-tabs', 'analytics-toolbar', 'analytics-content'])
  })
})
