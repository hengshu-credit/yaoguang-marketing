import { Divider } from 'antd'
import { WorkspacePageHeader } from '../navigation/WorkspacePageHeader'

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
        <WorkspacePageHeader title={title} description={description} />
      ) : (
        <div className="mb-2 text-2xl font-medium">{title}</div>
      )}
      {!primary && <div className="workspace-page-description">{description}</div>}

      <Divider className={className} style={{ margin: '0 0 20px' }} />
    </>
  )
}
