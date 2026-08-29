import { useMemo, useState } from 'react'
import {
  Alert,
  App,
  Button,
  Col,
  Drawer,
  Form,
  Input,
  Row,
  Segmented,
  Select,
  Space,
  Tabs
} from 'antd'
import type { ButtonProps } from 'antd'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import type {
  CreateTemplateRequest,
  PreviewTemplateRequest,
  PreviewTemplateResponse,
  PushTemplate,
  SMSTemplate,
  Template,
  TemplateTranslation,
  UpdateTemplateRequest
} from '../../services/api/template'
import type { Workspace } from '../../services/api/types'
import { templatesApi } from '../../services/api/template'
import ChannelMessagePreview from './ChannelMessagePreview'

type MessageChannel = 'sms' | 'push'

interface MessageTemplateDrawerProps {
  workspace: Workspace
  template?: Template
  fromTemplate?: Template
  buttonContent?: React.ReactNode
  buttonProps?: ButtonProps
  onClose?: () => void
}

interface TranslationFormValue {
  sms?: { body?: string; sender_id?: string }
  push?: {
    title?: string
    body?: string
    image_url?: string
    deep_link?: string
    data_json?: string
  }
}

interface MessageTemplateFormValues {
  id: string
  name: string
  channel: MessageChannel
  category: string
  sms_body?: string
  sender_id?: string
  push_title?: string
  push_body?: string
  image_url?: string
  deep_link?: string
  push_data?: string
  test_data?: string
  translations?: Record<string, TranslationFormValue>
}

const parseJSONObject = (value: string | undefined, label: string): Record<string, unknown> => {
  if (!value?.trim()) return {}
  const parsed: unknown = JSON.parse(value)
  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error(`${label} must be a JSON object`)
  }
  return parsed as Record<string, unknown>
}

