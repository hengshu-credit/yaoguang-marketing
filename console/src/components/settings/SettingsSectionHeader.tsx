import { Divider } from 'antd'
import { WorkspacePageTitle } from '../navigation/WorkspacePageTitle'

interface SettingsSectionHeaderProps {
  title: string
  description: string
  /** Page sections get the shared h1; nested settings groups retain their local heading. */
  primary?: boolean
  /** Extra classes for the trailing divider, e.g. `!mb-4` to tighten the gap. */
  className?: string
}

export function SettingsSectionHeader({
  title,
  description,
  primary = true,
  className
}: SettingsSectionHeaderProps) {
  return (
    <>
      {primary ? (
        <WorkspacePageTitle style={{ marginBottom: 8 }}>{title}</WorkspacePageTitle>
      ) : (
        <div className="mb-2 text-2xl font-medium">{title}</div>
      )}
      <div className="text-gray-500">{description}</div>

      <Divider className={className ? `mb-12 ${className}` : 'mb-12'} />
    </>
  )
}
