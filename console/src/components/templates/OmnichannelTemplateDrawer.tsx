import { useEffect, useMemo, useRef, useState } from 'react'
import { Alert, App, Button, Col, Drawer, Form, Input, Row, Select, Space, Tabs } from 'antd'
import type { ButtonProps } from 'antd'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useDebouncedCallback } from 'use-debounce'
import { useLingui } from '@lingui/react/macro'
import type { ChannelDefinition, ContentFamily } from '../../services/api/channels'
import type { Workspace } from '../../services/api/types'
import type { ChannelTemplateContent, Template, TemplateTranslation } from '../../services/api/template'
import { templatesApi } from '../../services/api/template'
import OmnichannelFields from './OmnichannelFields'
import OmnichannelPreview from './OmnichannelPreview'
import TemplateCategorySelect from './TemplateCategorySelect'

interface OmnichannelTemplateDrawerProps {
  workspace: Workspace
  definitions: ChannelDefinition[]
  defaultChannel?: string
  template?: Template
  fromTemplate?: Template
  buttonContent?: React.ReactNode
  buttonProps?: ButtonProps
  livePreviewDelay?: number | false
}

interface MaterialDraft {
  family?: ContentFamily
  title?: string
  body?: string
  footer?: string
  media?: { type?: 'image' | 'video' | 'audio' | 'file'; url?: string; alt_text?: string }
  cards?: ChannelTemplateContent['cards']
  actions?: ChannelTemplateContent['actions']
  external_template?: ChannelTemplateContent['external_template']
  webhook?: ChannelTemplateContent['webhook']
  data_json?: string
}

interface OmnichannelFormValues {
  id: string
  name: string
  channel: string
  family: ContentFamily
  category: string
  content: MaterialDraft
  translations?: Record<string, { content?: MaterialDraft }>
  test_data?: string
}

const parseObject = (value: string | undefined, label: string): Record<string, unknown> => {
  if (!value?.trim()) return {}
  const parsed: unknown = JSON.parse(value)
  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') throw new Error(`${label} must be a JSON object`)
  return parsed as Record<string, unknown>
}

const normalizeContent = (draft: MaterialDraft | undefined, family: ContentFamily): ChannelTemplateContent => {
  const source = draft || {}
  const content: ChannelTemplateContent = { family }
  if (source.title) content.title = source.title
  if (source.body) content.body = source.body
  if (source.footer) content.footer = source.footer
  if (source.media?.url) content.media = { type: source.media.type || 'image', url: source.media.url, alt_text: source.media.alt_text }
  if (source.cards?.length) content.cards = source.cards
  if (source.actions?.length) content.actions = source.actions
  if (source.external_template) content.external_template = source.external_template
  if (source.webhook) content.webhook = { ...source.webhook, content_type: 'application/json' }
  const data = parseObject(source.data_json, 'Custom data')
  if (Object.keys(data).length) content.data = data
  return content
}

const hasPreviewContent = (content: ChannelTemplateContent): boolean => {
  switch (content.family) {
    case 'external_template': return Boolean(content.external_template?.id && content.external_template?.language)
    case 'webhook_payload': return Boolean(content.webhook?.body?.trim())
    case 'carousel': return Boolean(content.cards?.length && content.cards.every(card => card.body?.trim()))
    case 'notification': return Boolean(content.title?.trim() && content.body?.trim())
    default: return Boolean(content.body?.trim())
  }
}

