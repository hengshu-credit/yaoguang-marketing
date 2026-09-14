import { Alert, Button, Collapse, Descriptions, Drawer, Space, Table, Tabs, Tag, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { BusinessDefinition } from '../../services/api/business'
import { businessDefinitionDisplayName } from './builtin-event-labels'
import { builtinEnvelopeFields, builtinEventOriginLabel, getBuiltinEventReference, type EventReferenceField } from './builtin-event-reference'
import { SchemaFieldsEditor } from './SchemaFieldsEditor'

export function BuiltinEventDetails({ definition, onClose }: { definition: BusinessDefinition; onClose: () => void }) {
  const { t, i18n } = useLingui()
  const reference = getBuiltinEventReference(definition.key, i18n)
  const fields = (rows: EventReferenceField[]) => <Table size="small" rowKey="path" dataSource={rows} pagination={false} expandable={{ defaultExpandAllRows: true, indentSize: 18 }} scroll={{ x: 640 }} columns={[
    { title: t`Field`, dataIndex: 'path', width: 265, render: (path: string) => <Typography.Text code>{path.split('.').pop()}</Typography.Text> },
    { title: t`Type`, dataIndex: 'type', width: 120 },
    { title: t`Meaning`, dataIndex: 'description' }
  ]} />
  const example = (value: Record<string, unknown>, label: string) => <div>
    <Typography.Paragraph copyable={{ text: JSON.stringify(value, null, 2) }} strong>{label}</Typography.Paragraph>
    <pre aria-label={label} style={{ margin: 0, padding: 16, maxHeight: 390, overflow: 'auto', border: '1px solid #e5e7eb', borderRadius: 8, background: '#f8fafc', fontSize: 12 }}>{JSON.stringify(value, null, 2)}</pre>
  </div>
  return <Drawer open title={businessDefinitionDisplayName(definition, i18n)} size={980} onClose={onClose} extra={<Button onClick={onClose}>{t`Close`}</Button>}>
    <Space wrap className="mb-4"><Tag color="blue">{t`Built-in event`}</Tag><Typography.Text code copyable>{definition.key}</Typography.Text>{reference && <Tag color="purple">{builtinEventOriginLabel(reference.origin, i18n)}</Tag>}</Space>
    <Alert className="mb-4" type="info" showIcon title={t`Produced through a platform-controlled flow`} description={t`These events are recorded by internal producers after a platform operation, tracking request or provider/client feedback. Do not post them to events.track. Adding an extension field does not automatically collect its value.`} />
    <Tabs defaultActiveKey="reporting" items={[
      { key: 'reporting', label: t`Trigger and payload`, children: reference ? <div className="space-y-5">
        <Descriptions size="small" column={1} bordered items={[
          { key: 'trigger', label: t`When it happens`, children: reference.trigger },
          { key: 'setup', label: t`How it reaches the platform`, children: reference.setup },
          ...(reference.endpoints.length ? [{ key: 'endpoints', label: t`Related entry points`, children: <Space orientation="vertical" className="w-full">{reference.endpoints.map(endpoint => <Typography.Text key={endpoint} code style={{ overflowWrap: 'anywhere' }}>{endpoint}</Typography.Text>)}</Space> }] : [])
        ]} />
        {reference.notes.map(note => <Alert key={note} type="warning" showIcon title={note} />)}
        <Typography.Paragraph type="secondary">{t`The examples show internal event-bus messages, not provider webhook bodies or a public events.track request. Timeline payload data.changes corresponds to the event ledger properties; actual fields depend on the producer path.`}</Typography.Paragraph>
        <Tabs items={reference.variants.map((variant, index) => ({ key: String(index), label: variant.label, children: <div className="space-y-4">{fields(variant.fields)}{example(variant.example, t`Internal event example`)}</div> }))} />
        <Collapse items={[
          { key: 'envelope', label: t`Common event envelope`, children: fields(builtinEnvelopeFields(i18n)) },
          ...(reference.receiptExample ? [{ key: 'receipt', label: t`Normalized receipt API example`, children: <div><Alert className="mb-3" type="info" title={t`This is authorized deliveryReceipts.ingest input, not a provider's original webhook body. Replace the example identifiers with an actual sent message and a unique provider receipt ID.`} />{example(reference.receiptExample, t`Normalized receipt request`)}</div> }] : [])
        ]} />
      </div> : <Alert type="warning" showIcon title={t`Producer documentation is not available for this built-in event yet.`} description={t`Do not infer an upload format from an empty schema. Check the event's producer implementation or existing event records.`} /> },
      { key: 'extensions', label: t`Declared extension fields`, children: <div className="space-y-4">
        <Alert type="info" showIcon title={t`Declared fields are separate from built-in producer fields`} description={t`These are the fields recorded in the event definition, including optional additions. They do not replace the producer payload documented on the first tab. Use the event list to add fields; the producer must supply their values.`} />
        <SchemaFieldsEditor value={definition.schema} disabled onChange={() => {}} />
      </div> }
    ]} />
  </Drawer>
}
