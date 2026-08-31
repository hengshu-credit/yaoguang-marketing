import { useState } from 'react'
import { AutoComplete, Button, Input, Table, Modal, Form, Popconfirm } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import type { DataFeedHeader } from '../../services/api/broadcast'

interface HeadersEditorProps {
  value?: DataFeedHeader[]
  onChange?: (headers: DataFeedHeader[]) => void
  disabled?: boolean
  valuePlaceholder?: string
}

const COMMON_HEADER_NAMES = [
  'Authorization',
  'X-API-Key',
  'X-Auth-Token',
  'X-Client-ID',
  'X-Client-Secret',
  'Accept',
  'Content-Type',
  'Accept-Language',
  'User-Agent'
]

export function HeadersEditor({
  value = [],
  onChange,
  disabled = false,
  valuePlaceholder = 'Bearer {{ contact.custom_string_1 }}'
}: HeadersEditorProps) {
  const { t } = useLingui()
  const [modalOpen, setModalOpen] = useState(false)
  const [form] = Form.useForm()

  const handleAddHeader = () => {
    form.validateFields().then((values) => {
      const newHeaders = [...value, { name: values.name, value: values.value }]
      onChange?.(newHeaders)
      form.resetFields()
      setModalOpen(false)
    })
  }

  const handleRemoveHeader = (index: number) => {
    const newHeaders = value.filter((_, i) => i !== index)
    onChange?.(newHeaders)
  }

  const columns = [
    {
      title: t`Headers`,
      dataIndex: 'name',
      key: 'name',
      width: 180
    },
    {
      title: t`Content`,
      dataIndex: 'value',
      key: 'value'
    },
    {
      title: (
        <Button
          type="primary"
          ghost
          size="small"
          icon={<PlusOutlined />}
          onClick={() => setModalOpen(true)}
          disabled={disabled}
        >
          {t`Add`}
        </Button>
      ),
      key: 'action',
      width: 80,
      align: 'right' as const,
      render: (_: unknown, __: DataFeedHeader, index: number) => (
        <Popconfirm
          title={t`Delete Header`}
          description={t`Are you sure you want to delete this header?`}
          onConfirm={() => handleRemoveHeader(index)}
          okText={t`Yes`}
          cancelText={t`No`}
        >
          <Button
            type="text"
            icon={<DeleteOutlined />}
            disabled={disabled}
          />
        </Popconfirm>
      )
    }
  ]

  return (
    <div className="space-y-2">
      {value.length > 0 ? (
        <Table
          dataSource={value.map((h, i) => ({ ...h, key: i }))}
          columns={columns}
          showHeader={true}
          pagination={false}
          size="small"
        />
      ) : (
        <Button type="primary" ghost block size="small" onClick={() => setModalOpen(true)} disabled={disabled}>
          {t`Add Header`}
        </Button>
      )}

      <Modal
        title={t`Add Header`}
        open={modalOpen}
        onCancel={() => {
          form.resetFields()
          setModalOpen(false)
        }}
        onOk={handleAddHeader}
        okText={t`Add`}
        cancelText={t`Cancel`}
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="name"
            label={t`Header`}
            rules={[{ required: true, message: t`Header is required` }]}
          >
            <AutoComplete
              options={COMMON_HEADER_NAMES.map((header) => ({ value: header }))}
              placeholder="Authorization"
              filterOption={(inputValue, option) =>
                String(option?.value ?? '').toLowerCase().includes(inputValue.toLowerCase())
              }
            />
          </Form.Item>
          <Form.Item
            name="value"
            label={t`Content`}
            rules={[{ required: true, message: t`Content is required` }]}
            extra={t`Header content supports template placeholders`}
          >
            <Input placeholder={valuePlaceholder} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
