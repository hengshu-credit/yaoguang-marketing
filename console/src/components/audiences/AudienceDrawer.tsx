import { Alert, Button, Drawer, Form, Input, Space, Spin, Typography } from 'antd'
import { useEffect, useMemo, useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { useQueryClient } from '@tanstack/react-query'
import { audienceApi } from '../../services/api/marketing'
import type { List, TreeNode } from '../../services/api/segment'
import { TreeNodeInput } from '../segment/input'
import { isTreeQueryable } from '../segment/tree_completeness'
import { TableSchemas } from '../segment/table_schemas'

interface AudienceDrawerProps {
  open: boolean
  workspaceId: string
  lists: List[]
  onClose: () => void
  onSaved?: () => void
  customFieldLabels?: Record<string, string>
}

const emptyTree = (): TreeNode => ({
  kind: 'branch',
  branch: { operator: 'and', leaves: [] }
})

export function AudienceDrawer({
  open,
  workspaceId,
  lists,
  onClose,
  onSaved,
  customFieldLabels
}: AudienceDrawerProps) {
  const { t } = useLingui()
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [tree, setTree] = useState<TreeNode>(emptyTree)
  const [draftTree, setDraftTree] = useState<TreeNode>()
  const [previewTotal, setPreviewTotal] = useState<number>()
  const [previewLoading, setPreviewLoading] = useState(false)
  const [previewStale, setPreviewStale] = useState(false)
  const [previewError, setPreviewError] = useState<string>()
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string>()

  const schemas = useMemo(() => ({
    contacts: TableSchemas.contacts,
    contact_lists: TableSchemas.contact_lists,
    contact_timeline: TableSchemas.contact_timeline,
    custom_events_goals: TableSchemas.custom_events_goals
  }), [])
  const effectiveTree = draftTree ?? tree
  const effectiveHash = JSON.stringify(effectiveTree)

  useEffect(() => {
    if (!open || !isTreeQueryable(effectiveTree)) {
      setPreviewStale(previewTotal !== undefined)
      return
    }
    let active = true
    setPreviewStale(previewTotal !== undefined)
    setPreviewError(undefined)
    const timer = window.setTimeout(async () => {
      setPreviewLoading(true)
      try {
        const response = await audienceApi.preview(workspaceId, { condition: effectiveTree })
        if (!active) return
        setPreviewTotal(response.total)
        setPreviewStale(false)
      } catch (error) {
        if (!active) return
        setPreviewError(error instanceof Error ? error.message : t`Failed to preview audience`)
        setPreviewStale(previewTotal !== undefined)
      } finally {
        if (active) setPreviewLoading(false)
      }
    }, 350)
    return () => {
      active = false
      window.clearTimeout(timer)
    }
    // effectiveHash deliberately makes a deep condition edit observable.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, workspaceId, effectiveHash])

  const resetAndClose = () => {
    setName('')
    setDescription('')
    setTree(emptyTree())
    setDraftTree(undefined)
    setPreviewTotal(undefined)
    setPreviewError(undefined)
    setSaveError(undefined)
    onClose()
  }

  const save = async () => {
    if (!name.trim() || !isTreeQueryable(tree)) return
    setSaving(true)
    setSaveError(undefined)
    try {
      await audienceApi.create(workspaceId, name.trim(), description.trim(), { condition: tree }, 'dynamic')
      await queryClient.invalidateQueries({ queryKey: ['audiences', workspaceId] })
      onSaved?.()
      resetAndClose()
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : t`Failed to save audience`)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Drawer
      open={open}
      size="90%"
      title={t`Create dynamic audience`}
      onClose={resetAndClose}
      destroyOnHidden
      extra={(
        <Space>
          <Button onClick={resetAndClose}>{t`Cancel`}</Button>
          <Button type="primary" loading={saving} disabled={!name.trim() || !isTreeQueryable(tree)} onClick={() => void save()}>
            {t`Save audience`}
          </Button>
        </Space>
      )}
    >
      <Alert
        type="info"
        showIcon
        className="mb-4"
        title={t`This page stores filter logic, not a live member list`}
        description={t`Marketing runs select the latest audience when execution starts, freeze the candidates, and check every customer again immediately before each message.`}
      />
      <Form layout="vertical">
        <Form.Item label={t`Audience name`} htmlFor="audience-name" required>
          <Input id="audience-name" value={name} onChange={(event) => setName(event.target.value)} />
        </Form.Item>
        <Form.Item label={t`Description`}>
          <Input.TextArea value={description} onChange={(event) => setDescription(event.target.value)} />
        </Form.Item>
        <Form.Item label={t`Filter conditions`} required>
          <TreeNodeInput
            value={tree}
            onChange={(next) => {
              setTree(next)
              setDraftTree(undefined)
            }}
            onDraftTreeChange={setDraftTree}
            schemas={schemas}
            lists={lists}
            workspaceId={workspaceId}
            customFieldLabels={customFieldLabels}
          />
        </Form.Item>
      </Form>
      <div className={`mt-4 ${previewStale ? 'opacity-50' : ''}`}>
        <Space>
          {previewLoading && <Spin size="small" />}
          {previewTotal !== undefined && (
            <Typography.Text strong>{t`${previewTotal} customers match`}</Typography.Text>
          )}
          {previewTotal === undefined && !previewLoading && (
            <Typography.Text type="secondary">{t`Complete a condition to preview the audience size`}</Typography.Text>
          )}
        </Space>
      </div>
      {previewError && <Alert className="mt-3" type="warning" showIcon message={previewError} />}
      {saveError && <Alert className="mt-3" type="error" showIcon message={saveError} />}
    </Drawer>
  )
}
