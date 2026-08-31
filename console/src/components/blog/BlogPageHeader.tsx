import { useLingui } from '@lingui/react/macro'
import { ContentCenterTabs } from '../navigation/WorkspaceSectionTabs'

const BlogPageHeader: React.FC<{ workspaceId: string }> = ({ workspaceId }) => {
  const { t } = useLingui()
  return <div className="px-6 pt-6">
    <h1 className="mb-6 text-2xl font-medium">{t`Categories`}</h1>
    <ContentCenterTabs workspaceId={workspaceId} activeKey="blog" />
  </div>
}

export default BlogPageHeader

