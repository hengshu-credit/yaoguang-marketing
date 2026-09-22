import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { Form, Input, Select } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faEnvelope } from '@fortawesome/free-solid-svg-icons'
import type { MJMLComponentType } from '../types'
import { BaseEmailBlock, type OnUpdateAttributesFunction, type PreviewProps } from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import PanelLayout from '../panels/PanelLayout'

// Functional component for settings panel with i18n support
const MjmlSettingsPanel: React.FC<{ attributes: Record<string, unknown>; onUpdate: OnUpdateAttributesFunction }> = ({ attributes, onUpdate }) => {
  const { t } = useLingui()

  return <PanelLayout title={t`Email Attributes`}>
    <Form layout="vertical">
      <Form.Item label={t`Document language`} extra={t`Used for accessibility and language metadata in the compiled email.`}>
        <Input value={String(attributes.lang ?? '')} placeholder="en" onChange={(event) => onUpdate({ lang: event.target.value.trim() || undefined })} />
      </Form.Item>
      <Form.Item label={t`Text direction`}>
        <Select
          className="w-full"
          value={String(attributes.dir ?? 'ltr')}
          options={[{ value: 'ltr', label: t`Left to right` }, { value: 'rtl', label: t`Right to left` }]}
          onChange={(value) => onUpdate({ dir: value })}
        />
      </Form.Item>
    </Form>
  </PanelLayout>
}

/**
 * Implementation for mjml root blocks
 */
export class MjmlBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return <FontAwesomeIcon icon={faEnvelope} className="opacity-70" />
  }

  getLabel(): string {
    return 'Email'
  }

  getDescription(): React.ReactNode {
    return 'Root container for the entire email document'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mjml'] || {}
  }

  canHaveChildren(): boolean {
    return true
  }

  getValidChildTypes(): MJMLComponentType[] {
    return ['mj-head', 'mj-body']
  }

  /**
   * Render the settings panel for the mjml block
   */
  renderSettingsPanel(onUpdate: OnUpdateAttributesFunction): React.ReactNode {
    return <MjmlSettingsPanel attributes={(this.block.attributes ?? {}) as Record<string, unknown>} onUpdate={onUpdate} />
  }

  getEdit(props: PreviewProps): React.ReactNode {
    const {
      selectedBlockId,
      onSelectBlock,
      onUpdateBlock,
      attributeDefaults,
      emailTree,
      onCloneBlock,
      onDeleteBlock,
      onSaveBlock: onSave,
      savedBlocks
    } = props

    const key = this.block.id
    const isSelected = selectedBlockId === this.block.id
    const blockClasses = `email-block-hover ${isSelected ? 'selected' : ''}`.trim()

    const selectionStyle: React.CSSProperties = isSelected
      ? { position: 'relative', zIndex: 10 }
      : {}

    const handleClick = (e: React.MouseEvent) => {
      e.stopPropagation()
      if (onSelectBlock) {
        onSelectBlock(this.block.id)
      }
    }

    const attrs = EmailBlockClass.mergeWithAllDefaults(
      'mjml',
      this.block.attributes as Record<string, unknown>,
      attributeDefaults
    )

    // Pass the current block as emailTree since this is the root
    const currentEmailTree = emailTree || this.block

    return (
      <div
        key={key}
        style={{
          fontFamily: 'Arial, sans-serif',
          direction: attrs.direction as 'ltr' | 'rtl' | undefined,
          ...selectionStyle
        }}
        className={blockClasses}
        onClick={handleClick}
        lang={attrs.lang}
      >
        {this.block.children?.map((child) => (
          <React.Fragment key={child.id}>
            {EmailBlockClass.renderEmailBlock(
              child,
              attributeDefaults,
              selectedBlockId,
              onSelectBlock,
              currentEmailTree,
              onUpdateBlock,
              onCloneBlock,
              onDeleteBlock,
              onSave,
              savedBlocks
            )}
          </React.Fragment>
        ))}
      </div>
    )
  }
}
