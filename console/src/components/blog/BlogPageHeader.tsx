import { useLingui } from '@lingui/react/macro'
import { ContentCenterTabs } from '../navigation/WorkspaceSectionTabs'
import { WorkspacePageTitle } from '../navigation/WorkspacePageTitle'

const BlogPageHeader: React.FC<{ workspaceId: string }> = ({ workspaceId }) => {
  const { t } = useLingui()
  return <div>
    <div className="flex justify-between items-center mb-6">
      <WorkspacePageTitle>{t`Blog Content`}</WorkspacePageTitle>
    </div>
    <ContentCenterTabs workspaceId={workspaceId} activeKey="blog" />
  </div>
}

export default BlogPageHeader

