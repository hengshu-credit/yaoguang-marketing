import { Skeleton, Tabs } from 'antd'
import type { TabsProps } from 'antd'
import { Link } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { useWorkspacePermissions } from '../../contexts/AuthContext'

export type ContentCenterTabKey = 'templates' | 'blog' | 'file-manager'
export type DataAnalyticsTabKey =
  | 'marketing'
  | 'dashboard'
  | 'live'
  | 'explore'
  | 'goals'
  | 'filters'
  | 'annotations'

interface SectionTabsProps {
  activeKey: string
  ariaLabel: string
  items: TabsProps['items']
  loading: boolean
}

function SectionTabs({ activeKey, ariaLabel, items, loading }: SectionTabsProps) {
  if (loading) {
    return (
      <nav aria-label={ariaLabel} aria-busy="true" className="min-h-12 overflow-hidden">
        <Skeleton.Input active size="small" style={{ width: 360 }} />
      </nav>
    )
  }

  return (
    <nav aria-label={ariaLabel} className="max-w-full overflow-hidden">
      <Tabs
        activeKey={activeKey}
        items={items}
        animated={false}
        tabBarStyle={{ marginBottom: 20 }}
      />
    </nav>
  )
}

const canRead = (permission: { read: boolean; write: boolean } | undefined) =>
  Boolean(permission?.read || permission?.write)

export function ContentCenterTabs(props: {
  workspaceId: string
  activeKey: ContentCenterTabKey
}) {
  const { t } = useLingui()
  const { permissions, loading } = useWorkspacePermissions(props.workspaceId)
  const items: TabsProps['items'] = []

  if (canRead(permissions?.templates)) {
    items.push({
      key: 'templates',
      label: (
        <Link to="/console/workspace/$workspaceId/templates" params={{ workspaceId: props.workspaceId }}>
          {t`Template Management`}
        </Link>
      )
    })
  }

  if (canRead(permissions?.workspace)) {
    items.push(
      {
        key: 'blog',
        label: (
          <Link to="/console/workspace/$workspaceId/blog" params={{ workspaceId: props.workspaceId }}>
            {t`Blog Content`}
          </Link>
        )
      },
      {
        key: 'file-manager',
        label: (
          <Link
            to="/console/workspace/$workspaceId/file-manager"
            params={{ workspaceId: props.workspaceId }}
          >
            {t`File Manager`}
          </Link>
        )
      }
    )
  }

  return (
    <SectionTabs
      activeKey={props.activeKey}
      ariaLabel={t`Content Center`}
      items={items}
      loading={loading}
    />
  )
}

export function DataAnalyticsTabs(props: {
  workspaceId: string
  activeKey: DataAnalyticsTabKey
}) {
  const { t } = useLingui()
  const { permissions, loading } = useWorkspacePermissions(props.workspaceId)
  const items: TabsProps['items'] = []

  if (canRead(permissions?.message_history)) {
    items.push({
      key: 'marketing',
      label: (
        <Link to="/console/workspace/$workspaceId/analytics" params={{ workspaceId: props.workspaceId }}>
          {t`Marketing Overview`}
        </Link>
      )
    })
  }

  if (canRead(permissions?.web_analytics)) {
    items.push(
      {
        key: 'dashboard',
        label: (
          <Link
            to="/console/workspace/$workspaceId/web-analytics/$tab"
            params={{ workspaceId: props.workspaceId, tab: 'dashboard' }}
          >
            {t`Website Overview`}
          </Link>
        )
      },
      {
        key: 'live',
        label: (
          <Link
            to="/console/workspace/$workspaceId/web-analytics/live"
            params={{ workspaceId: props.workspaceId }}
          >
            {t`Live Visitors`}
          </Link>
        )
      },
      {
        key: 'explore',
        label: (
          <Link
            to="/console/workspace/$workspaceId/web-analytics/$tab"
            params={{ workspaceId: props.workspaceId, tab: 'explore' }}
          >
            {t`Multidimensional Analysis`}
          </Link>
        )
      },
      {
        key: 'goals',
        label: (
          <Link
            to="/console/workspace/$workspaceId/web-analytics/$tab"
            params={{ workspaceId: props.workspaceId, tab: 'goals' }}
          >
            {t`Conversion Goals`}
          </Link>
        )
      },
      {
        key: 'filters',
        label: (
          <Link
            to="/console/workspace/$workspaceId/web-analytics/$tab"
            params={{ workspaceId: props.workspaceId, tab: 'filters' }}
          >
            {t`Attribution Rules`}
          </Link>
        )
      },
      {
        key: 'annotations',
        label: (
          <Link
            to="/console/workspace/$workspaceId/web-analytics/$tab"
            params={{ workspaceId: props.workspaceId, tab: 'annotations' }}
          >
            {t`Analytics Annotations`}
          </Link>
        )
      }
    )
  }

  return (
    <SectionTabs
      activeKey={props.activeKey}
      ariaLabel={t`Data Analytics`}
      items={items}
      loading={loading}
    />
  )
}
