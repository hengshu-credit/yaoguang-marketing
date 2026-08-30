import { Alert, Button, Space, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { describeApiError } from '../../services/api/errors'

export interface ActionableErrorProps {
  error: unknown
  onRetry?: () => void
  retrying?: boolean
  fixHref?: string
  fixLabel?: string
  onFix?: () => void
  fieldLabels?: Record<string, string>
}

function fallbackFieldLabel(field: string): string {
  const words = field.replace(/[._-]+/g, ' ').trim()
  return words ? words.charAt(0).toUpperCase() + words.slice(1) : field
}

export function ActionableError({
  error,
  onRetry,
  retrying,
  fixHref,
  fixLabel,
  onFix,
  fieldLabels
}: ActionableErrorProps) {
  const { t } = useLingui()
  const description = describeApiError(error)
  const commonFieldLabels: Record<string, string> = {
    customer_no: t`Customer number`,
    external_user_id: t`External user ID`,
    name: t`Name`,
    channel: t`Channel`,
    provider: t`Provider`
  }

  const focusField = (field: string) => {
    const target = document.getElementById(`field-${field}`)
      ?? document.querySelector<HTMLElement>(`[name="${field}"]`)
    target?.focus()
    target?.scrollIntoView?.({ block: 'center', behavior: 'smooth' })
  }

  const actions = (
    <Space wrap>
      {description.retryable && onRetry && (
        <Button size="small" onClick={onRetry} loading={retrying}>{t`Try again`}</Button>
      )}
      {fixHref && <Button size="small" type="primary" href={fixHref}>{fixLabel ?? t`Open fix`}</Button>}
      {onFix && <Button size="small" type="primary" onClick={onFix}>{fixLabel ?? t`Open fix`}</Button>}
    </Space>
  )

  return (
    <Alert
      role="alert"
      type="error"
      showIcon
      title={description.title}
      action={(description.retryable && onRetry) || fixHref || onFix ? actions : undefined}
      description={(
        <Space orientation="vertical" size={4} style={{ width: '100%' }}>
          <Typography.Text>{description.impact}</Typography.Text>
          <Typography.Text>{description.nextStep}</Typography.Text>
          {description.fieldErrors.length > 0 && (
            <ul className="actionable-error-fields">
              {description.fieldErrors.map(({ field, message }) => {
                const label = fieldLabels?.[field] ?? commonFieldLabels[field] ?? fallbackFieldLabel(field)
                return (
                  <li key={`${field}-${message}`}>
                    <Button type="link" size="small" onClick={() => focusField(field)} aria-label={`${label}: ${message}`}>
                      {label}: {message}
                    </Button>
                  </li>
                )
              })}
            </ul>
          )}
          {description.requestId && (
            <Typography.Text type="secondary" copyable={{ text: description.requestId }}>
              {t`Request ID`}: {description.requestId}
            </Typography.Text>
          )}
        </Space>
      )}
    />
  )
}
