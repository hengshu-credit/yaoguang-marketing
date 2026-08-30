import { useCallback, useEffect, useState } from 'react'
import { Alert, Button, Checkbox, Space, Spin, Typography } from 'antd'
import {
  CheckCircleOutlined,
  ExclamationCircleOutlined,
  ReloadOutlined,
  StopOutlined
} from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import {
  automationApi,
  type JourneyPreflightIssue,
  type JourneyPreflightResult
} from '../../services/api/automation'

const { Paragraph, Text, Title } = Typography

interface JourneyPreflightPanelProps {
  workspaceId: string
  automationId: string
  onActivated?: () => void
  onFixIssue?: (issue: JourneyPreflightIssue) => void
}

export function JourneyPreflightPanel({
  workspaceId,
  automationId,
  onActivated,
  onFixIssue
}: JourneyPreflightPanelProps) {
  const { t } = useLingui()
  const [result, setResult] = useState<JourneyPreflightResult>()
  const [loading, setLoading] = useState(true)
  const [activating, setActivating] = useState(false)
  const [confirmWarnings, setConfirmWarnings] = useState(false)
  const [error, setError] = useState<string>()
  const activationCheckFailed = t`Activation check failed`

  const runPreflight = useCallback(async () => {
    setLoading(true)
    setError(undefined)
    setConfirmWarnings(false)
    try {
      setResult(
        await automationApi.preflight({
          workspace_id: workspaceId,
          automation_id: automationId
        })
      )
    } catch (cause) {
      setResult(undefined)
      setError(cause instanceof Error ? cause.message : activationCheckFailed)
    } finally {
      setLoading(false)
    }
  }, [activationCheckFailed, automationId, workspaceId])

  useEffect(() => {
    void runPreflight()
  }, [runPreflight])

  const activate = async () => {
    if (!result || result.blocking_count > 0 || (result.warning_count > 0 && !confirmWarnings)) {
      return
    }
    setActivating(true)
    setError(undefined)
    try {
      await automationApi.activate({
        workspace_id: workspaceId,
        automation_id: automationId,
        preflight_hash: result.summary_hash,
        confirm_warnings: confirmWarnings
      })
      onActivated?.()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t`Failed to activate journey`)
    } finally {
      setActivating(false)
    }
  }

  const canActivate =
    Boolean(result) &&
    result!.blocking_count === 0 &&
    (result!.warning_count === 0 || confirmWarnings)

  return (
    <section aria-label={t`Activation preflight`} className="journey-preflight-panel">
      <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
        <div className="flex items-start justify-between gap-3">
          <div>
            <Title level={4} style={{ margin: 0 }}>
              {t`Activation preflight`}
            </Title>
            <Paragraph type="secondary" style={{ margin: '4px 0 0' }}>
              {t`Check the trigger, flow, templates, providers, variables, and message frequency policies before activation.`}
            </Paragraph>
          </div>
          <Button icon={<ReloadOutlined />} loading={loading} onClick={() => void runPreflight()}>
            {t`Run again`}
          </Button>
        </div>

        {error && <Alert type="error" showIcon title={t`Activation check failed`} description={error} />}

        {loading ? (
          <div className="flex min-h-40 items-center justify-center">
            <Spin description={t`Checking journey...`} />
          </div>
        ) : result ? (
          <>
            {result.issues.length === 0 ? (
              <Alert
                type="success"
                showIcon
                title={t`Ready to activate`}
                description={t`No blocking issues or warnings were found.`}
              />
            ) : (
              <ul className="divide-y divide-gray-200 rounded-lg border border-gray-200">
                {result.issues.map((issue) => (
                  <li key={`${issue.code}:${issue.node_id ?? issue.fix_path ?? issue.title}`} className="flex items-start gap-3 p-4">
                    {issue.severity === 'blocking' ? (
                      <StopOutlined className="mt-1" style={{ color: '#cf1322' }} />
                    ) : (
                      <ExclamationCircleOutlined className="mt-1" style={{ color: '#d48806' }} />
                    )}
                    <div className="min-w-0 flex-1">
                      <Space wrap>
                        <Text strong>{issue.title}</Text>
                        <Text type={issue.severity === 'blocking' ? 'danger' : 'warning'}>
                          {issue.severity === 'blocking' ? t`Blocking` : t`Warning`}
                        </Text>
                      </Space>
                      <div>
                        <Text type="secondary">{issue.description}</Text>
                      </div>
                      {issue.node_id && <Text code>{t`Node: ${issue.node_id}`}</Text>}
                    </div>
                    {(issue.node_id || issue.fix_path) && (
                      <Button
                        type="link"
                        aria-label={t`Fix ${issue.title}`}
                        onClick={() => onFixIssue?.(issue)}
                      >
                        {t`Fix`}
                      </Button>
                    )}
                  </li>
                ))}
              </ul>
            )}

            {result.warning_count > 0 && result.blocking_count === 0 && (
              <Checkbox
                checked={confirmWarnings}
                onChange={(event) => setConfirmWarnings(event.target.checked)}
              >
                {t`I understand these warnings and want to activate this journey.`}
              </Checkbox>
            )}

            <div className="flex items-center justify-between gap-3 border-t border-gray-200 pt-4">
              <Text type="secondary">
                {result.blocking_count > 0
                  ? t`${result.blocking_count} blocking issue(s) must be fixed.`
                  : result.warning_count > 0
                    ? t`${result.warning_count} warning(s) require confirmation.`
                    : t`This check is valid for five minutes unless the journey changes.`}
              </Text>
              <Button
                type="primary"
                aria-label={t`Activate journey`}
                icon={<CheckCircleOutlined />}
                loading={activating}
                disabled={!canActivate}
                onClick={() => void activate()}
              >
                {t`Activate journey`}
              </Button>
            </div>
          </>
        ) : null}
      </Space>
    </section>
  )
}
