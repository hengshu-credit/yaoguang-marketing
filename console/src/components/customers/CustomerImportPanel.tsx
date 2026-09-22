import { useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Card, Descriptions, Popconfirm, Progress, Select, Space, Steps, Table, Tag, Typography, Upload, App } from 'antd'
import { DownloadOutlined, InboxOutlined, StopOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { listsApi } from '../../services/api/list'
import { importJobApi, type ImportJob } from '../../services/api/marketing'
import { ActionableError } from '../errors/ActionableError'
import { ImportTemplateButtons } from '../contacts/ImportTemplateButtons'
import { CONTACT_IMPORT_ACCEPT, isContactImportFile } from '../../utils/contactImportFile'

interface CustomerImportPanelProps {
  workspaceId: string
  canWrite: boolean
  canBindLists?: boolean
}

export function CustomerImportPanel({ workspaceId, canWrite, canBindLists = true }: CustomerImportPanelProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [selectedListIDs, setSelectedListIDs] = useState<string[]>([])
  const [importJob, setImportJob] = useState<ImportJob>()
  const [uploading, setUploading] = useState(false)
  const [retryingJobId, setRetryingJobId] = useState<string>()
  const lists = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId }),
    enabled: canBindLists
  })
  const listNameByID = useMemo(
    () => new Map((lists.data?.lists ?? []).map((list) => [list.id, list.name])),
    [lists.data?.lists]
  )
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
  const upload = async (file: File) => {
    if (!canWrite || uploading) return false
    if (!isContactImportFile(file)) {
      message.error(t`Please upload a CSV, XLSX or XLS file.`)
      return false
    }
    setUploading(true)
    try {
      const job = await importJobApi.upload(workspaceId, file, canBindLists ? selectedListIDs : [])
      setImportJob(job)
      await queryClient.invalidateQueries({ queryKey: ['customer-imports', workspaceId] })
      if (job.status === 'rejected') message.warning(t`The import was rejected. Check the file format, headers and configured import limits.`)
      else message.success(t`The file was received and will continue processing in the background.`)
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Upload failed`)
    } finally {
      setUploading(false)
    }
    return false
  }
  const currentImport = activeImport.data ?? importJob
  // Refresh the directory retained behind the import tab after background work finishes.
  const completedJobs = (importHistory.data?.items ?? [])
    .filter((job) => ['completed', 'cancelled'].includes(job.status))
    .map((job) => job.id).sort().join(',')
  useEffect(() => {
    if (!completedJobs && !['completed', 'cancelled'].includes(currentImport?.status ?? '')) return
    void queryClient.invalidateQueries({ queryKey: ['contacts', workspaceId] })
    void queryClient.invalidateQueries({ queryKey: ['total-contacts', workspaceId] })
  }, [completedJobs, currentImport?.status, queryClient, workspaceId])
  const total = currentImport?.counters.total ?? 0
  const terminal = (currentImport?.counters.succeeded ?? 0) + (currentImport?.counters.failed ?? 0)
  const percent = total === 0 ? 0 : Math.round((terminal / total) * 100)
  const activeStatus = currentImport?.status
  const importStep = activeStatus === 'completed' || activeStatus === 'needs_attention' || activeStatus === 'retrying' || activeStatus === 'cancelled' ? 4 : activeStatus === 'rejected' ? 2 : activeStatus === 'staged' || activeStatus === 'processing' ? 3 : 0
  const cancelImport = async (job: ImportJob) => {
    if (!canWrite) return
    await importJobApi.cancel(workspaceId, job.id)
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['customer-imports', workspaceId] }),
      queryClient.invalidateQueries({ queryKey: ['customer-import', workspaceId, job.id] })
    ])
    message.success(t`Import cancelled; every unfinished row is recorded as failed.`)
  }
  const retryImport = async (job: ImportJob) => {
    if (!canWrite || retryingJobId) return
    setRetryingJobId(job.id)
    try {
      await importJobApi.retry(workspaceId, job.id)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['customer-imports', workspaceId] }),
        queryClient.invalidateQueries({ queryKey: ['customer-import', workspaceId, job.id] })
      ])
      message.success(t`Failed rows were queued for retry.`)
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Unable to retry failed rows`)
    } finally {
      setRetryingJobId(undefined)
    }
  }
  const renderBoundLists = (listIDs: string[] | undefined) => {
    if (!listIDs?.length) return <Typography.Text type="secondary">{t`No list binding`}</Typography.Text>
    return <Space wrap>{listIDs.map((listID) => <Tag key={listID}>{listNameByID.get(listID) || listID}</Tag>)}</Space>
  }

  return (
    <Card>
      <Alert
        type="info"
        showIcon
        title={t`Imports are durable and close-safe`}
        description={t`Every row is recorded before processing. Existing list suppression states are preserved when imported customers are added to lists.`}
        className="mb-4"
      />
      {canBindLists ? <div className="mb-5 max-w-xl">
        <Typography.Text strong>{t`Add imported customers to lists`}</Typography.Text>
        <Typography.Paragraph type="secondary" className="mb-2 mt-1">
          {t`Optional. Choose one or more lists; leaving this empty imports customers without changing list membership.`}
        </Typography.Paragraph>
        <Select
          aria-label={t`Target lists`}
          mode="multiple"
          allowClear
          className="w-full"
          value={selectedListIDs}
          onChange={setSelectedListIDs}
          loading={lists.isLoading}
          disabled={lists.isLoading || lists.isError || !canWrite || uploading}
          placeholder={lists.isLoading ? t`Loading lists...` : t`Select lists (optional)`}
          options={(lists.data?.lists ?? []).map((list) => ({ value: list.id, label: list.name }))}
        />
        {lists.isError ? <Alert className="mt-2" type="warning" showIcon title={t`Lists could not be loaded. You can still import without list binding.`} /> : null}
      </div> : null}

      <Steps className="mb-6" current={importStep} items={[
        { title: t`Upload`, content: t`Receive the complete file` },
        { title: t`Map fields`, content: t`Read customer identifiers` },
        { title: t`Validate`, content: t`Retain invalid rows` },
        { title: t`Process`, content: t`Continue after this page closes` },
        { title: t`Complete`, content: t`Review successes and failures` }
      ]} />
      <ImportTemplateButtons workspaceId={workspaceId} />
      <Upload.Dragger
        accept={CONTACT_IMPORT_ACCEPT}
        maxCount={1}
        beforeUpload={(file) => { void upload(file); return false }}
        showUploadList={false}
        disabled={!canWrite || uploading || (canBindLists && lists.isLoading)}
      >
        <p className="ant-upload-drag-icon"><InboxOutlined /></p>
        <p>{uploading ? t`Uploading...` : t`Click or drag a CSV or Excel file here`}</p>
        <p>{t`Include external_id, email or phone; at least one customer identifier is required.`}</p>
        <p>{t`Excel imports the first worksheet. Keep identifiers and phone numbers as text to preserve leading zeros and long numbers.`}</p>
      </Upload.Dragger>

      {currentImport ? (
        <Card size="small" className="mt-4">
          <Space orientation="vertical" className="w-full">
            <Space wrap><Typography.Text strong>{currentImport.filename}</Typography.Text><Tag>{currentImport.status}</Tag>{renderBoundLists(currentImport.list_ids)}</Space>
            <Progress percent={percent} status={['rejected', 'cancelled', 'needs_attention'].includes(currentImport.status) ? 'exception' : currentImport.status === 'completed' ? 'success' : 'active'} />
            <Descriptions size="small" column={{ xs: 2, sm: 5 }} items={[
              { key: 'total', label: t`Total`, children: total },
              { key: 'pending', label: t`Pending`, children: currentImport.counters.pending },
              { key: 'processing', label: t`Processing`, children: currentImport.counters.processing },
              { key: 'success', label: t`Succeeded`, children: currentImport.counters.succeeded },
              { key: 'failed', label: t`Failed`, children: currentImport.counters.failed }
            ]} />
            <Space>
              {currentImport.counters.failed > 0 ? <Button icon={<DownloadOutlined />} onClick={() => void importJobApi.downloadErrors(workspaceId, currentImport.id)}>{t`Download failed rows`}</Button> : null}
              {canWrite && currentImport.status === 'needs_attention' ? <Button loading={retryingJobId === currentImport.id} onClick={() => void retryImport(currentImport)}>{t`Retry failed rows`}</Button> : null}
              {canWrite && ['uploading', 'staged', 'processing'].includes(currentImport.status) ? <Popconfirm title={t`Unfinished rows will be marked as failed; successful rows are not rolled back.`} onConfirm={() => cancelImport(currentImport)}><Button danger icon={<StopOutlined />}>{t`Cancel import`}</Button></Popconfirm> : null}
            </Space>
          </Space>
        </Card>
      ) : null}

      <Card size="small" title={t`Recent imports`} className="mt-4">
        {importHistory.isError && <ActionableError error={importHistory.error} onRetry={() => void importHistory.refetch()} retrying={importHistory.isFetching} />}
        {activeImport.isError && <ActionableError error={activeImport.error} onRetry={() => void activeImport.refetch()} retrying={activeImport.isFetching} />}
        <Table<ImportJob>
          rowKey="id"
          loading={importHistory.isLoading}
          pagination={false}
          dataSource={importHistory.data?.items ?? []}
          scroll={{ x: 760 }}
          columns={[
            { title: t`File`, dataIndex: 'filename' },
            { title: t`Lists`, dataIndex: 'list_ids', render: renderBoundLists },
            { title: t`Status`, dataIndex: 'status', render: (status: ImportJob['status']) => <Tag color={status === 'needs_attention' ? 'orange' : undefined}>{status}</Tag> },
            { title: t`Total`, render: (_: unknown, job: ImportJob) => job.counters.total },
            { title: t`Succeeded`, render: (_: unknown, job: ImportJob) => job.counters.succeeded },
            { title: t`Failed`, render: (_: unknown, job: ImportJob) => job.counters.failed },
            { title: t`Actions`, render: (_: unknown, job: ImportJob) => <Space><Button size="small" onClick={() => setImportJob(job)}>{t`View`}</Button>{job.counters.failed > 0 ? <Button size="small" onClick={() => void importJobApi.downloadErrors(workspaceId, job.id)}>{t`Failed rows`}</Button> : null}{canWrite && job.status === 'needs_attention' ? <Button size="small" loading={retryingJobId === job.id} onClick={() => void retryImport(job)}>{t`Retry`}</Button> : null}</Space> }
          ]}
        />
      </Card>
    </Card>
  )
}
