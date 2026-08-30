# 自动化旅程频次与消息频控操作手册

本文用于业务运营、实施和排障人员。三个“频次”解决不同问题，必须分别配置，不能互相替代。

| 能力 | 业务问题 | 作用时点 | 结果 |
| --- | --- | --- | --- |
| 进入频次（Automation frequency） | 同一客户遇到触发事件时是否创建新旅程实例 | 事件匹配后、创建 Journey instance 前 | `once` 每客户只进入一次；`every_time` 每个新事件进入一次 |
| 进入保护（Entry guard） | 即使允许再次进入，是否因冷却期或并发实例数暂缓/拒绝 | 进入频次去重后、创建实例前 | `guard_deferred`、`guard_denied`，不消费消息频控额度 |
| 消息频控（Message frequency cap） | 已进入旅程后，这一条消息是否允许触达 | 创建 Delivery Intent 前 | `allow`、`defer` 或 `suppressed`；不会撤销或阻止 Journey enrollment |

## 1. 进入频次

在“自动化旅程 → 创建/编辑 → 进入频次与保护”中选择：

- **每个联系人一次**：适合开户欢迎、首次认证、生命周期里程碑。同一 Workspace 内以 `automation_id + customer_id` 唯一；后续不同事件返回 `already_once`。
- **每次**：适合交易提醒、风险信号、余额变化。同一事件重放以 `automation_id + customer_id + origin_event_id` 去重并返回 `replayed_event`；新的 `origin_event_id` 创建新实例。

请求中的核心配置示例：

```json
{
  "trigger": {
    "event_kind": "custom_event",
    "custom_event_name": "risk.changed",
    "frequency": "every_time"
  }
}
```

数据库防重边界：

```sql
CREATE UNIQUE INDEX uq_journey_enrollments_once
ON journey_enrollments(automation_id, customer_id)
WHERE frequency = 'once';

CREATE UNIQUE INDEX uq_journey_enrollments_event
ON journey_enrollments(automation_id, customer_id, origin_event_id)
WHERE frequency = 'every_time';
```

这些约束属于最终一致性边界；应用重试、Broker 重复投递和并发 Worker 都不能绕过。

## 2. 进入保护

进入保护是可选的业务保险丝，适用于 `every_time`。可配置：

- 冷却时间：在 UI 中按小时填写；API 中 `cooldown` 使用纳秒，与 Go `time.Duration` JSON 表示一致。
- 最大并发实例数：只统计该客户在该自动化中的活跃实例。

```json
{
  "trigger": {
    "event_kind": "custom_event",
    "custom_event_name": "risk.changed",
    "frequency": "every_time",
    "entry_guard": {
      "enabled": true,
      "cooldown": 3600000000000,
      "max_concurrent": 1
    }
  }
}
```

达到并发上限且配置了冷却期时，系统记录 `guard_deferred` 和 `retry_at`；没有可等待窗口时记录 `guard_denied`。决策保存在 `journey_entry_decisions`，排障时不要只查看旧的 Email 投影表。

## 3. 消息频控

在旅程向导的“消息频控”步骤或频控后台配置。三层策略并行生效：

- `campaign`：单个营销活动的限制；
- `trigger`：单个自动化/时间触发来源的限制；
- `workspace_global`：Workspace 全量触达限制。

策略按渠道独立计算，适用的三层窗口在 Redis 中原子预占，避免只扣掉部分额度。示例：

```json
{
  "workspace_id": "workspace-a",
  "name": "风险提醒每日上限",
  "scope": "trigger",
  "scope_ref": "risk-alert-journey",
  "channel": "sms",
  "max_events": 2,
  "window_kind": "sliding",
  "window_seconds": 86400,
  "deny_action": "suppress",
  "priority": 200,
  "enabled": true
}
```

消息被拒绝时仍保留 Journey instance，并创建状态为 `suppressed` 的 Delivery Intent，`suppression_reason` 指明命中的频控层。Redis 不可用且已配置频控时采取 fail-closed：消息进入 `deferred`，不绕过频控发送。

## 4. 推荐配置

| 场景 | 进入频次 | 进入保护 | 消息频控 |
| --- | --- | --- | --- |
| 新客欢迎 | 每客户一次 | 关闭 | Workspace 全局 Email 日上限 |
| 交易/风险事件提醒 | 每次 | 1 小时冷却、最多 1 个活跃实例 | Trigger SMS 日上限 + Workspace 全局上限 |
| 定期复购召回 | 每次 | 按活动周期冷却 | Campaign 上限 + Workspace 全局上限 |

## 5. 排障顺序

1. 在 Customer 360 的“旅程”页签确认是否创建实例。
2. 打开“旅程轨迹”，查看 `entry_decisions`：`already_once`/`replayed_event` 是正确去重，`guard_*` 是进入保护。
3. 查看轨迹关联的 Delivery：`suppressed` 是业务频控，`deferred` 可能是窗口等待或频控基础设施不可用。
4. 在投递中心按 Customer、Source type=`automation`、Source ID 和状态过滤。
5. 只有 `unknown` 才需要人工核验 Provider；不要把频控抑制当成发送失败重试。

关键追踪链为：`request_id → event_id → journey_instance_id → delivery_id → provider_receipt_id`。日志和截图不得展示完整 Email、Phone 或消息正文。
