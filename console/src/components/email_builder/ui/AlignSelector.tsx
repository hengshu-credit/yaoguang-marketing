import React from 'react'
import { Radio } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faAlignLeft, faAlignCenter, faAlignRight } from '@fortawesome/free-solid-svg-icons'

interface AlignSelectorProps {
  value?: string
  onChange: (value: string | undefined) => void
  allowToggle?: boolean
  disabled?: boolean
  options?: Array<{
    label: React.ReactNode
    value: string
  }>
}

const AlignSelector: React.FC<AlignSelectorProps> = ({
  value,
  onChange,
  allowToggle = false,
  disabled = false,
  options = [
    {
      label: <FontAwesomeIcon icon={faAlignLeft} className="opacity-70" />,
      value: 'left'
    },
    {
      label: <FontAwesomeIcon icon={faAlignCenter} className="opacity-70" />,
      value: 'center'
    },
    {
      label: <FontAwesomeIcon icon={faAlignRight} className="opacity-70" />,
      value: 'right'
    }
  ]
}) => {
  const handleClick = (clickedValue: string) => {
    if (disabled) return
    if (allowToggle && value === clickedValue) {
      onChange(undefined)
    } else {
      onChange(clickedValue)
    }
  }

  return (
    <Radio.Group
      size="small"
      value={value || (allowToggle ? undefined : 'left')}
      onChange={(e) => {
        if (!allowToggle && !disabled) {
          onChange(e.target.value)
        }
      }}
      optionType="button"
      buttonStyle="solid"
      disabled={disabled}
    >
      {options.map((option) => (
        <Radio.Button
          key={option.value}
          value={option.value}
          onClick={() => allowToggle && handleClick(option.value)}
          disabled={disabled}
        >
          {option.label}
        </Radio.Button>
      ))}
    </Radio.Group>
  )
}

export default AlignSelector
