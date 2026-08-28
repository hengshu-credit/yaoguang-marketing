import React from 'react'
import { Tooltip } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faQuestionCircle } from '@fortawesome/free-solid-svg-icons'

interface InputLayoutProps {
  label: React.ReactNode | string
  help?: string
  layout?: 'horizontal' | 'vertical'
  children: React.ReactNode
  className?: string
}

const InputLayout: React.FC<InputLayoutProps> = ({
  label,
  help,
  children,
  className = 'mt-4',
  layout = 'horizontal'
}) => {
  const flexDirection = layout === 'vertical' ? 'flex-col' : ''
  const alignmentClass = 'items-start'

  return (
    <div className={`flex ${flexDirection} ${alignmentClass} gap-2 ${className}`}>
      <div className="flex items-start gap-1 min-w-[150px]">
        {typeof label === 'string' ? (
          <label className="text-xs font-medium text-slate-700 pt-1">{label}:</label>
        ) : (
          label
        )}
        {help && (
          <Tooltip title={help}>
            <FontAwesomeIcon
              icon={faQuestionCircle}
              className="text-xs text-gray-400 opacity-70 cursor-help"
            />
          </Tooltip>
        )}
      </div>
      <div className={layout === 'vertical' ? 'w-full' : 'flex-1'}>{children}</div>
    </div>
  )
}

export default InputLayout