const MessageTemplateDrawer: React.FC<MessageTemplateDrawerProps> = ({
  workspace,
  template,
  fromTemplate,
  buttonContent,
  buttonProps,
  onClose
}) => {
  const { t } = useLingui()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [form] = Form.useForm<MessageTemplateFormValues>()
  const [open, setOpen] = useState(false)
  const [preview, setPreview] = useState<PreviewTemplateResponse | null>(null)
  const [previewError, setPreviewError] = useState<string | null>(null)
  const [platform, setPlatform] = useState<'android' | 'ios' | 'web'>('android')
  const channel = Form.useWatch('channel', form) || 'sms'
  const sourceTemplate = template || fromTemplate

  const defaultLanguage = workspace.settings?.default_language || 'en'
  const [previewLanguage, setPreviewLanguage] = useState(defaultLanguage)
  const languages = useMemo(() => {
    const configured = workspace.settings?.languages || []
    return [defaultLanguage, ...configured.filter((language) => language !== defaultLanguage)]
  }, [defaultLanguage, workspace.settings?.languages])

  const initialValues = useMemo<MessageTemplateFormValues>(() => {
    const existingChannel: MessageChannel = sourceTemplate?.channel === 'push' ? 'push' : 'sms'
    const translations: Record<string, TranslationFormValue> = {}
    for (const [language, translation] of Object.entries(sourceTemplate?.translations || {})) {
      translations[language] = {
        sms: translation.sms,
        push: translation.push
          ? { ...translation.push, data_json: JSON.stringify(translation.push.data || {}, null, 2) }
          : undefined
      }
    }
    return {
      id: template?.id || '',
      name: fromTemplate ? `${fromTemplate.name} copy` : template?.name || '',
      channel: existingChannel,
      category: sourceTemplate?.category || 'marketing',
      sms_body: sourceTemplate?.sms?.body || '',
      sender_id: sourceTemplate?.sms?.sender_id || '',
      push_title: sourceTemplate?.push?.title || '',
      push_body: sourceTemplate?.push?.body || '',
      image_url: sourceTemplate?.push?.image_url || '',
      deep_link: sourceTemplate?.push?.deep_link || '',
      push_data: JSON.stringify(sourceTemplate?.push?.data || {}, null, 2),
      test_data: JSON.stringify(sourceTemplate?.test_data || {}, null, 2),
      translations
    }
  }, [fromTemplate, sourceTemplate, template])

  const buildPayload = (values: MessageTemplateFormValues) => {
    const translations: Record<string, TemplateTranslation> = {}
    for (const language of languages) {
      if (language === defaultLanguage) continue
      const translated = values.translations?.[language]
      if (values.channel === 'sms' && translated?.sms?.body?.trim()) {
        translations[language] = {
          sms: { body: translated.sms.body, sender_id: translated.sms.sender_id }
        }
      }
      if (values.channel === 'push' && translated?.push?.title?.trim() && translated.push.body?.trim()) {
        translations[language] = {
          push: {
            title: translated.push.title,
            body: translated.push.body,
            image_url: translated.push.image_url,
            deep_link: translated.push.deep_link,
            data: parseJSONObject(translated.push.data_json, `${language} push data`)
          }
        }
      }
    }

    const sms: SMSTemplate | undefined =
      values.channel === 'sms' ? { body: values.sms_body || '', sender_id: values.sender_id } : undefined
    const push: PushTemplate | undefined =
      values.channel === 'push'
        ? {
            title: values.push_title || '',
            body: values.push_body || '',
            image_url: values.image_url,
            deep_link: values.deep_link,
            data: parseJSONObject(values.push_data, 'Push data')
          }
        : undefined

    return {
      workspace_id: workspace.id,
      id: values.id,
      name: values.name,
      channel: values.channel,
      category: values.category,
      sms,
      push,
      translations,
      test_data: parseJSONObject(values.test_data, 'Template data')
    }
  }

  const saveMutation = useMutation({
    mutationFn: async (values: MessageTemplateFormValues) => {
      const payload = buildPayload(values)
      if (template) {
        return templatesApi.update({
          ...payload,
          base_version: template.version
        } as UpdateTemplateRequest)
      }
      return templatesApi.create(payload as CreateTemplateRequest)
    },
    onSuccess: () => {
      message.success(template ? t`Template updated successfully` : t`Template created successfully`)
      queryClient.invalidateQueries({ queryKey: ['templates', workspace.id] })
      closeDrawer()
    },
    onError: (error: Error) => message.error(error.message)
  })

  const previewMutation = useMutation({
    mutationFn: async (values: MessageTemplateFormValues) => {
      const payload = buildPayload(values)
      return templatesApi.preview({
        workspace_id: payload.workspace_id,
        channel: payload.channel,
        sms: payload.sms,
        push: payload.push,
        translations: payload.translations,
        language: previewLanguage,
        platform: payload.channel === 'push' ? platform : undefined,
        test_data: payload.test_data
      } as PreviewTemplateRequest)
    },
    onSuccess: (result) => {
      setPreview(result)
      setPreviewError(null)
    },
    onError: (error: Error) => {
      setPreview(null)
      setPreviewError(error.message)
    }
  })

  const closeDrawer = () => {
    setOpen(false)
    setPreview(null)
    setPreviewError(null)
    setPlatform('android')
    setPreviewLanguage(defaultLanguage)
    form.resetFields()
    onClose?.()
  }

  const openDrawer = () => {
    setOpen(true)
  }

  const handlePreview = async () => {
    setPreviewError(null)
    try {
      const values = await form.validateFields()
      previewMutation.mutate(values)
    } catch (error) {
      const validationError = error as {
        errorFields?: Array<{ errors?: string[] }>
      }
      const validationMessages = validationError.errorFields
        ?.flatMap((field) => field.errors || [])
        .filter(Boolean)
      if (validationMessages?.length) {
        setPreviewError(validationMessages.join('; '))
      } else if (error instanceof Error) {
        setPreviewError(error.message)
      } else {
        setPreviewError(t`Complete the required fields before previewing.`)
      }
    }
  }

  const jsonValidator = (label: string) => async (_: unknown, value: string | undefined) => {
    try {
      parseJSONObject(value, label)
    } catch (error) {
      throw new Error(error instanceof Error ? error.message : `${label} is invalid`)
    }
  }

  const languageTabs = languages.map((language) => {
    const isDefault = language === defaultLanguage
    const smsBodyName = isDefault ? 'sms_body' : ['translations', language, 'sms', 'body']
    const senderName = isDefault ? 'sender_id' : ['translations', language, 'sms', 'sender_id']
    const pushTitleName = isDefault ? 'push_title' : ['translations', language, 'push', 'title']
    const pushBodyName = isDefault ? 'push_body' : ['translations', language, 'push', 'body']
    const imageName = isDefault ? 'image_url' : ['translations', language, 'push', 'image_url']
    const deepLinkName = isDefault ? 'deep_link' : ['translations', language, 'push', 'deep_link']
    const pushDataName = isDefault ? 'push_data' : ['translations', language, 'push', 'data_json']

    return {
      key: language,
      label: isDefault ? `${language} (${t`default`})` : language,
      children:
        channel === 'sms' ? (
          <>
            <Form.Item label={t`Sender ID`} name={senderName}>
              <Input maxLength={32} placeholder={t`Optional alphanumeric sender`} />
            </Form.Item>
            <Form.Item
              label={t`Message`}
              name={smsBodyName}
              rules={isDefault ? [{ required: true, message: t`Message is required` }] : undefined}
            >
              <Input.TextArea autoSize={{ minRows: 6, maxRows: 14 }} maxLength={10000} />
            </Form.Item>
          </>
        ) : (
          <>
            <Form.Item
              label={t`Title`}
              name={pushTitleName}
              rules={isDefault ? [{ required: true, message: t`Title is required` }] : undefined}
            >
              <Input maxLength={512} />
            </Form.Item>
            <Form.Item
              label={t`Message`}
              name={pushBodyName}
              rules={isDefault ? [{ required: true, message: t`Message is required` }] : undefined}
            >
              <Input.TextArea autoSize={{ minRows: 4, maxRows: 10 }} maxLength={4096} />
            </Form.Item>
            <Form.Item label={t`Image URL`} name={imageName}>
              <Input placeholder="https://..." />
            </Form.Item>
            <Form.Item label={t`Deep link`} name={deepLinkName}>
              <Input placeholder="myapp://..." />
            </Form.Item>
            <Form.Item
              label={t`Custom data (JSON)`}
              name={pushDataName}
              rules={[{ validator: jsonValidator(`${language} push data`) }]}
            >
              <Input.TextArea autoSize={{ minRows: 3, maxRows: 10 }} className="font-mono" />
            </Form.Item>
          </>
        )
    }
  })

  return (
    <>
      <Button {...buttonProps} onClick={openDrawer}>
        {buttonContent || (template ? t`Edit Template` : t`Create SMS / Push`)}
      </Button>
      <Drawer
        title={template ? t`Edit message template` : t`Create message template`}
        open={open}
        onClose={closeDrawer}
        size={1180}
        destroyOnHidden
        extra={
          <Space>
            <Button
              onClick={handlePreview}
              loading={previewMutation.isPending}
            >
              {t`Preview`}
            </Button>
            <Button type="primary" loading={saveMutation.isPending} onClick={() => form.submit()}>
              {t`Save`}
            </Button>
          </Space>
        }
      >
        <Row gutter={24}>
          <Col xs={24} lg={13}>
            <Form<MessageTemplateFormValues>
              form={form}
              layout="vertical"
              initialValues={initialValues}
              onFinish={(values) => saveMutation.mutate(values)}
            >
              <Row gutter={12}>
                <Col span={12}>
                  <Form.Item label="API ID" name="id" rules={[{ required: true }]}>
                    <Input disabled={Boolean(template)} maxLength={32} />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item label={t`Name`} name="name" rules={[{ required: true }]}>
                    <Input maxLength={32} />
                  </Form.Item>
                </Col>
              </Row>
              <Row gutter={12}>
                <Col span={12}>
                  <Form.Item label={t`Channel`} name="channel">
                    <Segmented
                      block
                      disabled={Boolean(template)}
                      options={[
                        { label: 'SMS', value: 'sms' },
                        { label: t`Push`, value: 'push' }
                      ]}
                    />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item label={t`Category`} name="category" rules={[{ required: true }]}>
                    <Select
                      options={[
                        { label: t`Marketing`, value: 'marketing' },
                        { label: t`Transactional`, value: 'transactional' },
                        { label: t`Welcome`, value: 'welcome' },
                        { label: t`Other`, value: 'other' }
                      ]}
                    />
                  </Form.Item>
                </Col>
              </Row>
              <Tabs
                activeKey={previewLanguage}
                items={languageTabs}
                onChange={(language) => {
                  setPreviewLanguage(language)
                  setPreview(null)
                }}
              />
              <Form.Item
                label={t`Template data (JSON)`}
                name="test_data"
                rules={[{ validator: jsonValidator('Template data') }]}
              >
                <Input.TextArea autoSize={{ minRows: 5, maxRows: 14 }} className="font-mono" />
              </Form.Item>
            </Form>
          </Col>
          <Col xs={24} lg={11}>
            {previewError && <Alert type="error" showIcon title={previewError} className="mb-4" />}
            {preview ? (
              <ChannelMessagePreview
                preview={preview}
                platform={platform}
                onPlatformChange={(nextPlatform) => {
                  setPlatform(nextPlatform)
                  setPreview(null)
                }}
              />
            ) : (
              <Alert
                type="info"
                showIcon
                title={t`Preview uses the server Liquid renderer and your current unsaved values.`}
              />
            )}
          </Col>
        </Row>
      </Drawer>
    </>
  )
}

export default MessageTemplateDrawer
