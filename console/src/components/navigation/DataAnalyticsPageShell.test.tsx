import { render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

vi.mock('./WorkspaceSectionTabs', () => ({
  DataAnalyticsTabs: () => <div>analytics tabs</div>
}))

import { DataAnalyticsPageShell } from './DataAnalyticsPageShell'

describe('DataAnalyticsPageShell', () => {
  it.each([
    ['marketing', 'Marketing Overview'],
    ['dashboard', 'Website Overview'],
    ['live', 'Live Visitors'],
    ['explore', 'Multidimensional Analysis'],
    ['goals', 'Conversion Goals'],
    ['filters', 'Attribution Rules'],
    ['annotations', 'Analytics Annotations']
  ] as const)('renders one title and description for %s', (activeKey, title) => {
    render(
      <DataAnalyticsPageShell workspaceId="ws1" activeKey={activeKey}>
        content
      </DataAnalyticsPageShell>
    )

    expect(screen.getByRole('heading', { level: 1, name: title })).toBeInTheDocument()
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1)
    expect(screen.getByTestId('analytics-header')).toHaveTextContent(title)
    expect(within(screen.getByTestId('analytics-header')).getByText((_, node) => node?.tagName === 'P')).not.toBeEmptyDOMElement()
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
