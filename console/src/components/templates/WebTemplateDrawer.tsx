import { useRef, useState } from 'react'
import { Alert, App, Button, Col, Drawer, Form, Input, Row, Tabs } from 'antd'
import type { ButtonProps } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { Template, WebTemplate } from '../../services/api/template'
import { templatesApi } from '../../services/api/template'
import type { Workspace } from '../../services/api/types'
import { NotifuseEditor, type NotifuseEditorRef } from '../blog_editor'
import { extractTextContent, jsonToHtml } from '../blog/utils'
import EmailClientFrame from './EmailClientFrame'
import TemplateCategorySelect from './TemplateCategorySelect'
import { webTemplateHtml } from './webTemplateContent'

interface WebTemplateDrawerProps {
  workspace: Workspace
  template?: Template
  fromTemplate?: Template
  buttonProps?: ButtonProps
  buttonContent?: React.ReactNode
  onClose?: () => void
}

export default function WebTemplateDrawer({ workspace, template, fromTemplate, buttonProps, buttonContent, onClose }: WebTemplateDrawerProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [form] = Form.useForm<{ id: string; name: string; category: string }>()
  const watchedName = Form.useWatch('name', form)
  const source = template || fromTemplate
  const defaultLanguage = workspace.settings.default_language || 'en'
  const languages = Array.from(new Set([defaultLanguage, ...(workspace.settings.languages || []), ...Object.keys(source?.translations || {})]))
  const initialDrafts = (): Record<string, WebTemplate> => Object.fromEntries(languages.map(language => [language,
    (language === defaultLanguage ? source?.web : source?.translations?.[language]?.web) || { content: { type: 'doc', content: [] }, html: '' }
  ]))
  const [drafts, setDrafts] = useState(initialDrafts)
  const [language, setLanguage] = useState(defaultLanguage)
  const [open, setOpen] = useState(false)
  const editor = useRef<NotifuseEditorRef>(null)
  const draft = drafts[language]
  const close = () => { setOpen(false); onClose?.() }
  const save = useMutation({
    mutationFn: async (values: { id: string; name: string; category: string }) => {
      if (!webTemplateHtml(drafts[defaultLanguage]).trim()) throw new Error(t`Content is required`)
      const translations = { ...source?.translations }
      for (const current of languages) {
        if (current === defaultLanguage) continue
        if (webTemplateHtml(drafts[current]).trim()) translations[current] = { web: drafts[current] }
        else delete translations[current]
      }
      const payload = { ...values, workspace_id: workspace.id, channel: 'web', web: drafts[defaultLanguage], translations, test_data: source?.test_data }
      return template ? templatesApi.update({ ...payload, base_version: template.version }) : templatesApi.create(payload)
    },
    onSuccess: () => { void queryClient.invalidateQueries({ queryKey: ['templates', workspace.id] }); message.success(template ? t`Template updated successfully` : t`Template created successfully`); close() }
  })
  return <>
    <Button {...buttonProps} onClick={() => { form.resetFields(); setDrafts(initialDrafts()); setLanguage(defaultLanguage); save.reset(); setOpen(true) }}>{buttonContent || (template ? t`Edit Template` : t`Create template`)}</Button>
    <Drawer title={template ? t`Edit Template` : t`Create template`} open={open} onClose={close} size={1180} destroyOnHidden extra={<Button type="primary" loading={save.isPending} onClick={() => form.submit()}>{t`Save`}</Button>}>
      {save.error && <Alert className="mb-4" type="error" showIcon title={save.error.message} />}
      <Row gutter={24}>
        <Col xs={24} lg={13}>
          <Form form={form} layout="vertical" initialValues={{ id: template?.id || '', name: fromTemplate ? `${fromTemplate.name} copy` : template?.name || '', category: source?.category || 'marketing' }} onFinish={values => save.mutate(values)}>
            <Form.Item name="id" label="API ID" rules={[{ required: true }]}><Input maxLength={32} disabled={Boolean(template)} /></Form.Item>
            <Form.Item name="name" label={t`Name`} rules={[{ required: true }]}><Input maxLength={32} /></Form.Item>
            <Form.Item name="category" label={t`Category`} rules={[{ required: true }]}><TemplateCategorySelect workspaceId={workspace.id} /></Form.Item>
          </Form>
          <Tabs activeKey={language} onChange={setLanguage} items={languages.map(current => ({ key: current, label: current === defaultLanguage ? `${current} (${t`default`})` : current }))} />
          <div className="rounded-lg border border-slate-200 bg-white p-4">
            <NotifuseEditor key={`${open}:${language}`} ref={editor} initialContent={webTemplateHtml(draft)} showHeader={false} onChange={json => {
              if (typeof json.type !== 'string') return
              const tree = json as unknown as Parameters<typeof jsonToHtml>[0]
              setDrafts(previous => ({ ...previous, [language]: { content: json, html: editor.current?.getHTML() || jsonToHtml(tree), plain_text: extractTextContent(tree) } }))
            }} />
          </div>
        </Col>
        <Col xs={24} lg={11}><EmailClientFrame html={webTemplateHtml(draft)} title={watchedName || 'Web'} web /></Col>
      </Row>
    </Drawer>
  </>
}
