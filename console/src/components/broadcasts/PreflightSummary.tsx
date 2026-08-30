import { Alert, Button, Card, Col, Descriptions, Row, Space, Tag, Typography } from 'antd'
import type { MarketingPreflightResult } from '../../services/api/broadcast'

const { Text } = Typography

interface PreflightSummaryProps {
  result: MarketingPreflightResult
  workspaceId: string
  onRefresh?: () => void
  refreshing?: boolean
}

export function PreflightSummary({ result, workspaceId, onRefresh, refreshing }: PreflightSummaryProps) {
  const counts = result.counts
  return (
    <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
      <Alert
        type={result.blocking_count > 0 ? 'error' : result.warning_count > 0 ? 'warning' : 'success'}
        showIcon
        title={result.blocking_count > 0 ? `发现 ${result.blocking_count} 项必须修复的问题` : '发送前检查已完成'}
        description={result.blocking_count > 0 ? '修复后请重新检查。当前活动不能发送或定时。' : '实际频控仍会在每位客户发送前原子执行。'}
        action={onRefresh ? <Button size="small" loading={refreshing} onClick={onRefresh}>重新检查</Button> : undefined}
      />
      <Card size="small" title="预计影响范围">
        <Descriptions size="small" column={{ xs: 2, sm: 4 }} items={[
          { key: 'target', label: '目标客户', children: counts.target_total },
          { key: 'reachable', label: '预计可触达', children: counts.reachable },
          { key: 'identity', label: '缺少身份', children: counts.missing_identity },
          { key: 'consent', label: '缺少同意', children: counts.missing_consent },
          { key: 'suppressed', label: '已抑制', children: counts.suppressed },
          { key: 'frequency', label: '预计频控', children: counts.frequency_deny },
          { key: 'variables', label: '变量失败', children: counts.variable_failures }
        ]} />
      </Card>
      {result.issues.length > 0 && (
        <Row gutter={[12, 12]}>
          {result.issues.map((issue) => (
            <Col span={24} key={issue.code}>
              <Card size="small">
                <Space orientation="vertical" size={2} style={{ width: '100%' }}>
                  <Space><Tag color={issue.severity === 'blocking' ? 'red' : 'gold'}>{issue.severity === 'blocking' ? '必须修复' : '请确认'}</Tag><Text strong>{issue.title}</Text></Space>
                  <Text type="secondary">{issue.description}</Text>
                  {issue.fix_path && <Button type="link" size="small" style={{ paddingInline: 0 }} href={`/console/workspace/${workspaceId}${issue.fix_path}`}>前往修复</Button>}
                </Space>
              </Card>
            </Col>
          ))}
        </Row>
      )}
      <Text type="secondary">检查结果将在 {new Date(result.expires_at).toLocaleTimeString()} 前有效；发送时后端会重新校验。</Text>
    </Space>
  )
}
