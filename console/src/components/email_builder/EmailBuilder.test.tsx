import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, screen, waitFor } from '@testing-library/react'
import EmailBuilder from './EmailBuilder'
import type { EmailBlock } from './types'
import type { EmailPreviewMetadata } from '../preview'

// Mock the heavy child panels so the test exercises only EmailBuilder's own
// compile orchestration (not Preview's iframe, the tree/edit/settings panels,
// or overlay scrollbars).
vi.mock('./panels/TreePanel', () => ({ TreePanel: () => null }))
vi.mock('./panels/EditPanel', () => ({ EditPanel: () => null }))
vi.mock('./panels/SettingsPanel', () => ({ SettingsPanel: () => null }))
vi.mock('./panels/Preview', () => ({
  // forwardRef so EmailBuilder passing templateDataRef doesn't warn
  Preview: React.forwardRef(({ html, errors, emailPreview }: { html: string; errors?: { message: string }[]; emailPreview?: EmailPreviewMetadata }) => <div><div data-testid="compiled-email">{html}{errors?.[0]?.message}</div><div data-testid="email-metadata">{JSON.stringify(emailPreview)}</div></div>)
}))
vi.mock('overlayscrollbars-react', () => ({
  OverlayScrollbarsComponent: ({ children }: { children?: React.ReactNode }) => <>{children}</>
}))

// Minimal valid email tree; `marker` lets each version differ so JSON.stringify
// (the recompile key) changes between renders.
const makeTree = (marker: string): EmailBlock =>
  ({
    id: 'root',
    type: 'mjml',
    children: [
      { id: 'head', type: 'mj-head', children: [] },
      {
        id: 'body',
        type: 'mj-body',
        attributes: {},
        children: [{ id: 'text-1', type: 'mj-text', content: marker, children: [] }]
      }
    ]
  } as unknown as EmailBlock)

const props = (
  tree: EmailBlock,
  onCompile: ReturnType<typeof vi.fn>,
  forcedViewMode: 'edit' | 'preview' | null
) => ({
  tree,
  onTreeChange: vi.fn(),
  onCompile,
  testData: undefined,
  onTestDataChange: vi.fn(),
  onSaveBlock: vi.fn(),
  forcedViewMode
})

describe('EmailBuilder preview auto-recompile', () => {
  let onCompile: ReturnType<typeof vi.fn>

  beforeEach(() => {
    onCompile = vi.fn().mockResolvedValue({ html: '<html></html>', mjml: '<mjml></mjml>' })
  })

  it('compiles once on entering preview instead of sending duplicate requests', async () => {
    render(<EmailBuilder {...props(makeTree('one'), onCompile, 'preview')} />)
    await screen.findByTestId('compiled-email')
    expect(onCompile).toHaveBeenCalledTimes(1)
  })

  it('uses resolved subject and summary from the same compile as the body, then refreshes changed settings', async () => {
    onCompile.mockResolvedValueOnce({ html: 'First body', mjml: '', subject: 'Hello Ada', subject_preview: 'Benefits for Ada' })
      .mockResolvedValueOnce({ html: 'Updated body', mjml: '', subject: 'New Ada', subject_preview: 'New benefits' })
    const base = props(makeTree('same body'), onCompile, 'preview')
    const { rerender } = render(<EmailBuilder {...base} emailPreview={{ senderName: 'First sender', subject: 'Hello {{name}}', previewText: 'Benefits for {{name}}' }} />)
    await screen.findByText('First body')
    expect(screen.getByTestId('email-metadata')).toHaveTextContent('Hello Ada')
    expect(screen.getByTestId('email-metadata')).toHaveTextContent('Benefits for Ada')
    rerender(<EmailBuilder {...base} emailPreview={{ senderName: 'New sender', subject: 'New {{name}}', previewText: 'New benefits' }} />)
    await screen.findByText('Updated body')
    expect(screen.getByTestId('email-metadata')).toHaveTextContent('New sender')
    expect(screen.getByTestId('email-metadata')).toHaveTextContent('New Ada')
    expect(onCompile).toHaveBeenCalledTimes(2)
  })

  it('ignores stale compilation responses after a newer tree is displayed', async () => {
    let finishOld!: (value: { html: string; mjml: string }) => void
    onCompile.mockImplementationOnce(() => new Promise(resolve => { finishOld = resolve })).mockResolvedValueOnce({ html: 'Latest email', mjml: '' })
    const { rerender } = render(<EmailBuilder {...props(makeTree('old'), onCompile, 'preview')} />)
    await waitFor(() => expect(onCompile).toHaveBeenCalledTimes(1))
    rerender(<EmailBuilder {...props(makeTree('new'), onCompile, 'preview')} />)
    expect(await screen.findByText('Latest email')).toBeVisible()
    await act(async () => finishOld({ html: 'Old email', mjml: '' }))
    expect(screen.queryByText('Old email')).not.toBeInTheDocument()
    expect(screen.getByText('Latest email')).toBeVisible()
  })

  it('recompiles the preview when the tree changes while in preview mode', async () => {
    const { rerender } = render(<EmailBuilder {...props(makeTree('one'), onCompile, 'preview')} />)
    // Flush the initial mount compile, then isolate the tree-change behavior.
    await waitFor(() => expect(onCompile).toHaveBeenCalled())
    onCompile.mockClear()

    const updated = makeTree('two')
    rerender(<EmailBuilder {...props(updated, onCompile, 'preview')} />)

    await waitFor(() => expect(onCompile).toHaveBeenCalledWith(updated, undefined))
  })

  it('does NOT recompile when the tree changes in edit mode', async () => {
    const { rerender } = render(<EmailBuilder {...props(makeTree('one'), onCompile, null)} />)
    // Edit mode must not compile on mount...
    expect(onCompile).not.toHaveBeenCalled()

    rerender(<EmailBuilder {...props(makeTree('two'), onCompile, null)} />)
    // ...nor after a tree change, even past the debounce window.
    await new Promise((resolve) => setTimeout(resolve, 600))
    expect(onCompile).not.toHaveBeenCalled()
  })

  it('coalesces a rapid burst of tree changes into a single recompile (debounce)', async () => {
    const { rerender } = render(<EmailBuilder {...props(makeTree('a'), onCompile, 'preview')} />)
    await waitFor(() => expect(onCompile).toHaveBeenCalled())
    onCompile.mockClear()

    // Three quick edits (mimics a multi-step AI burst) before the debounce fires.
    rerender(<EmailBuilder {...props(makeTree('b'), onCompile, 'preview')} />)
    rerender(<EmailBuilder {...props(makeTree('c'), onCompile, 'preview')} />)
    const last = makeTree('d')
    rerender(<EmailBuilder {...props(last, onCompile, 'preview')} />)

    await waitFor(() => expect(onCompile).toHaveBeenCalledTimes(1))
    expect(onCompile).toHaveBeenCalledWith(last, undefined)

    // No further compiles after the debounce settles.
    await new Promise((resolve) => setTimeout(resolve, 250))
    expect(onCompile).toHaveBeenCalledTimes(1)
  })
})
