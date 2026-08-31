import { Button, Col, Form, Input, Row, Space } from 'antd'
import type { NamePath } from 'antd/es/form/interface'
import { useLingui } from '@lingui/react/macro'
import type { ChannelDefinition, ContentFamily } from '../../services/api/channels'

interface OmnichannelFieldsProps {
  definition: ChannelDefinition
  family: ContentFamily
  prefix?: NamePath
  required?: boolean
}

const OmnichannelFields: React.FC<OmnichannelFieldsProps> = ({ definition, family, prefix = 'content', required = true }) => {
  const { t } = useLingui()
  const path = (field: string | number, ...rest: Array<string | number>): NamePath => [
    ...(Array.isArray(prefix) ? prefix : [prefix]), field, ...rest
  ]
  const bodyLimit = definition.limits.max_body_runes || 10000
  const titleLimit = definition.limits.max_title_runes || 512

  if (family === 'external_template') {
    return <>
      <Row gutter={12}>
        <Col span={12}><Form.Item label={t`Platform template ID`} name={path('external_template', 'id')} rules={required ? [{ required: true }] : undefined}><Input /></Form.Item></Col>
        <Col span={12}><Form.Item label={t`Platform language`} name={path('external_template', 'language')} rules={required ? [{ required: true }] : undefined}><Input placeholder="es_MX" /></Form.Item></Col>
      </Row>
      <Form.Item label={t`Platform category`} name={path('external_template', 'category')}><Input /></Form.Item>
      <Form.List name={path('external_template', 'parameters')}>
        {(fields, { add, remove }) => <Space orientation="vertical" className="w-full">
          {fields.map((field) => <Space key={field.key} align="baseline" className="w-full">
            <Form.Item {...field} name={[field.name, 'name']} rules={[{ required: true }]}><Input placeholder={t`Parameter name`} /></Form.Item>
            <Form.Item {...field} name={[field.name, 'value']} rules={[{ required: true }]}><Input placeholder="{{ data.value }}" /></Form.Item>
            <Button type="link" danger onClick={() => remove(field.name)}>{t`Remove`}</Button>
          </Space>)}
          <Button type="dashed" onClick={() => add()}>{t`Add parameter`}</Button>
        </Space>}
      </Form.List>
    </>
  }

  if (family === 'webhook_payload') {
    return <>
      <Form.Item label={t`Content type`} name={path('webhook', 'content_type')} initialValue="application/json"><Input disabled /></Form.Item>
      <Form.Item label={t`JSON request body`} name={path('webhook', 'body')} rules={required ? [{ required: true }] : undefined}>
        <Input.TextArea rows={10} className="font-mono" maxLength={definition.limits.max_payload_bytes || 131072} />
      </Form.Item>
    </>
  }

  if (family === 'carousel') {
    return <Form.List name={path('cards')}>
      {(fields, { add, remove }) => <Space orientation="vertical" className="w-full">
        {fields.map((field, index) => <div key={field.key} className="rounded-md border border-gray-200 p-3">
          <Form.Item label={`${t`Card`} ${index + 1}`} name={[field.name, 'title']}><Input maxLength={titleLimit} /></Form.Item>
          <Form.Item label={t`Message`} name={[field.name, 'body']} rules={required ? [{ required: true }] : undefined}><Input.TextArea rows={3} maxLength={bodyLimit} /></Form.Item>
          <Button type="link" danger onClick={() => remove(field.name)}>{t`Remove card`}</Button>
        </div>)}
        <Button type="dashed" disabled={fields.length >= (definition.limits.max_cards || 10)} onClick={() => add()}>{t`Add card`}</Button>
      </Space>}
    </Form.List>
  }

  return <>
    {family !== 'text' && <Form.Item label={t`Title`} name={path('title')} rules={family === 'notification' ? [{ required: true }] : undefined}>
      <Input maxLength={titleLimit} />
    </Form.Item>}
    <Form.Item label={t`Message`} name={path('body')} rules={required ? [{ required: true }] : undefined}>
      <Input.TextArea rows={6} maxLength={bodyLimit} />
    </Form.Item>
    {family !== 'text' && <>
      <Form.Item label={t`Media URL`} name={path('media', 'url')}><Input placeholder="https://..." /></Form.Item>
      <Form.Item name={path('media', 'type')} initialValue="image" hidden><Input /></Form.Item>
      <Form.Item label={t`Footer`} name={path('footer')}><Input maxLength={1024} /></Form.Item>
    </>}
    <Form.Item label={t`Custom data (JSON)`} name={path('data_json')}><Input.TextArea rows={3} className="font-mono" /></Form.Item>
  </>
}

export default OmnichannelFields
