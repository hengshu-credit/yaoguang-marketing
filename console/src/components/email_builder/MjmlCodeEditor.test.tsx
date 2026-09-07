import { act, render, screen, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { describe, expect, it, vi } from 'vitest'
import MjmlCodeEditor from './MjmlCodeEditor'
import type { EmailPreviewMetadata } from '../preview'

vi.mock('@monaco-editor/react', () => ({ default: () => null }))
vi.mock('../preview', () => ({ EmailPreview: ({ html, email }: { html: string; email?: EmailPreviewMetadata }) => <div><div data-testid="code-email-body">{html}</div><div data-testid="code-email-header">{JSON.stringify(email)}</div></div> }))

describe('MJML preview updates', () => {
  it('refreshes settings metadata and ignores an older source response', async () => {
    let finishOld!: (value: { html: string; mjml: string; subject: string }) => void
    const compile = vi.fn().mockImplementationOnce(() => new Promise(resolve => { finishOld = resolve })).mockResolvedValueOnce({ html: 'New compiled body', mjml: '', subject: 'New Ada', subject_preview: 'New summary' })
    const props = { onMjmlSourceChange: vi.fn(), onTestDataChange: vi.fn(), onCompile: compile }
    const { rerender } = render(<App><MjmlCodeEditor {...props} mjmlSource="<mjml>Old</mjml>" emailPreview={{ subject: 'Old {{name}}', senderName: 'Sender' }} /></App>)
    await waitFor(() => expect(compile).toHaveBeenCalledTimes(1))
    rerender(<App><MjmlCodeEditor {...props} mjmlSource="<mjml>New</mjml>" emailPreview={{ subject: 'New {{name}}', senderName: 'Sender' }} /></App>)
    await waitFor(() => expect(screen.getByTestId('code-email-body')).toHaveTextContent('New compiled body'), { timeout: 2500 })
    await act(async () => finishOld({ html: 'Old compiled body', mjml: '', subject: 'Old Ada' }))
    expect(screen.getByTestId('code-email-body')).toHaveTextContent('New compiled body')
    expect(screen.getByTestId('code-email-header')).toHaveTextContent('New Ada')
    expect(screen.getByTestId('code-email-header')).toHaveTextContent('New summary')
  })

  it('clears the old content after the source is emptied', async () => {
    const props = { onMjmlSourceChange: vi.fn(), onTestDataChange: vi.fn(), onCompile: vi.fn().mockResolvedValue({ html: 'Old body', mjml: '' }) }
    const { rerender } = render(<App><MjmlCodeEditor {...props} mjmlSource="<mjml />" /></App>)
    await waitFor(() => expect(screen.getByTestId('code-email-body')).toHaveTextContent('Old body'))
    rerender(<App><MjmlCodeEditor {...props} mjmlSource="" /></App>)
    await waitFor(() => expect(screen.getByTestId('code-email-body')).not.toHaveTextContent('Old body'), { timeout: 2500 })
  })
})
