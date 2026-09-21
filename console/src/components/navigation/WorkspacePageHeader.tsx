import type { ReactNode } from 'react'
import { WorkspacePageTitle } from './WorkspacePageTitle'

export function WorkspacePageHeader({ title,description,actions,testId }: {title: ReactNode;description?: ReactNode;actions?: ReactNode;testId?:string}) {
  return <header className="workspace-page-header" data-testid={testId}>
    <div className="workspace-page-heading"><WorkspacePageTitle>{title}</WorkspacePageTitle>{description && <div className="workspace-page-description">{description}</div>}</div>
    {actions && <div className="workspace-page-actions">{actions}</div>}
  </header>
}
