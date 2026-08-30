import { useMemo, useState } from 'react'
import { useParams, Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Alert, Button, Card, Descriptions, Form, Input, Progress, Select, Space, Tabs, Typography, Upload, message } from 'antd'
import { InboxOutlined } from '@ant-design/icons'
import { listsApi } from '../services/api/list'
import { audienceApi, importJobApi, type Audience, type ImportJob } from '../services/api/marketing'

const { Title, Paragraph, Text } = Typography

export function AudiencesPage() {
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId/audiences' })
  const [form] = Form.useForm<{ name: string; description: string; list_id: string }>()
  const [previewTotal, setPreviewTotal] = useState<number>()
  const [audience, setAudience] = useState<Audience>()
  const [building, setBuilding] = useState(false)
  const [importJob, setImportJob] = useState<ImportJob>()
  const [uploading, setUploading] = useState(false)
  const lists = useQuery({ queryKey: ['lists', workspaceId], queryFn: () => listsApi.list({ workspace_id: workspaceId }) })
  const listOptions = useMemo(() => (lists.data?.lists ?? []).map((list) => ({ label: list.name, value: list.id })), [lists.data])

  const definition = () => ({ leaf_type: 'list' as const, ref_id: form.getFieldValue('list_id') })
  const preview = async () => {
    try {
      await form.validateFields(['list_id'])
      const result = await audienceApi.preview(workspaceId, definition())
      setPreviewTotal(result.total)
    } catch (error) {
      if (error instanceof Error) message.error(error.message)
    }
  }
  const saveAndBuild = async () => {
    const values = await form.validateFields()
    setBuilding(true)
    try {
      const created = await audienceApi.create(workspaceId, values.name, values.description ?? '', definition())
      const build = await audienceApi.build(workspaceId, created.id, created.active_version)
      setAudience({ ...created, active_build_id: build.build_id })
      setPreviewTotal(build.member_count)
      message.success(`客群已生成，共 ${build.member_count} 位客户`)
    } catch (error) {
      message.error(error instanceof Error ? error.message : '客群生成失败')
    } finally {
      setBuilding(false)
    }
  }
  const upload = async (file: File) => {
    setUploading(true)
    try {
      const job = await importJobApi.upload(workspaceId, file)
      setImportJob(job)
      message.success(job.status === 'rejected' ? '名单已完整接收，但因后台限制被明确拒绝' : '名单已完整接收，等待处理')
    } catch (error) {
      message.error(error instanceof Error ? error.message : '上传失败')
    } finally {
      setUploading(false)
    }
    return false
  }
  const processChunk = async () => {
    if (!importJob) return
    await importJobApi.process(workspaceId, importJob.id)
    setImportJob(await importJobApi.get(workspaceId, importJob.id))
  }

  const total = importJob?.counters.total ?? 0
  const terminal = (importJob?.counters.succeeded ?? 0) + (importJob?.counters.failed ?? 0)
  const percent = total === 0 ? 0 : Math.round((terminal / total) * 100)

  return (
    <div className="p-6" style={{ maxWidth: 1100 }}>
      <Title level={2}>客群</Title>
      <Paragraph type="secondary">用业务名单创建可复现客群，或上传大批量客户数据。系统会记录每一行的处理结果，不会静默丢弃。</Paragraph>
      <Tabs items={[
        {
          key: 'audience', label: '客群定义', children: (
            <Card title="从名单创建客群">
              <Form form={form} layout="vertical" style={{ maxWidth: 640 }}>
                <Form.Item name="name" label="客群名称" rules={[{ required: true, message: '请输入客群名称' }]}><Input placeholder="例如：近30天高意向客户" /></Form.Item>
                <Form.Item name="description" label="业务说明"><Input.TextArea placeholder="说明这个客群用于什么场景" /></Form.Item>
                <Form.Item name="list_id" label="选择名单" rules={[{ required: true, message: '请选择名单' }]}>
                  <Select loading={lists.isLoading} options={listOptions} placeholder="选择一个已维护的名单" showSearch optionFilterProp="label" />
                </Form.Item>
                <Space><Button onClick={preview}>预览人数</Button><Button type="primary" loading={building} onClick={saveAndBuild}>保存并生成客群</Button></Space>
              </Form>
              {previewTotal !== undefined && <Alert className="mt-4" type="info" showIcon message={`当前预计 ${previewTotal} 位客户`} description="活动开始时会冻结收件人快照；之后名单变化不会改变已启动活动。" />}
              {audience && <Descriptions className="mt-4" bordered size="small" items={[{ key: 'id', label: '客群 ID', children: audience.id }, { key: 'version', label: '版本', children: audience.active_version }, { key: 'build', label: '构建状态', children: audience.active_build_id ? '已完成' : '待构建' }]} />}
            </Card>
          )
        },
        {
          key: 'lists', label: '名单管理', children: <Card><Paragraph>名单用于持续维护客户归属；客群用于活动版本和组合运算。</Paragraph><Link to="/console/workspace/$workspaceId/lists" params={{ workspaceId }}><Button>进入名单管理</Button></Link></Card>
        },
        {
          key: 'import', label: '批量导入', children: (
            <Card title="客户名单导入">
              <Alert type="success" showIcon message="默认支持最多 1,000,000 行" description="上限和处理分片可由后台配置。上传完成前不会开始营销处理；格式错误行也会保留为明确失败。" className="mb-4" />
              <Upload.Dragger accept=".csv,text/csv" maxCount={1} beforeUpload={(file) => { void upload(file); return false }} showUploadList={false} disabled={uploading}>
                <p className="ant-upload-drag-icon"><InboxOutlined /></p><p>点击或拖拽 CSV 文件到这里</p><p>建议包含 external_user_id、email、phone 列，至少提供一种客户标识</p>
              </Upload.Dragger>
              {importJob && <Card size="small" className="mt-4">
                <Space direction="vertical" style={{ width: '100%' }}>
                  <Text strong>{importJob.filename}</Text><Progress percent={percent} status={importJob.status === 'rejected' ? 'exception' : 'active'} />
                  <Descriptions size="small" column={5} items={[
                    { key: 'total', label: '总行数', children: total }, { key: 'pending', label: '待处理', children: importJob.counters.pending },
                    { key: 'processing', label: '处理中', children: importJob.counters.processing }, { key: 'success', label: '成功', children: importJob.counters.succeeded },
                    { key: 'failed', label: '明确失败', children: importJob.counters.failed }
                  ]} />
                  {['staged', 'processing'].includes(importJob.status) && <Button type="primary" onClick={processChunk}>处理下一批</Button>}
                </Space>
              </Card>}
            </Card>
          )
        }
      ]} />
    </div>
  )
}

