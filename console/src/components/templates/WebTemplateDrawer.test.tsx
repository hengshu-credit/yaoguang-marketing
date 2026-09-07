import React from 'react'
import { App } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { templatesApi } from '../../services/api/template'
import type { Workspace } from '../../services/api/types'
import WebTemplateDrawer from './WebTemplateDrawer'
import { webTemplateHtml, webPreviewDocument } from './webTemplateContent'

vi.mock('../blog_editor', () => ({ NotifuseEditor: React.forwardRef(({ initialContent, onChange }: { initialContent: string; onChange: (content: Record<string, unknown>) => void }, ref) => {
  React.useImperativeHandle(ref, () => ({ getHTML: () => '' }))
  return <textarea aria-label="Web content" defaultValue={initialContent} onChange={event => onChange({ type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: event.target.value }] }] })} />
}) }))
vi.mock('./TemplateCategorySelect', () => ({ default: () => null }))
const mocks = vi.hoisted(() => ({ create: vi.fn(), update: vi.fn() }))
vi.mock('../../services/api/template', () => ({ templatesApi: mocks }))
const workspace = { id: 'ws1', settings: { default_language: 'en', languages: ['en', 'zh'] } } as Workspace
const renderDrawer = () => render(<QueryClientProvider client={new QueryClient()}><App><WebTemplateDrawer workspace={workspace} /></App></QueryClientProvider>)

describe('native Web material', () => {
  beforeEach(() => { vi.clearAllMocks(); mocks.create.mockResolvedValue({ template: {} }) })
  it('renders a Web draft and saves native web content, html and text', async () => {
    const { container } = renderDrawer()
    await userEvent.click(screen.getByRole('button', { name: 'Create template' }))
    fireEvent.change(screen.getByLabelText('Web content'), { target: { value: 'Native page' } })
    expect(container.ownerDocument.querySelector('iframe')).toHaveAttribute('srcdoc', expect.stringContaining('<p>Native page</p>'))
    await userEvent.type(screen.getByLabelText('API ID'), 'native-web')
    await userEvent.type(screen.getByLabelText('Name'), 'Native Web')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(templatesApi.create).toHaveBeenCalledWith(expect.objectContaining({ channel: 'web', web: { content: { type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'Native page' }] }] }, html: '<p>Native page</p>', plain_text: 'Native page' } })))
    expect(mocks.create.mock.calls[0][0]).not.toHaveProperty('content_schema_version')
  })

  it('preserves distinct content when switching languages', async () => {
    renderDrawer()
    await userEvent.click(screen.getByRole('button', { name: 'Create template' }))
    fireEvent.change(screen.getByLabelText('Web content'), { target: { value: 'English page' } })
    await userEvent.click(screen.getByRole('tab', { name: 'zh' }))
    fireEvent.change(screen.getByLabelText('Web content'), { target: { value: '中文网页' } })
    await userEvent.click(screen.getByRole('tab', { name: 'en (default)' }))
    expect(screen.getByLabelText('Web content')).toHaveValue('<p>English page</p>')
  })

  it('can preview legacy Web JSON without a cached HTML value', () => {
    expect(webTemplateHtml({ content: { type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: '<safe>' }] }] } })).toBe('<p>&lt;safe&gt;</p>')
    const complete = '<!doctype html><html><body>Styled page</body></html>'
    expect(webPreviewDocument(complete)).toBe(complete)
  })
})
