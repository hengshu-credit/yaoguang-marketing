import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { I18nProvider } from '@lingui/react'
import { i18n } from '../../i18n'
import { ApiError } from '../../services/api/errors'
import { ActionableError } from './ActionableError'

function renderError(error: unknown, props: Partial<React.ComponentProps<typeof ActionableError>> = {}) {
  return render(
    <I18nProvider i18n={i18n}>
      <ActionableError error={error} {...props} />
    </I18nProvider>
  )
}

describe('ActionableError', () => {
  it('shows impact, next step, a retry action and a copyable request ID', () => {
    const onRetry = vi.fn()
    renderError(new ApiError('unavailable', 503, {
      request_id: 'req-503',
      error: { code: 'service_unavailable', message: 'provider is unavailable' }
    }), { onRetry })

    expect(screen.getByRole('alert')).toHaveTextContent('Service temporarily unavailable')
    expect(screen.getByText('This operation is delayed; no data was discarded.')).toBeInTheDocument()
    expect(screen.getByText(/Request ID: req-503/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(onRetry).toHaveBeenCalledOnce()
  })

  it('links field errors back to the corresponding form field', () => {
    render(
      <I18nProvider i18n={i18n}>
        <input id="field-customer_no" aria-label="Customer number" />
        <ActionableError
          error={new ApiError('invalid', 400, {
            error: {
              code: 'validation_error',
              message: 'Please correct the highlighted field',
              field_errors: { customer_no: 'Use a valid Yaoguang customer number' }
            }
          })}
        />
      </I18nProvider>
    )

    fireEvent.click(screen.getByRole('button', { name: /Customer number: Use a valid/ }))
    expect(screen.getByRole('textbox', { name: 'Customer number' })).toHaveFocus()
  })

  it('does not offer a blind retry for an uncertain delivery', () => {
    renderError(new ApiError('unknown delivery', 409, {
      trace_id: 'trace-unknown',
      error: { code: 'unknown_delivery', message: 'provider result is unknown' }
    }), { onRetry: vi.fn(), fixHref: '/deliveries', fixLabel: 'Open delivery details' })

    expect(screen.getByText('Delivery outcome is uncertain')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Try again' })).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Open delivery details' })).toHaveAttribute('href', '/deliveries')
  })
})
