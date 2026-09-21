import { Alert, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { BusinessDefinitionKind } from '../../services/api/business'

export function BusinessConfigurationGuide({ kind }: { kind: BusinessDefinitionKind }) {
  const { t } = useLingui()
  if (kind === 'event' || kind === 'event_mapping') return <Alert type="info" showIcon title={kind === 'event' ? t`An event code describes an action; a report ID identifies one occurrence` : t`Field mappings adapt an external payload, without creating a second event protocol`} description={kind === 'event' ? t`For your own events, configure reporting access and data fields, validate an example, then activate and copy the integration instructions. Built-in events are produced by the platform and remain read-only.` : t`Use a mapping only when your application cannot send the standard event object. Define and activate the target event first, then map the external JSON and validate a sample.`} />
  if (kind !== 'object_type' && kind !== 'object_relation') return null
  return <Alert type="info" showIcon title={kind === 'object_type' ? t`Object types describe things a customer can have more than once` : t`Object relations connect two business records`} description={<div className="space-y-2">
    <Typography.Paragraph className="mb-0">{kind === 'object_type' ? t`Use customer attributes for information such as membership level. Use objects for orders, contracts or repayment plans: one customer can have several records, each with its own ID, fields, events and states.` : t`For example, one contract can have many repayment plans. Define contract and repayment_plan first, then create a one-to-many relation from contract to repayment_plan. Each repayment plan can then belong to one contract.`}</Typography.Paragraph>
    <Typography.Paragraph className="mb-0">{kind === 'object_type' ? t`Configure the record fields first (for example order_id and amount), select its identity fields, then activate it. Report events with this object type and its external ID to follow that individual record.` : t`Creating a relation only defines the allowed connection. Link actual object records through the relation API; the records must belong to the same customer and source system. Customer ownership is automatic and does not need a separate relation.`}</Typography.Paragraph>
    <Typography.Text strong>{t`Customer → multiple contracts → multiple repayment plans per contract`}</Typography.Text>
  </div>} />
}
