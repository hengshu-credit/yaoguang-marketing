import { useEffect, useRef, useState } from 'react'
import { Button, Modal } from 'antd'
import type { ButtonProps } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { ChannelDefinition } from '../../services/api/channels'
import type { Template } from '../../services/api/template'
import type { Workspace } from '../../services/api/types'
import ChannelPicker from './ChannelPicker'
import { CreateTemplateDrawer } from './CreateTemplateDrawer'
import MessageTemplateDrawer from './MessageTemplateDrawer'
import OmnichannelTemplateDrawer from './OmnichannelTemplateDrawer'

interface TemplateActionProps {
  workspace: Workspace
  definitions: ChannelDefinition[]
  buttonProps?: ButtonProps
  autoOpenChannel?: string
}

interface TemplateEditorButtonProps extends TemplateActionProps {
  template?: Template
  fromTemplate?: Template
  buttonContent?: React.ReactNode
}

export const TemplateEditorButton: React.FC<TemplateEditorButtonProps> = ({
  workspace, definitions, template, fromTemplate, buttonProps, buttonContent
}) => {
  const source = template || fromTemplate
  if (!source) return null
  if (source.channel === 'email') {
    return <CreateTemplateDrawer workspace={workspace} template={template} fromTemplate={fromTemplate} buttonProps={buttonProps as Record<string, unknown>} buttonContent={buttonContent} />
  }
  if (source.channel === 'sms' || source.channel === 'push') {
    return <MessageTemplateDrawer workspace={workspace} template={template} fromTemplate={fromTemplate} buttonProps={buttonProps} buttonContent={buttonContent} />
  }
  return <OmnichannelTemplateDrawer workspace={workspace} definitions={definitions} template={template} fromTemplate={fromTemplate} buttonProps={buttonProps} buttonContent={buttonContent} />
}

export const CreateTemplateButton: React.FC<TemplateActionProps> = ({ workspace, definitions, buttonProps, autoOpenChannel }) => {
  const { t } = useLingui()
  const [open, setOpen] = useState(false)
  const [selected, setSelected] = useState<string>()
  const autoOpened = useRef(false)
  const definition = definitions.find((item) => item.id === selected)
  const close = () => { setOpen(false); setSelected(undefined) }

  useEffect(() => {
    if (!autoOpened.current && autoOpenChannel && definitions.some((item) => item.id === autoOpenChannel)) {
      autoOpened.current = true
      setSelected(autoOpenChannel)
      setOpen(true)
    }
  }, [autoOpenChannel, definitions])

  return <>
    <Button type="primary" {...buttonProps} onClick={() => setOpen(true)}>{t`Create template`}</Button>
    <Modal title={t`Choose a channel`} open={open} onCancel={close} footer={null} width={920} destroyOnHidden>
      <ChannelPicker definitions={definitions} value={selected} onSelect={setSelected} />
      {definition && <div className="mt-5 flex justify-end border-t border-gray-200 pt-4">
        {definition.id === 'email' ? (
          <CreateTemplateDrawer workspace={workspace} buttonContent={t`Continue with Email`} onClose={close} />
        ) : definition.id === 'sms' || definition.id === 'push' ? (
          <MessageTemplateDrawer workspace={workspace} defaultChannel={definition.id} buttonContent={t`Continue with ${definition.label_key}`} onClose={close} />
        ) : (
          <OmnichannelTemplateDrawer workspace={workspace} definitions={definitions} defaultChannel={definition.id} buttonContent={t`Continue with ${definition.label_key}`} />
        )}
      </div>}
    </Modal>
  </>
}
