import { useQuery } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { Button, Collapse, Descriptions, Empty, Skeleton, Space, Tabs, Tag, Typography } from 'antd'
import { businessActivityApi } from '../../services/api/businessActivity'
import { ActionableError } from '../errors/ActionableError'

export function CustomerBusinessActivity({workspaceId,customerId,active=true}: {workspaceId:string;customerId:string;active?:boolean}) {
  const {t,i18n} = useLingui()
  const events = useQuery({queryKey:['customer-360',workspaceId,customerId,'business-events'],queryFn:()=>businessActivityApi.events(workspaceId,customerId),enabled:active})
  const objects = useQuery({queryKey:['customer-360',workspaceId,customerId,'business-objects'],queryFn:()=>businessActivityApi.objects(workspaceId,customerId),enabled:active})
  const date = (value:string) => new Date(value).toLocaleString(i18n.locale)
  const status = (value:string) => ({accepted:t`Accepted`,expired:t`Expired`,backfill:t`Historical import`,pending_identity:t`Waiting for customer identity`,associated_history:t`Linked historical event`}[value] ?? t`Unknown status`)
  const eventContent = events.isError ? <ActionableError error={events.error} onRetry={()=>void events.refetch()} retrying={events.isFetching}/> : events.isLoading ? <Skeleton active/> : !events.data?.events.length ? <Empty description={t`No business events yet`}/> : <Collapse items={events.data.events.map(row=>({key:row.event_id,label:<Space wrap><Typography.Text strong>{row.event?.event_name ?? row.event?.event_code}</Typography.Text><Tag>{status(row.processing_status)}</Tag><Typography.Text type="secondary">{date(row.event?.triggered_at ?? row.reported_at)}</Typography.Text></Space>,children:<><Descriptions size="small" column={1} items={[
    {key:'report',label:t`Report ID`,children:row.report_id},
    {key:'code',label:t`Event code`,children:row.event?.event_code},
    {key:'triggered',label:t`Triggered at`,children:row.event && date(row.event.triggered_at)},
    {key:'reported',label:t`Reported at`,children:date(row.reported_at)},
    {key:'expires',label:t`Expires at`,children:row.event?.expires_at ? date(row.event.expires_at) : t`Never expires`},
    {key:'revision',label:t`Definition revision`,children:row.event?.definition_revision},
    {key:'mapping',label:t`Mapping revision`,children:row.event?.mapping_revision},
  ]}/><Typography.Title level={5}>{t`Customer attributes`}</Typography.Title><pre style={{whiteSpace:'pre-wrap',overflowWrap:'anywhere'}}>{JSON.stringify(row.event?.customer_attributes ?? {},null,2)}</pre><Typography.Title level={5}>{t`Event data`}</Typography.Title><pre style={{whiteSpace:'pre-wrap',overflowWrap:'anywhere'}}>{JSON.stringify(row.event?.data ?? {},null,2)}</pre></>}))}/>
  const objectContent = objects.isError ? <ActionableError error={objects.error} onRetry={()=>void objects.refetch()} retrying={objects.isFetching}/> : objects.isLoading ? <Skeleton active/> : !objects.data?.objects.length ? <Empty description={t`No business objects yet`}/> : <Collapse items={objects.data.objects.map(row=>({key:row.id,label:<Space><Tag>{row.object_type}</Tag><Typography.Text>{row.external_key}</Typography.Text></Space>,children:<><Descriptions size="small" column={1} items={[{key:'source',label:t`Source`,children:row.source},{key:'updated',label:t`Updated at`,children:date(row.updated_at)},{key:'revision',label:t`Definition revision`,children:row.definition_revision}]}/><pre style={{whiteSpace:'pre-wrap',overflowWrap:'anywhere'}}>{JSON.stringify(row.attributes,null,2)}</pre></>}))}/>
  return <Space orientation="vertical" style={{width:'100%'}}><Space><Typography.Text type="secondary">{t`Showing the latest 100 records. Every repeated occurrence is retained.`}</Typography.Text><Button loading={events.isFetching||objects.isFetching} onClick={()=>{void events.refetch();void objects.refetch()}}>{t`Refresh`}</Button></Space><Tabs items={[{key:'events',label:t`Business events`,children:eventContent},{key:'objects',label:t`Business objects`,children:objectContent}]}/></Space>
}
