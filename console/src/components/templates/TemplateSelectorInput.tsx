import React, { useState, useEffect } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Input, Drawer, List, Empty, Spin, Button, Space } from 'antd'
import { EyeOutlined, SearchOutlined, PlusOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { templatesApi } from '../../services/api/template'
import type { Template, TemplateChannel, Workspace } from '../../services/api/types'
import TemplatePreviewPopover from './TemplatePreviewDrawer'
import { useAuth } from '../../contexts/AuthContext'

interface TemplateSelectorInputProps {
  value?: string | null
  onChange?: (value: string | null) => void
  workspaceId: string
  category?: string
  purpose?: Template['category_purpose']
  placeholder?: string
  channel?: TemplateChannel
  clearable?: boolean
  disabled?: boolean
  size?: 'small' | 'middle' | 'large'
}

const TemplateSelectorInput: React.FC<TemplateSelectorInputProps> = ({
  value,
  onChange,
  workspaceId,
  category,
  purpose,
  channel = 'email',
  placeholder,
  clearable = true,
  disabled = false,
  size
}) => {
  const { t } = useLingui()
  const defaultPlaceholder = placeholder || t`Select a template`
  const [open, setOpen] = useState<boolean>(false)
  const [selectedTemplate, setSelectedTemplate] = useState<Template | null>(null)
  const [searchQuery, setSearchQuery] = useState<string>('')
  const { workspaces } = useAuth()

  // Find the current workspace from the workspaces array
  const currentWorkspace = workspaces.find((workspace) => workspace.id === workspaceId)

  // Fetch templates with optional category filter
  const {
    data: templatesResponse,
    isLoading,
    refetch,
  } = useQuery({
    queryKey: ['templates', workspaceId, category, purpose, channel],
    queryFn: async () => {
      const response = await templatesApi.list({
        workspace_id: workspaceId,
        category,
        channel
      })
      return response
    },
    enabled: !!workspaceId,
    refetchOnWindowFocus: true
  })

  // Keep the displayed template in sync with the controlled `value` prop.
  // The same component instance can be reused while `value` changes — e.g. switching
  // between email nodes in the automation editor — so we must re-resolve the template
  // whenever `value` no longer matches what we're showing, not just on first load.
  useEffect(() => {
    // Value cleared → clear the displayed template
    if (!value) {
      if (selectedTemplate) setSelectedTemplate(null)
      return
    }
    // Already showing the right template
    if (selectedTemplate?.id === value && selectedTemplate.channel === channel) return
    if (!workspaceId) return

    // Guard against out-of-order responses when `value` changes rapidly
    let cancelled = false
    templatesApi
      .get({ workspace_id: workspaceId, id: value })
      .then((response) => {
        if (cancelled || !response.template) return
        if (response.template.channel !== channel) {
          setSelectedTemplate(null)
          onChange?.(null)
          return
        }
        setSelectedTemplate(response.template)
      })
      .catch((error) => {
        console.error('Failed to fetch template details:', error)
      })
    return () => {
      cancelled = true
    }
  }, [value, workspaceId, channel, selectedTemplate, onChange])

  // Get templates array from response
  const templates = (templatesResponse?.templates || []).filter((template) => {
    if (template.channel !== channel) return false
    if (!purpose) return true
    return (
      template.category_purpose === purpose ||
      (!template.category_purpose && template.category === purpose)
    )
  })

  // Filter templates based on search query
  const filteredTemplates = templates.filter((template) =>
    template.name.toLowerCase().includes(searchQuery.toLowerCase())
  )

  const handleSelect = (template: Template) => {
    setSelectedTemplate(template)
    onChange?.(template.id)
    setOpen(false)
  }

  const showDrawer = () => {
    if (!disabled) {
      void refetch()
      setOpen(true)
    }
  }

  const onClose = () => {
    setOpen(false)
    setSearchQuery('')
  }

  const openTemplateManager = () => {
    const url = `/console/workspace/${encodeURIComponent(workspaceId)}/templates?create_channel=${encodeURIComponent(channel)}`
    window.open(url, '_blank', 'noopener,noreferrer')
  }

  if (!currentWorkspace) {
    return <div>{t`Loading...`}</div>
  }

  return (
    <>
      <Space.Compact style={{ width: '100%' }}>
        <Input
          value={selectedTemplate?.name || ''}
          placeholder={defaultPlaceholder}
          readOnly={!clearable}
          disabled={disabled}
          onClick={showDrawer}
          onClear={() => {
            setSelectedTemplate(null)
            onChange?.(null)
          }}
          allowClear={clearable}
          size={size}
          style={{ flex: 1 }}
        />
        {selectedTemplate && currentWorkspace && (
          <TemplatePreviewPopover record={selectedTemplate} workspace={currentWorkspace}>
            <Button icon={<EyeOutlined />} />
          </TemplatePreviewPopover>
        )}
      </Space.Compact>

      <Drawer
        title={t`Select Template`}
        size={600}
        onClose={onClose}
        open={open}
        styles={{
          body: { paddingBottom: 80 }
        }}
      >
        <div style={{ marginBottom: 16, display: 'flex', gap: 8 }}>
          <Input
            placeholder={t`Search templates...`}
            prefix={<SearchOutlined />}
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            style={{ flex: 1 }}
          />
          <Button type="primary" icon={<PlusOutlined />} onClick={openTemplateManager}>
            {t`Create new ${channel.toUpperCase()} template`}
          </Button>
        </div>

        {isLoading ? (
          <div style={{ textAlign: 'center', padding: '40px 0' }}>
            <Spin size="large" />
          </div>
        ) : filteredTemplates.length > 0 ? (
          <List
            itemLayout="horizontal"
            bordered
            dataSource={filteredTemplates}
            size="small"
            renderItem={(template) => (
              <List.Item
                actions={[
                  <TemplatePreviewPopover
                    key="preview"
                    record={template}
                    workspace={currentWorkspace as Workspace}
                  >
                    <Button type="text" icon={<EyeOutlined />} />
                  </TemplatePreviewPopover>,
                  <Button key="select" type="link" onClick={() => handleSelect(template)}>
                    {t`Select`}
                  </Button>
                ]}
              >
                <List.Item.Meta
                  title={
                    <a onClick={() => handleSelect(template)} style={{ cursor: 'pointer' }}>
                      {template.name}
                    </a>
                  }
                  description={template.category || t`No category`}
                />
              </List.Item>
            )}
          />
        ) : (
          <Empty
            description={
              category
                ? t`No templates found for ${category.replace('_', ' ')} category`
                : t`No templates found`
            }
            image={Empty.PRESENTED_IMAGE_SIMPLE}
          >
            <Button type="primary" icon={<PlusOutlined />} onClick={openTemplateManager}>
              {t`Create new ${channel.toUpperCase()} template`}
            </Button>
          </Empty>
        )}
      </Drawer>
    </>
  )
}

export default TemplateSelectorInput
