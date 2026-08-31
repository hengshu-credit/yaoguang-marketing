import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import BlogPageHeader from './BlogPageHeader'

vi.mock('../navigation/WorkspaceSectionTabs', () => ({ ContentCenterTabs: () => <nav>content tabs</nav> }))

describe('BlogPageHeader', () => {
  it('places the category heading before the content-center tabs', () => {
    render(<BlogPageHeader workspaceId="ws1" />)
    const heading = screen.getByRole('heading', { name: /Categories/i })
    const tabs = screen.getByRole('navigation')
    expect(heading.compareDocumentPosition(tabs) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })
})

