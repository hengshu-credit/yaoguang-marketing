import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { PreflightSummary } from './PreflightSummary'
import type { MarketingPreflightResult } from '../../services/api/broadcast'

const result: MarketingPreflightResult = {
  workspace_id: 'workspace-1', broadcast_id: 'broadcast-1', summary_hash: 'hash',
  generated_at: '2026-08-30T08:00:00Z', expires_at: '2026-08-30T08:05:00Z',
  blocking_count: 1, warning_count: 1,
  counts: { target_total: 100, reachable: 80, missing_identity: 5, missing_consent: 10, suppressed: 5, frequency_deny: 0, variable_failures: 0 },
  issues: [
    { code: 'provider_missing', severity: 'blocking', title: '尚未配置营销渠道', description: '请先配置 Provider', fix_path: '/settings/integrations' },
    { code: 'consent_missing', severity: 'warning', title: '部分客户缺少营销同意', description: '10 位客户缺少同意' }
  ]
}

describe('PreflightSummary', () => {
  it('separates blocking issues, warnings, and recipient conservation counts', () => {
    render(<PreflightSummary result={result} workspaceId="workspace-1" />)
    expect(screen.getByText('发现 1 项必须修复的问题')).toBeInTheDocument()
    expect(screen.getByText('目标客户')).toBeInTheDocument()
    expect(screen.getByText('预计可触达')).toBeInTheDocument()
    expect(screen.getByText('尚未配置营销渠道')).toBeInTheDocument()
    expect(screen.getByText('部分客户缺少营销同意')).toBeInTheDocument()
  })
})
