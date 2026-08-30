import { useEffect, useMemo, useState } from 'react'
import { useParams, Link } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Card, Descriptions, Form, Input, Popconfirm, Progress, Select, Space, Steps, Table, Tabs, Tag, Typography, Upload, message } from 'antd'
import { DeleteOutlined, DownloadOutlined, InboxOutlined, PlusOutlined, StopOutlined } from '@ant-design/icons'
import { listsApi } from '../services/api/list'
import { listSegments } from '../services/api/segment'
import { audienceApi, importJobApi, type Audience, type AudienceExpression, type AudienceLeafType, type AudienceOperator, type ImportJob } from '../services/api/marketing'

const { Title, Paragraph, Text } = Typography

export function AudiencesPage() {
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId/audiences' })
  const [form] = Form.useForm<{ name: string; description: string }>()
  const [previewTotal, setPreviewTotal] = useState<number>()
  const [audience, setAudience] = useState<Audience>()
  const [building, setBuilding] = useState(false)
  const [importJob, setImportJob] = useState<ImportJob>()
  const [uploading, setUploading] = useState(false)
  const [operator, setOperator] = useState<AudienceOperator>('union')
  const [rules, setRules] = useState<Array<{ key: string; leaf_type: AudienceLeafType; ref_id: string }>>([
    { key: 'rule-1', leaf_type: 'list', ref_id: '' }
  ])
  const queryClient = useQueryClient()
  const lists = useQuery({ queryKey: ['lists', workspaceId], queryFn: () => listsApi.list({ workspace_id: workspaceId }) })
  const segments = useQuery({ queryKey: ['segments', workspaceId], queryFn: () => listSegments({ workspace_id: workspaceId, with_count: true }) })
  const audiences = useQuery({ queryKey: ['audiences', workspaceId], queryFn: () => audienceApi.list(workspaceId) })
  const listOptions = useMemo(() => (lists.data?.lists ?? []).map((list) => ({ label: list.name, value: list.id })), [lists.data])
  const segmentOptions = useMemo(() => (segments.data?.segments ?? []).map((segment) => ({ label: segment.name, value: segment.id })), [segments.data])
  const audienceOptions = useMemo(() => (audiences.data?.items ?? []).map((item) => ({ label: item.name, value: item.id })), [audiences.data])
  const importHistory = useQuery({
    queryKey: ['customer-imports', workspaceId],
    queryFn: () => importJobApi.list(workspaceId),
    refetchInterval: (query) => query.state.data?.items.some((item) => ['staged', 'processing', 'uploading'].includes(item.status)) ? 3000 : false
  })
  const activeImport = useQuery({
    queryKey: ['customer-import', workspaceId, importJob?.id],
    queryFn: () => importJobApi.get(workspaceId, importJob!.id),
    enabled: Boolean(importJob?.id),
    initialData: importJob,
    refetchInterval: (query) => query.state.data && ['staged', 'processing', 'uploading'].includes(query.state.data.status) ? 2000 : false
  })
  const activeBuild = useQuery({
    queryKey: ['audience-build', workspaceId, audience?.active_build_id],
    queryFn: () => audienceApi.buildStatus(workspaceId, audience!.active_build_id!),
    enabled: Boolean(audience?.active_build_id),
    refetchInterval: (query) => query.state.data && ['pending', 'building'].includes(query.state.data.status) ? 1500 : false
  })

  useEffect(() => {
    if (activeBuild.data?.status === 'completed') {
      setPreviewTotal(activeBuild.data.member_count)
      setBuilding(false)
      void queryClient.invalidateQueries({ queryKey: ['audiences', workspaceId] })
    } else if (activeBuild.data && ['failed', 'cancelled'].includes(activeBuild.data.status)) {
      setBuilding(false)
    }
  }, [activeBuild.data, queryClient, workspaceId])

  const definition = (): AudienceExpression => rules.length === 1
    ? { leaf_type: rules[0].leaf_type, ref_id: rules[0].ref_id }
    : { operator, children: rules.map((rule) => ({ leaf_type: rule.leaf_type, ref_id: rule.ref_id })) }
  const validateRules = () => {
    if (rules.some((rule) => !rule.ref_id)) {
      message.error('请为每一条客群条件选择具体来源')
      return false
    }
    if (operator === 'exclusion' && rules.length !== 2) {
      message.error('排除客群必须正好包含“保留”和“排除”两条条件')
      return false
    }
    return true
  }
  const preview = async () => {
    try {
      if (!validateRules()) return
      const result = await audienceApi.preview(workspaceId, definition())
      setPreviewTotal(result.total)
    } catch (error) {
      if (error instanceof Error) message.error(error.message)
    }
  }
  const saveAndBuild = async () => {
    const values = await form.validateFields()
    if (!validateRules()) return
    setBuilding(true)
    let backgroundStarted = false
    try {
      const kind: Audience['kind'] = rules.length > 1 || rules[0].leaf_type === 'audience' ? 'composite' : rules[0].leaf_type === 'segment' ? 'dynamic' : 'static'
      const created = await audienceApi.create(workspaceId, values.name, values.description ?? '', definition(), kind)
      const build = await audienceApi.build(workspaceId, created.id, created.active_version)
      setAudience({ ...created, active_build_id: build.build_id })
      if (build.member_count > 0) {
        setPreviewTotal(build.member_count)
        setBuilding(false)
        message.success(`客群已生成，共 ${build.member_count} 位客户`)
      } else {
        backgroundStarted = true
        message.success('客群已保存并进入后台构建；现在可以安全关闭页面')
      }
      await queryClient.invalidateQueries({ queryKey: ['audiences', workspaceId] })
    } catch (error) {
      message.error(error instanceof Error ? error.message : '客群生成失败')
    } finally {
      if (!backgroundStarted) setBuilding(false)
    }
  }
  const updateRule = (key: string, patch: Partial<{ leaf_type: AudienceLeafType; ref_id: string }>) => {
    setRules((current) => current.map((rule) => rule.key === key ? { ...rule, ...patch } : rule))
  }
  const referenceOptions = (leafType: AudienceLeafType) => leafType === 'list' ? listOptions : leafType === 'segment' ? segmentOptions : audienceOptions
  const removeAudience = async (item: Audience) => {
    try {
      await audienceApi.delete(workspaceId, item.id)
      await queryClient.invalidateQueries({ queryKey: ['audiences', workspaceId] })
      message.success('客群已归档')
    } catch (error) {
      message.error(error instanceof Error ? error.message : '客群仍被其他客群或活动使用，无法删除')
    }
  }
  const upload = async (file: File) => {
    setUploading(true)
    try {
      const job = await importJobApi.upload(workspaceId, file)
      setImportJob(job)
      await queryClient.invalidateQueries({ queryKey: ['customer-imports', workspaceId] })
      message.success(job.status === 'rejected' ? '名单已完整接收，但因后台限制被明确拒绝' : '名单已完整接收，后台将自动处理；现在可以安全关闭页面')
    } catch (error) {
      message.error(error instanceof Error ? error.message : '上传失败')
    } finally {
      setUploading(false)
    }
    return false
  }
  const currentImport = activeImport.data ?? importJob
  const total = currentImport?.counters.total ?? 0
  const terminal = (currentImport?.counters.succeeded ?? 0) + (currentImport?.counters.failed ?? 0)
  const percent = total === 0 ? 0 : Math.round((terminal / total) * 100)
  const activeStatus = currentImport?.status
  const importStep = activeStatus === 'completed' || activeStatus === 'cancelled' ? 4 : activeStatus === 'rejected' ? 2 : activeStatus === 'staged' || activeStatus === 'processing' ? 3 : 0
  const cancelImport = async (job: ImportJob) => {
    await importJobApi.cancel(workspaceId, job.id)
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['customer-imports', workspaceId] }),
      queryClient.invalidateQueries({ queryKey: ['customer-import', workspaceId, job.id] })
    ])
    message.success('导入已取消，未处理行均已记录为明确失败')
  }

  return (
    <div className="p-6" style={{ maxWidth: 1100 }}>
      <Title level={2}>客群</Title>
      <Paragraph type="secondary">用业务名单创建可复现客群，或上传大批量客户数据。系统会记录每一行的处理结果，不会静默丢弃。</Paragraph>
      <Tabs items={[
        {
          key: 'audience', label: '客群定义', children: (
            <Card title="创建可复用客群">
              <Form form={form} layout="vertical" style={{ maxWidth: 640 }}>
                <Form.Item name="name" label="客群名称" rules={[{ required: true, message: '请输入客群名称' }]}><Input placeholder="例如：近30天高意向客户" /></Form.Item>
                <Form.Item name="description" label="业务说明"><Input.TextArea placeholder="说明这个客群用于什么场景" /></Form.Item>
                {rules.length > 1 && <Form.Item label="组合方式">
                  <Select value={operator} onChange={setOperator} options={[
                    { value: 'union', label: '满足任一条件（并集）' },
                    { value: 'intersection', label: '同时满足全部条件（交集）' },
                    { value: 'exclusion', label: '从第一组中排除第二组' }
                  ]} />
                </Form.Item>}
                <Form.Item label="客群条件" required>
                  <Space orientation="vertical" style={{ width: '100%' }}>
                    {rules.map((rule, index) => <Card key={rule.key} size="small">
                      <Space wrap style={{ width: '100%' }}>
                        <Text type="secondary">{operator === 'exclusion' && rules.length > 1 ? (index === 0 ? '保留' : '排除') : `条件 ${index + 1}`}</Text>
                        <Select value={rule.leaf_type} style={{ width: 140 }} onChange={(leaf_type) => updateRule(rule.key, { leaf_type, ref_id: '' })} options={[
                          { value: 'list', label: '业务名单' }, { value: 'segment', label: '动态分群' }, { value: 'audience', label: '已有客群' }
                        ]} />
                        <Select value={rule.ref_id || undefined} style={{ minWidth: 260 }} loading={lists.isLoading || segments.isLoading || audiences.isLoading} showSearch optionFilterProp="label" placeholder="选择具体对象" options={referenceOptions(rule.leaf_type)} onChange={(ref_id) => updateRule(rule.key, { ref_id })} />
                        {rules.length > 1 && <Button aria-label="删除条件" icon={<DeleteOutlined />} onClick={() => setRules((current) => current.filter((item) => item.key !== rule.key))} />}
                      </Space>
                    </Card>)}
                    <Button icon={<PlusOutlined />} disabled={operator === 'exclusion' && rules.length >= 2} onClick={() => setRules((current) => [...current, { key: `rule-${Date.now()}`, leaf_type: 'list', ref_id: '' }])}>添加条件</Button>
                  </Space>
                </Form.Item>
                <Space><Button onClick={preview}>预览人数</Button><Button type="primary" loading={building} onClick={saveAndBuild}>保存并生成客群</Button></Space>
              </Form>
              {previewTotal !== undefined && <Alert className="mt-4" type="info" showIcon message={`当前预计 ${previewTotal} 位客户`} description="活动开始时会冻结收件人快照；之后名单变化不会改变已启动活动。" />}
              {audience && <Descriptions className="mt-4" bordered size="small" items={[{ key: 'id', label: '客群 ID', children: audience.id }, { key: 'version', label: '版本', children: audience.active_version }, { key: 'build', label: '构建状态', children: activeBuild.data?.status === 'completed' ? `已完成（${activeBuild.data.member_count} 位客户）` : activeBuild.data?.status === 'failed' ? `失败：${activeBuild.data.error_detail ?? '请稍后重试'}` : audience.active_build_id ? '后台生成中' : '待构建' }]} />}
              <Table className="mt-6" rowKey="id" loading={audiences.isLoading} pagination={false} dataSource={audiences.data?.items ?? []} columns={[
                { title: '客群名称', dataIndex: 'name' },
                { title: '类型', dataIndex: 'kind', render: (kind: Audience['kind']) => ({ static: '名单客群', dynamic: '动态客群', composite: '组合客群' }[kind]) },
                { title: '版本', dataIndex: 'active_version' },
                { title: '最近构建', render: (_: unknown, item: Audience) => item.active_build_id ? '已完成' : '待生成' },
                { title: '操作', render: (_: unknown, item: Audience) => <Popconfirm title="仅未被其他客群或活动使用时才能删除。" onConfirm={() => removeAudience(item)}><Button danger size="small">删除</Button></Popconfirm> }
              ]} />
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
              <Steps className="mb-6" current={importStep} items={[
                { title: '上传文件', content: '完整接收原文件' },
                { title: '字段识别', content: '识别客户编号和联系方式' },
                { title: '完整校验', content: '错误行也会留档' },
                { title: '后台处理', content: '页面关闭后继续' },
                { title: '完成', content: '成功与失败可追溯' }
              ]} />
              <Upload.Dragger accept=".csv,text/csv" maxCount={1} beforeUpload={(file) => { void upload(file); return false }} showUploadList={false} disabled={uploading}>
                <p className="ant-upload-drag-icon"><InboxOutlined /></p><p>点击或拖拽 CSV 文件到这里</p><p>建议包含 external_user_id、email、phone 列，至少提供一种客户标识</p>
              </Upload.Dragger>
              {currentImport && <Card size="small" className="mt-4">
                <Space orientation="vertical" style={{ width: '100%' }}>
                  <Space><Text strong>{currentImport.filename}</Text><Tag>{currentImport.status}</Tag></Space>
                  <Progress percent={percent} status={['rejected', 'cancelled'].includes(currentImport.status) ? 'exception' : currentImport.status === 'completed' ? 'success' : 'active'} />
                  <Descriptions size="small" column={5} items={[
                    { key: 'total', label: '总行数', children: total }, { key: 'pending', label: '待处理', children: currentImport.counters.pending },
                    { key: 'processing', label: '处理中', children: currentImport.counters.processing }, { key: 'success', label: '成功', children: currentImport.counters.succeeded },
                    { key: 'failed', label: '明确失败', children: currentImport.counters.failed }
                  ]} />
                  <Space>
                    {currentImport.counters.failed > 0 && <Button icon={<DownloadOutlined />} onClick={() => void importJobApi.downloadErrors(workspaceId, currentImport.id)}>下载失败明细</Button>}
                    {['uploading', 'staged', 'processing'].includes(currentImport.status) && <Popconfirm title="取消后，未处理行会记录为明确失败，已成功行不会回滚。" onConfirm={() => cancelImport(currentImport)}><Button danger icon={<StopOutlined />}>取消导入</Button></Popconfirm>}
                  </Space>
                </Space>
              </Card>}
              <Card size="small" title="最近导入任务" className="mt-4">
                <Table rowKey="id" loading={importHistory.isLoading} pagination={false} dataSource={importHistory.data?.items ?? []} columns={[
                  { title: '文件', dataIndex: 'filename' },
                  { title: '状态', dataIndex: 'status', render: (status: ImportJob['status']) => <Tag>{status}</Tag> },
                  { title: '总行数', render: (_: unknown, job: ImportJob) => job.counters.total },
                  { title: '成功', render: (_: unknown, job: ImportJob) => job.counters.succeeded },
                  { title: '明确失败', render: (_: unknown, job: ImportJob) => job.counters.failed },
                  { title: '操作', render: (_: unknown, job: ImportJob) => <Space><Button size="small" onClick={() => setImportJob(job)}>查看</Button>{job.counters.failed > 0 && <Button size="small" onClick={() => void importJobApi.downloadErrors(workspaceId, job.id)}>失败明细</Button>}</Space> }
                ]} />
              </Card>
            </Card>
          )
        }
      ]} />
    </div>
  )
}
