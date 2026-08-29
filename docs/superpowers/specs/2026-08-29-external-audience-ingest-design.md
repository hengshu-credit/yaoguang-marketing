# Notifuse 外部受众与事件接入设计

## 目标

为业务系统提供一个适合高并发重试的批量写入入口，覆盖：

- 创建或增量更新联系人主档；
- 更新业务生命周期状态和任意业务属性；
- 增加、移除或全量替换联系人标签；
- 更新联系人在名单中的营销同意状态；
- 上报带稳定外部 ID 的业务事件，并立即进入实时自动化链路。

## 数据边界

- `contacts` 保留邮件投递所需的规范化主档字段。
- `contact_profiles` 保存外部系统拥有的 `status` 和 `attributes`。业务状态不复用名单状态，避免把“冻结用户”误当成“退订用户”。
- `contact_tags` 保存可索引的标签关系；写入使用集合语义，重复请求不产生重复关系。
- `contact_lists.status` 继续作为订阅、退订、退信和投诉的唯一权威。
- `custom_events(event_name, external_id)` 继续作为外部事件幂等键。

画像和标签变化由数据库触发器写入 `contact_timeline`。v40 的固定桥接器随后在同一事务写入事件账本和 outbox，因此响应成功表示实时链路的权威输入已经持久化，而不表示 RabbitMQ 或下游渠道已经处理完成。

## HTTP 契约

`POST /api/ingest.batch` 接受一个工作区和最多 500 条记录：

```json
{
  "workspace_id": "acme",
  "contacts": [{
    "id": "crm-user-42",
    "contact": {"email": "user@example.com", "first_name": "Lin"},
    "status": "active",
    "attributes": {"plan": "pro", "score": 91},
    "tags": {"operation": "set", "values": ["paid", "beta"]},
    "list_memberships": [{"list_id": "product", "status": "active"}]
  }],
  "events": [{
    "id": "checkout-889",
    "email": "user@example.com",
    "event_name": "checkout.completed",
    "external_id": "checkout-889",
    "occurred_at": "2026-08-29T05:00:00Z",
    "properties": {"amount": 299}
  }]
}
```

响应保持输入顺序并逐项返回 `accepted` 或 `error`。联系人和标签写入按最终状态收敛，事件以 `(event_name, external_id)` 严格幂等；调用方可以安全重试失败项。API key 至少需要 Contacts write；只要请求包含名单变更，还需要 Lists write。

读取联系人时，扩展画像统一出现在 `contact.profile`：`status`、`attributes` 和 `tags` 会被批量加载，因此联系人列表、广播和自动化不会产生逐联系人查询。Liquid 模板可直接使用 `contact.profile.status`、`contact.profile.attributes.plan` 和 `contact.profile.tags`。

分群条件使用三个固定白名单字段：

- `profile_status`：字符串状态；
- `profile_tags`：标签成员关系，编译为可命中 `(email, tag)` 主键的 `EXISTS`；
- `profile_attributes`：支持 JSON path 和字符串、数值、日期类型比较。

## 限流和故障语义

- 单请求最多 500 条记录，请求体最多 4 MiB。
- 每个 API 实例只允许固定数量的批量请求同时进入数据库；满载时返回 `429` 和 `Retry-After`，不在进程内无限排队。
- 无效记录不会阻止同批次中的其他记录。
- 数据库级批量失败只标记受影响的项；客户端重试不会重复标签或同时间版本的事件。
- RabbitMQ 不可用时，事务仍提交到 outbox；relay 恢复后继续发送。

## 后续扩展

- 以同一契约生成 TypeScript、Java、Go SDK 和 OpenAPI 文档。
- 对超大历史导入增加异步对象存储作业；实时接口继续保持小批、低延迟语义。
