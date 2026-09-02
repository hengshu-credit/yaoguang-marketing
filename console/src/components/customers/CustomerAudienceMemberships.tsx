import { useEffect, useState } from 'react'
import { Alert, Empty, Space, Spin, Tag, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { audienceApi, type AudienceCustomerMatch } from '../../services/api/marketing'

interface CustomerAudienceMembershipsProps {
  workspaceId: string
  customerId: string
  active: boolean
}

interface CalculationState {
  status: 'idle' | 'calculating' | 'complete'
  total: number
  completed: number
  failed: number
  matches: AudienceCustomerMatch[]
}

const idleState: CalculationState = {
  status: 'idle', total: 0, completed: 0, failed: 0, matches: []
}

export function CustomerAudienceMemberships({ workspaceId, customerId, active }: CustomerAudienceMembershipsProps) {
  const { t } = useLingui()
  const [calculation, setCalculation] = useState<CalculationState>(idleState)

  useEffect(() => {
    if (!active || !customerId) {
      setCalculation(idleState)
      return
    }

    let cancelled = false
    setCalculation({ status: 'calculating', total: 0, completed: 0, failed: 0, matches: [] })

    const calculate = async () => {
      try {
        const audiences = (await audienceApi.listAll(workspaceId)).filter((audience) => audience.kind === 'dynamic')
        if (cancelled) return
        if (audiences.length === 0) {
          setCalculation({ status: 'complete', total: 0, completed: 0, failed: 0, matches: [] })
          return
        }
        setCalculation({ status: 'calculating', total: audiences.length, completed: 0, failed: 0, matches: [] })

        await Promise.all(audiences.map(async (audience) => {
          try {
            const result = await audienceApi.matchCustomer(workspaceId, audience.id, customerId)
            if (cancelled) return
            setCalculation((current) => {
              const completed = current.completed + 1
              return {
                ...current,
                status: completed === current.total ? 'complete' : 'calculating',
                completed,
                matches: result.matches ? [...current.matches, result] : current.matches
              }
            })
          } catch {
            if (cancelled) return
            setCalculation((current) => {
              const completed = current.completed + 1
              return {
                ...current,
                status: completed === current.total ? 'complete' : 'calculating',
                completed,
                failed: current.failed + 1
              }
            })
          }
        }))
      } catch {
        if (!cancelled) {
          setCalculation({ status: 'complete', total: 0, completed: 0, failed: 1, matches: [] })
        }
      }
    }

    void calculate()
    return () => { cancelled = true }
  }, [active, customerId, workspaceId])

  const matchCount = calculation.matches.length
  const completedCount = calculation.completed
  const totalCount = calculation.total

  return (
    <section className="rounded-lg border border-gray-200 p-4" aria-labelledby="customer-audiences-title">
      <Typography.Title id="customer-audiences-title" level={5} className="mt-0">
        {t`Dynamic audience memberships`}
      </Typography.Title>

      {calculation.status === 'calculating' ? (
        <Space role="status" className="mb-3">
          <Spin size="small" />
          <Typography.Text type="secondary">
            {t`Calculating dynamic audience memberships (${completedCount}/${totalCount})`}
          </Typography.Text>
        </Space>
      ) : null}

      {matchCount > 0 ? (
        <div><Space wrap>{calculation.matches.map((membership) => (
          <Tag key={membership.audience_id} color="purple">
            {membership.name} · v{membership.audience_version}
          </Tag>
        ))}</Space></div>
      ) : null}

      {calculation.status === 'complete' && matchCount > 0 ? (
        <Typography.Text type="secondary" className="mt-3 block">
          {matchCount === 1
            ? t`Calculation complete: 1 matching dynamic audience`
            : t`Calculation complete: ${matchCount} matching dynamic audiences`}
        </Typography.Text>
      ) : null}

      {calculation.status === 'complete' && matchCount === 0 && calculation.failed === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t`No active audience memberships`} />
      ) : null}

      {calculation.failed > 0 ? (
        <Alert className="mt-3" type="warning" showIcon title={t`Some dynamic audiences could not be evaluated.`} />
      ) : null}
    </section>
  )
}
