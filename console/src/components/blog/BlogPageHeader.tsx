import { useLingui } from '@lingui/react/macro'
import { ContentCenterTabs } from '../navigation/WorkspaceSectionTabs'
import { WorkspacePageTitle } from '../navigation/WorkspacePageTitle'

const BlogPageHeader: React.FC<{ workspaceId: string }> = ({ workspaceId }) => {
  const { t } = useLingui()
  return <div className="px-6 pt-6">
    <WorkspacePageTitle style={{ marginBottom: 24 }}>{t`Categories`}</WorkspacePageTitle>
    <ContentCenterTabs workspaceId={workspaceId} activeKey="blog" />
  </div>
}

export default BlogPageHeader

