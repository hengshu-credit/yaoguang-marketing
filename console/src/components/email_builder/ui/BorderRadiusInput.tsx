import React, { memo } from 'react'
import { InputNumber } from 'antd'

interface BorderRadiusInputProps {
  value?: string
  onChange?: (value: string | undefined) => void
  placeholder?: string
  defaultValue?: string
  min?: number
  max?: number
  step?: number
  disabled?: boolean
  size?: 'small' | 'middle' | 'large'
  style?: React.CSSProperties
}

const BorderRadiusInput: React.FC<BorderRadiusInputProps> = memo(
  ({
    value,
    onChange,
    placeholder,
    defaultValue,
    min = 0,
    max = 100,
    step = 1,
    disabled = false,
    size = 'small',
    style = { width: '100px' }
  }) => {
    /**
     * Parse border radius to get numeric value
     */
    const parseBorderRadiusNumber = (borderRadius?: string): number | undefined => {
      if (!borderRadius) return undefined
      const match = borderRadius.match(/^(\d+(?:\.\d+)?)px?$/)
      return match ? parseFloat(match[1]) : undefined
    }

    const handleChange = (numValue: number | null) => {
      const formattedValue =
        numValue !== null && numValue !== undefined ? `${numValue}px` : undefined
      onChange?.(formattedValue)
    }

    const parsedValue = parseBorderRadiusNumber(value)
    const parsedPlaceholder = placeholder || (parseBorderRadiusNumber(defaultValue) || 0).toString()

    return (
      <InputNumber
        size={size}
        value={parsedValue}
        onChange={handleChange}
        placeholder={parsedPlaceholder}
        min={min}
        max={max}
        step={step}
        suffix="px"
        disabled={disabled}
        style={style}
      />
    )
  }
)

export default BorderRadiusInput
