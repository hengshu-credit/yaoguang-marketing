# 投递未知态核对手册

## 适用范围

本手册处理 `delivery_intents.status = unknown` 的投递。`unknown` 表示请求可能已经到达供应商，但瑶光没有获得可证明的结果；它不是普通失败，系统不会自动重发。

## 判定原则

1. 先通过投递中心或 `deliveries.get` 读取 intent、attempt、provider message ID、effect key、错误分类和时间。
2. 按 provider message ID 查询供应商；没有 message ID 时，使用 effect key（供应商支持幂等键时）、收件身份提示、模板和提交时间窗口交叉查询。
3. 查询暂时无结果时，调用 `POST /api/deliveries.reconcile` 保持待核对，不得重试。
4. 只有得到供应商侧证据后，才调用 `POST /api/deliveries.resolveUnknown`。操作者和原因会写入审计记录。

## 三种受控处置

| 供应商证据 | action | 结果 |
| --- | --- | --- |
| 明确已接受或已送达 | `mark_confirmed` | 标记已确认，禁止再发 |
| 明确永久拒绝且不应重试 | `mark_terminal_failed` | 标记终态失败 |
| 明确未接受请求，可以安全重试 | `retry_after_verified_not_accepted` | 创建受控重试机会 |

请求示例：

```json
{
  "workspace_id": "workspace-id",
  "intent_id": "delivery-intent-uuid",
  "action": "retry_after_verified_not_accepted",
  "reason": "供应商后台查询确认该请求未被接收，工单号 SUP-20260830-001"
}
```

不得使用直接 SQL 修改 intent、attempt、queue 或 reconciliation 状态。直接修改会绕过状态机、effect key 防重和审计链，并可能造成重复触达。

## 值班流程

1. 查看 `/readyz`；数据库不可用时先恢复依赖，未知态处置暂停。
2. 按 `unknown` 和最早创建时间筛选投递，优先处理高风险或大批量活动。
3. 保存供应商查询依据（工单、查询时间、返回状态），在处置原因中填写可追溯编号。
4. 执行受控 action 后重新读取投递详情，确认 intent、attempt、queue、reconciliation 状态一致。
5. 对同一 provider/错误分类的集中增长建立事故记录，停止相关活动或 provider 集成后再排查。

## 告警建议

- `unknown` 新增量连续 5 分钟大于 0：警告。
- 单 provider 15 分钟 `unknown / submitting` 超过 1% 或绝对值超过 20：严重。
- 最老 pending reconciliation 超过 15 分钟：警告；超过 60 分钟：严重。
- `submitting` attempt 超过 lease 且没有 reconciliation：严重，检查 worker 和账本一致性。
- 重复 effect key 查询结果非零：立即停止相关 dispatch。

阈值应按渠道 SLA 调整，但不得通过把 `unknown` 改成 `failed` 来消除告警。

## 数据保留与清理

- intent、attempt、receipt、reconciliation 与操作人/原因应按金融业务审计周期保留，默认不少于 24 个月。
- 成功队列行也保留到相同审计周期，不立即删除。
- 清理必须先归档完整关联链，并按 Workspace 分区/范围执行；不得只删除 queue 而留下不可解释的 intent。
- Identity 仅保存必要的引用或脱敏提示，导出和工单中不得复制敏感明文。

## 核查 SQL（只读）

```sql
SELECT effect_key, COUNT(*)
FROM delivery_intents
GROUP BY effect_key
HAVING COUNT(*) > 1;

SELECT provider, status, COUNT(*)
FROM delivery_attempts
WHERE created_at >= NOW() - INTERVAL '15 minutes'
GROUP BY provider, status;

SELECT MIN(created_at) AS oldest_pending, COUNT(*) AS backlog
FROM delivery_reconciliations
WHERE status = 'pending';
```

