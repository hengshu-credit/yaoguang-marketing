import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { App } from 'antd'
import SetupWizard from './SetupWizard'
import { setupApi } from '../services/api/setup'
import type { SetupStatus } from '../types/setup'

i18n.loadAndActivate({ locale: 'en', messages: {} })

vi.mock('../services/api/setup', () => ({
  setupApi: { getStatus: vi.fn(), initialize: vi.fn(), testSmtp: vi.fn() }
}))

// Only useNavigate is needed; keep the rest of the router real.
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return { ...actual, useNavigate: () => vi.fn() }
})

function mockStatus(oidcConfigured: boolean) {
  const status: SetupStatus = {
    is_installed: false,
    smtp_configured: false,
    api_endpoint_configured: false,
    root_email_configured: false,
    smtp_bridge_configured: false,
    oidc_configured: oidcConfigured
  }
  vi.mocked(setupApi.getStatus).mockResolvedValue(status)
}

const renderWizard = () =>
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <SetupWizard />
      </App>
    </I18nProvider>
  )

const ssoToggleLabel = 'Enable Single Sign-On (OpenID Connect)'

async function expandAdvanced() {
  // The SSO section lives inside the lazily-mounted "Advanced Settings" collapse.
  const adv = await screen.findByText('Advanced Settings')
  fireEvent.click(adv)
}

describe('SetupWizard — SSO step', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.API_ENDPOINT = 'https://app.example.com'
  })

  it('offers the SSO step when OIDC is not env-configured', async () => {
    mockStatus(false)
    renderWizard()
    await expandAdvanced()
    expect(await screen.findByText(ssoToggleLabel)).toBeInTheDocument()
  })

  it('hides the SSO step entirely when OIDC is configured via env', async () => {
    mockStatus(true)
    renderWizard()
    await expandAdvanced()
    // Give the (absent) section a chance to render, then assert it never does.
    await waitFor(() => expect(screen.queryByText(ssoToggleLabel)).not.toBeInTheDocument())
  })

  it('reveals the OIDC fields only after enabling the SSO switch', async () => {
    mockStatus(false)
    renderWizard()
    await expandAdvanced()

    await screen.findByText(ssoToggleLabel)
    // Fields are hidden until the operator flips the switch.
    expect(screen.queryByLabelText('Issuer URL')).not.toBeInTheDocument()

    const item = screen.getByText(ssoToggleLabel).closest('.ant-form-item')
    const toggle = item?.querySelector('button[role="switch"]') as HTMLElement
    expect(toggle).toBeTruthy()
    fireEvent.click(toggle)

    expect(await screen.findByLabelText('Issuer URL')).toBeInTheDocument()
    expect(screen.getByLabelText('Client ID')).toBeInTheDocument()
    expect(screen.getByLabelText('Client Secret')).toBeInTheDocument()
  })

  it('requires allowed domains once auto-create is switched on', async () => {
    mockStatus(false)
    renderWizard()
    await expandAdvanced()

    await screen.findByText(ssoToggleLabel)
    // Enable SSO to reveal the auto-create toggle.
    const ssoItem = screen.getByText(ssoToggleLabel).closest('.ant-form-item')
    fireEvent.click(ssoItem?.querySelector('button[role="switch"]') as HTMLElement)

    const autoLabel = 'Auto-create accounts on first sign-in'
    await screen.findByText(autoLabel)
    expect(screen.queryByLabelText('Allowed email domains')).not.toBeInTheDocument()

    const autoItem = screen.getByText(autoLabel).closest('.ant-form-item')
    fireEvent.click(autoItem?.querySelector('button[role="switch"]') as HTMLElement)

    expect(await screen.findByLabelText('Allowed email domains')).toBeInTheDocument()
  })
})
