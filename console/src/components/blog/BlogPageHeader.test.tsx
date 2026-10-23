import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import BlogPageHeader from './BlogPageHeader'

vi.mock('../navigation/WorkspaceSectionTabs', () => ({ ContentCenterTabs: () => <nav>content tabs</nav> }))

describe('BlogPageHeader', () => {
  it('places the article heading before the content-center tabs', () => {
    render(<BlogPageHeader workspaceId="ws1" />)
    const heading = screen.getByRole('heading', { level: 1, name: /Blog Content/i })
    const tabs = screen.getByRole('navigation')
    expect(heading.style.fontSize).toBe('var(--console-title-size, 22px)')
    expect(heading.style.fontWeight).toBe('600')
    expect(heading).toHaveClass('workspace-page-title')
    expect(heading.parentElement).toHaveClass('mb-6')
    expect(heading.compareDocumentPosition(tabs) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })
})