const OmnichannelTemplateDrawer: React.FC<OmnichannelTemplateDrawerProps> = ({
  workspace, definitions, defaultChannel, template, fromTemplate, buttonContent, buttonProps, livePreviewDelay = 300
}) => {
  const { t } = useLingui()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [form] = Form.useForm<OmnichannelFormValues>()
  const [open, setOpen] = useState(false)
  const [preview, setPreview] = useState<Awaited<ReturnType<typeof templatesApi.preview>> | null>(null)
  const [previewError, setPreviewError] = useState<string | null>(null)
  const [previewPending, setPreviewPending] = useState(false)
  const generation = useRef(0)
  const sourceTemplate = template || fromTemplate
  const genericDefinitions = definitions.filter(item => !['email', 'web', 'sms', 'push'].includes(item.id))
  const channel = Form.useWatch('channel', form) || sourceTemplate?.channel || defaultChannel || genericDefinitions[0]?.id
  const definition = genericDefinitions.find((candidate) => candidate.id === channel) || genericDefinitions[0]
  const family = Form.useWatch('family', form) || definition?.content_families[0]
  const watchedContent = Form.useWatch('content', { form, preserve: true }) as MaterialDraft | undefined
  const watchedTranslations = Form.useWatch('translations', { form, preserve: true }) as OmnichannelFormValues['translations']
  const defaultLanguage = workspace.settings.default_language || 'en'
  const languages = useMemo(() => [defaultLanguage, ...(workspace.settings.languages || []).filter((language) => language !== defaultLanguage)], [defaultLanguage, workspace.settings.languages])
  const [language, setLanguage] = useState(defaultLanguage)
  const [selectedProfile, setProfile] = useState('')
  const profile = definition?.preview_profiles.some(item => item.id === selectedProfile)
    ? selectedProfile : definition?.preview_profiles[0]?.id || ''

  const localContent = useMemo(() => {
    const draft = language === defaultLanguage ? watchedContent : watchedTranslations?.[language]?.content || watchedContent
    try {
      return normalizeContent(draft, family || 'text')
    } catch {
      const fallback: ChannelTemplateContent = { family: family || 'text' }
      if (draft?.title) fallback.title = draft.title
      if (draft?.body) fallback.body = draft.body
      return fallback
    }
  }, [defaultLanguage, family, language, watchedContent, watchedTranslations])
  const localPreview = useMemo(() => ({
    profile: profile || definition?.preview_profiles[0]?.id || '',
    direction: (language === 'ur' || language === 'ar' || language === 'he' ? 'rtl' : 'ltr') as 'rtl' | 'ltr',
    payload_bytes: new TextEncoder().encode(JSON.stringify(localContent)).length,
    message: localContent,
    warnings: []
  }), [definition?.preview_profiles, language, localContent, profile])

  const initialContent = sourceTemplate?.content
  const initialValues: OmnichannelFormValues = {
    id: template?.id || '', name: fromTemplate ? `${fromTemplate.name} copy` : template?.name || '', channel: sourceTemplate?.channel || defaultChannel || genericDefinitions[0]?.id || '',
    family: initialContent?.family || definition?.content_families[0] || 'text', category: sourceTemplate?.category || 'marketing',
    content: initialContent ? { ...initialContent, data_json: JSON.stringify(initialContent.data || {}, null, 2) } : { data_json: '{}' },
    translations: Object.fromEntries(Object.entries(sourceTemplate?.translations || {}).map(([key, value]) => [key, value.content ? { content: { ...value.content, data_json: JSON.stringify(value.content.data || {}, null, 2) } } : {}])),
    test_data: JSON.stringify(sourceTemplate?.test_data || {}, null, 2)
  }

  const buildPayload = (values: OmnichannelFormValues) => {
    const content = normalizeContent(values.content, values.family)
    const translations: Record<string, TemplateTranslation> = {}
    for (const currentLanguage of languages) {
      if (currentLanguage === defaultLanguage) continue
      const translatedDraft = values.translations?.[currentLanguage]?.content
      if (translatedDraft && (translatedDraft.body || translatedDraft.title || translatedDraft.cards?.length || translatedDraft.media?.url || translatedDraft.external_template?.id || translatedDraft.webhook?.body)) {
        translations[currentLanguage] = { content: normalizeContent(translatedDraft, values.family) }
      }
    }
    return {
      workspace_id: workspace.id, id: values.id, name: values.name, channel: values.channel,
      category: values.category, content_schema_version: 1, content, translations,
      test_data: parseObject(values.test_data, 'Template data')
    }
  }

  const requestPreview = async (validate: boolean) => {
    const currentGeneration = ++generation.current
    setPreviewPending(true)
    try {
      // Preview needs message fields; ID/name/category only belong to saving.
      if (validate) await form.validateFields([language === defaultLanguage ? ['content'] : ['translations', language, 'content']], { recursive: true })
      if (currentGeneration !== generation.current) return
      const values = form.getFieldsValue(true)
      if (!values.channel || !values.family) return
      const payload = buildPayload(values)
      // An incomplete draft is normal while typing or switching devices.
      // Explicit preview/save still validates and reports actionable errors.
      if (!validate && (!hasPreviewContent(payload.content) ||
        (payload.translations[language]?.content && !hasPreviewContent(payload.translations[language].content!)))) return
      const result = await templatesApi.preview({
        workspace_id: workspace.id, channel: payload.channel as Template['channel'], content_schema_version: 1,
        content: payload.content, translations: payload.translations, language, profile, test_data: payload.test_data
      })
      if (currentGeneration !== generation.current) return
      setPreview(result)
      setPreviewError(null)
    } catch (error) {
      if (currentGeneration !== generation.current) return
      setPreview(null)
      setPreviewError(error instanceof Error ? error.message : t`Complete the required fields before previewing.`)
    } finally {
      if (currentGeneration === generation.current) setPreviewPending(false)
    }
  }
  const debouncedPreview = useDebouncedCallback(() => void requestPreview(false), livePreviewDelay === false ? 300 : livePreviewDelay)
  const invalidatePreview = () => {
    generation.current++
    setPreview(null)
    setPreviewError(null)
    setPreviewPending(false)
    if (livePreviewDelay !== false) debouncedPreview()
  }
  useEffect(() => () => { generation.current++; debouncedPreview.cancel() }, [debouncedPreview])
  useEffect(() => {
    if (open && sourceTemplate?.content && livePreviewDelay !== false) debouncedPreview()
    return () => debouncedPreview.cancel()
  }, [open, sourceTemplate?.id, sourceTemplate?.version, sourceTemplate?.content, livePreviewDelay, debouncedPreview])

  const saveMutation = useMutation({
    mutationFn: async (values: OmnichannelFormValues) => {
      const payload = buildPayload(values)
      return template
        ? templatesApi.update({ ...payload, base_version: template.version })
        : templatesApi.create(payload)
    },
    onSuccess: () => {
      message.success(template ? t`Template updated successfully` : t`Template created successfully`)
      queryClient.invalidateQueries({ queryKey: ['templates', workspace.id] })
      debouncedPreview.cancel()
      generation.current++
      setOpen(false)
    },
    onError: (error: Error) => message.error(error.message)
  })

  const languageItems = definition ? languages.map((currentLanguage) => ({
    key: currentLanguage,
    label: currentLanguage === defaultLanguage ? `${currentLanguage} (${t`default`})` : currentLanguage,
    children: <OmnichannelFields
      definition={definition}
      family={family}
      required={currentLanguage === defaultLanguage}
      prefix={currentLanguage === defaultLanguage ? 'content' : ['translations', currentLanguage, 'content']}
    />
  })) : []

  if (!definition) return <Alert type="warning" showIcon title={t`This preview profile is not supported.`} />

  return <>
    <Button {...buttonProps} onClick={() => { form.resetFields(); setProfile(''); setLanguage(defaultLanguage); setPreview(null); setPreviewError(null); setPreviewPending(false); setOpen(true) }}>{buttonContent || (template ? t`Edit Template` : t`Create template`)}</Button>
    <Drawer
      title={template ? t`Edit omnichannel template` : t`Create omnichannel template`}
      open={open}
      onClose={() => { debouncedPreview.cancel(); generation.current++; setOpen(false); setPreview(null); setPreviewError(null); setPreviewPending(false) }}
      destroyOnHidden
      size={1180}
      extra={<Space>
        <Button onClick={() => { debouncedPreview.cancel(); void requestPreview(true) }} loading={previewPending}>{t`Preview`}</Button>
        <Button type="primary" onClick={() => form.submit()} loading={saveMutation.isPending}>{t`Save`}</Button>
      </Space>}
    >
      <Row gutter={24}>
        <Col xs={24} lg={13}>
          <Form form={form} layout="vertical" initialValues={initialValues} onFinish={() => saveMutation.mutate(form.getFieldsValue(true))} onValuesChange={invalidatePreview}>
            <Row gutter={12}>
              <Col span={12}><Form.Item label="API ID" name="id" rules={[{ required: true }]}><Input disabled={Boolean(template)} maxLength={32} /></Form.Item></Col>
              <Col span={12}><Form.Item label={t`Name`} name="name" rules={[{ required: true }]}><Input maxLength={32} /></Form.Item></Col>
            </Row>
            <Row gutter={12}>
              <Col span={12}><Form.Item label={t`Channel`} name="channel" rules={[{ required: true }]}><Select disabled={Boolean(template)} options={genericDefinitions.map((item) => ({ label: item.label_key, value: item.id }))} onChange={(next) => { const nextDefinition = genericDefinitions.find((item) => item.id === next); form.setFieldValue('family', nextDefinition?.content_families[0]); setProfile(''); invalidatePreview() }} /></Form.Item></Col>
              <Col span={12}><Form.Item label={t`Category`} name="category" rules={[{ required: true }]}><TemplateCategorySelect workspaceId={workspace.id} /></Form.Item></Col>
            </Row>
            <Form.Item label={t`Message type`} name="family" rules={[{ required: true }]}><Select options={definition.content_families.map((item) => ({ label: item.replace(/_/g, ' '), value: item }))} /></Form.Item>
            <Tabs activeKey={language} items={languageItems} onChange={(next) => { setLanguage(next); invalidatePreview() }} />
            <Form.Item label={t`Template data (JSON)`} name="test_data"><Input.TextArea rows={5} className="font-mono" /></Form.Item>
          </Form>
        </Col>
        <Col xs={24} lg={11}>
          {previewError && <Alert type="error" showIcon title={previewError} className="mb-4" />}
          <OmnichannelPreview key={channel} definition={definition} preview={preview?.channel_preview || localPreview} onProfileChange={(next) => { setProfile(next); invalidatePreview() }} />
          {!preview?.channel_preview && <Alert type="info" showIcon className="mt-4" title={t`Draft preview updates immediately. Server Liquid validation follows automatically.`} />}
        </Col>
      </Row>
    </Drawer>
  </>
}

export default OmnichannelTemplateDrawer
