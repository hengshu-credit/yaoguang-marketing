import { Form } from 'antd'
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { ChannelDefinition } from '../../services/api/channels'
import OmnichannelFields from './OmnichannelFields'

const whatsapp: ChannelDefinition = {
  id: 'whatsapp', label_key: 'WhatsApp', content_families: ['text', 'external_template'],
  preview_profiles: [{ id: 'whatsapp_android', label_key: 'WhatsApp Android', surface: 'mobile' }],
  delivery_modes: ['signed_webhook'], limits: { max_body_runes: 4096 }
}

const renderFields = (family: 'text' | 'external_template') => render(
  <Form initialValues={{ family }}><OmnichannelFields definition={whatsapp} family={family} /></Form>
)

describe('OmnichannelFields', () => {
  it('renders only the fields supported by an external platform template', () => {
    renderFields('external_template')
    expect(screen.getByLabelText(/Platform template ID/i)).toBeVisible()
    expect(screen.getByLabelText(/Platform language/i)).toBeVisible()
    expect(screen.queryByLabelText(/^Message$/i)).not.toBeInTheDocument()
  })

  it('applies channel limits to ordinary text content', () => {
    renderFields('text')
    expect(screen.getByLabelText(/^Message$/i)).toHaveAttribute('maxlength', '4096')
    expect(screen.queryByLabelText(/Platform template ID/i)).not.toBeInTheDocument()
  })
})

