# Yaoguang B1 Delivery Consistency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立 Email、SMS、Push、WhatsApp、Telegram、In-App、Webhook 可复用的统一投递账本，保证营销活动断点恢复不重复创建逻辑投递，并明确处理“供应商可能已接受但本地未收到响应”的不确定状态。

**Architecture:** 复用现有 `channel_sends` effect-key 账本、Realtime side-effect ledger、Webhook lease/retry 模式；新增通用 `delivery_intents/delivery_attempts/delivery_reconciliations`，把 `email_queue` 变为可审计的执行队列。营销活动先以稳定 effect key 原子预留 intent，再入队；Worker 原子 claim，提交供应商前进入 `submitting`，确认后写 `provider_accepted/confirmed`，超时或断连进入 `unknown`，禁止无条件自动重发。

**Tech Stack:** Go 1.25、PostgreSQL transactional outbox/`FOR UPDATE SKIP LOCKED`、Redis、RabbitMQ、Provider APIs、OpenCensus/Prometheus、React 18、Vitest、集成故障注入测试。

**Spec:** `docs/superpowers/specs/2026-08-30-yaoguang-domain-first-rearchitecture-design.md` 的 B1、Delivery Intent/Attempt/Receipt/Reconciliation、effect key 和失败语义章节。

## Global Constraints

- 稳定 effect key 为 `workspace + source_type + source_id + source_version + customer_id + node_or_phase + occurrence + variant` 的规范串经 SHA-256 编码；同一逻辑投递永远得到同一 key。
- 状态机只允许：`planned → reserved → queued → submitting → provider_accepted → confirmed`，以及受控进入 `suppressed/deferred/transient_failed/terminal_failed/unknown/cancelled`。
- Provider 支持幂等键时必须传 effect key；不支持时用 provider message ID 和 reconciliation 防止盲目重发。
- `unknown` 不是 `failed`；没有明确“供应商未接受”证据不得自动重新创建 attempt。
- 成功 Email 队列行不再立即删除；保留投递意图、attempt、receipt 和审计状态。
- 营销活动进度以 Delivery Intent 聚合为准，“已入队”不得显示为“已发送”。
- 不修改 V47；B1 使用 V48，并覆盖全新 Workspace 初始化。
- 每个任务先写失败测试，再实现，再运行故障路径验证。

---

### Task 1: 定义统一投递领域模型与 V48 schema

**Files:**

- Create: `internal/domain/delivery.go`
- Create: `internal/domain/delivery_test.go`
- Create: `internal/database/schema/delivery_tables.go`
- Create: `internal/database/schema/delivery_tables_test.go`
- Create: `internal/migrations/v48.go`
- Create: `internal/migrations/v48_test.go`
- Modify: `internal/database/init.go`
- Modify: `config/config.go`
- Modify: `internal/migrations/v47_test.go`

- [ ] **Step 1: 写状态机和 effect key 失败测试**

测试同一输入 key 稳定、任一维度变化 key 变化、Unicode/空白规范化一致；测试合法状态迁移和非法回退被拒绝。`occurrence` 必须是调用方确定的业务序号或 event ID，禁止使用当前时间和随机数。

- [ ] **Step 2: 运行 domain 测试确认失败**

```powershell
go test ./internal/domain -run 'Delivery|EffectKey' -count=1
```

- [ ] **Step 3: 实现领域类型**

定义 `DeliveryIntent`、`DeliveryAttempt`、`DeliveryReceiptLink`、`DeliveryReconciliation`、`DeliveryStatus`、`DeliverySource`、`DeliveryRepository`。状态转换通过 `CanTransitionTo` 和 service command 校验，不允许 Repository 任意字符串更新。

- [ ] **Step 4: 写并实现 schema contract**

`delivery_intents.effect_key` Workspace DB 内唯一；保存 source/version/customer/channel/template/variant/occurrence/status/suppression reason。`delivery_attempts` 按 intent + attempt_no 唯一，保存 provider、request hash、provider id、lease token/timestamps/error category。`delivery_reconciliations` 保存查询结果和人工处置。`email_queue` 增加 `delivery_intent_id`、`claim_token`、`lease_expires_at`、`completed_at`，并建立 claim 索引。

- [ ] **Step 5: 实现 V48 和 fresh-install 接线**

迁移现有 `channel_sends` 与尚存 `email_queue` 行到 delivery intents；无法补 customer_id 的历史行保留 Email 展示值并标记 `legacy_identity` metadata。将代码版本提升到 48.0，更新当前版本断言。

- [ ] **Step 6: 运行验证**

```powershell
go test ./internal/domain ./internal/database/schema ./internal/migrations -run 'Delivery|V48|RegisteredMigrations' -count=1
```

- [ ] **Step 7: 提交 Task 1**

```powershell
git add internal/domain/delivery.go internal/domain/delivery_test.go internal/database/schema/delivery_tables.go internal/database/schema/delivery_tables_test.go internal/migrations/v48.go internal/migrations/v48_test.go internal/database/init.go config/config.go internal/migrations/v47_test.go
git commit -m "feat(delivery): add v48 unified delivery ledger"
```

### Task 2: 实现投递预留、原子入队与原子 claim

**Files:**

- Create: `internal/repository/delivery_postgres.go`
- Create: `internal/repository/delivery_postgres_test.go`
- Modify: `internal/domain/email_queue.go`
- Modify: `internal/domain/email_queue_test.go`
- Modify: `internal/repository/email_queue_postgres.go`
- Modify: `internal/repository/email_queue_postgres_test.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: 写幂等预留和事务失败测试**

覆盖：相同 effect key + 相同 request hash 返回已有 intent；相同 key + 不同 hash 返回 conflict；intent insert 成功而 queue insert 失败时整个事务回滚；两个并发 claim 不能获得同一 entry。

- [ ] **Step 2: 运行 repository 测试确认失败**

```powershell
go test ./internal/repository -run 'DeliveryReserve|EnqueueIntent|ClaimPending' -count=1
```

- [ ] **Step 3: 实现 `ReserveAndEnqueueTx`**

一个 Workspace 事务完成 intent 幂等 insert、queue insert 和 intent `queued` 状态变更。冲突读取现有行并比较 canonical request hash。Repository 接口返回 `{Intent, Created, QueueCreated}`，调用方不得用随机 queue ID 推断重复。

- [ ] **Step 4: 将 Fetch + Mark 合并为原子 claim**

用单条 CTE：`SELECT ... FOR UPDATE SKIP LOCKED LIMIT n` 后 `UPDATE ... SET status='processing', claim_token=?, lease_expires_at=? RETURNING ...`。所有后续更新必须带 `id + claim_token` 乐观条件；旧 `FetchPending/MarkAsProcessing` 从接口和调用点移除。

- [ ] **Step 5: 实现 lease 恢复规则**

只有 `reserved/queued/transient_failed` 或 processing lease 过期且对应 attempt 明确未提交供应商时才可再次 claim；`submitting/provider_accepted/unknown` 不进入普通 retry 查询。

- [ ] **Step 6: 运行 repository 全套测试**

```powershell
go test ./internal/repository -run 'Delivery|EmailQueue' -count=1
```

- [ ] **Step 7: 提交 Task 2**

```powershell
git add internal/repository/delivery_postgres.go internal/repository/delivery_postgres_test.go internal/domain/email_queue.go internal/domain/email_queue_test.go internal/repository/email_queue_postgres.go internal/repository/email_queue_postgres_test.go internal/app/app.go
git commit -m "feat(delivery): reserve enqueue and claim atomically"
```

### Task 3: 让营销活动 recipient checkpoint 与投递预留同事务

**Files:**

- Modify: `internal/service/broadcast/queue_message_sender.go`
- Modify: `internal/service/broadcast/queue_message_sender_test.go`
- Modify: `internal/service/broadcast/orchestrator.go`
- Modify: `internal/service/broadcast/orchestrator_test.go`
- Modify: `internal/repository/broadcast_postgres.go`
- Modify: `internal/repository/broadcast_postgres_test.go`
- Modify: `internal/domain/broadcast.go`

- [ ] **Step 1: 写 crash-window 失败测试**

模拟以下断点：预留 intent 后进程崩溃、入队后 checkpoint 前崩溃、checkpoint 保存失败、同一 cursor 重跑。每种情况最终必须只有一个 Delivery Intent 和一个有效 queue entry，且 recipient checkpoint 可从 ledger 重建。

- [ ] **Step 2: 运行测试证明当前随机 ID + 分离 save 会重复**

```powershell
go test ./internal/service/broadcast ./internal/repository -run 'Crash|Checkpoint|EffectKey|IdempotentRecipient' -count=1
```

- [ ] **Step 3: 以 snapshot recipient 生成稳定 effect key**

Broadcast source version 取冻结的 campaign/broadcast version，customer ID 取 B0 authority UUID，phase 取 `primary/test/winner`，occurrence 取 snapshot recipient ordinal，variant 取冻结 variant。删除发送路径中的随机幂等 ID。

- [ ] **Step 4: 实现 Repository 原子操作**

新增 `ReserveRecipientDeliveryTx`：锁定当前 task/snapshot cursor，预留 intent、写 queue、标记 recipient `reserved`、推进已预留计数。task cursor 只作性能 checkpoint，Delivery Intent 唯一约束为正确性边界。

- [ ] **Step 5: 修改 orchestrator 恢复逻辑**

启动/重试时从第一个未有 intent 的 snapshot recipient 继续；checkpoint save 错误必须返回并停止当前批次，禁止仅记录日志后继续。

- [ ] **Step 6: 运行 Broadcast 测试**

```powershell
go test ./internal/service/broadcast ./internal/repository -run 'Broadcast|Recipient|Checkpoint|Delivery' -count=1
```

- [ ] **Step 7: 提交 Task 3**

```powershell
git add internal/service/broadcast/queue_message_sender.go internal/service/broadcast/queue_message_sender_test.go internal/service/broadcast/orchestrator.go internal/service/broadcast/orchestrator_test.go internal/repository/broadcast_postgres.go internal/repository/broadcast_postgres_test.go internal/domain/broadcast.go
git commit -m "fix(broadcast): make recipient reservation crash safe"
```

### Task 4: 将 Email Worker 改为持久状态机

**Files:**

- Modify: `internal/service/queue/worker.go`
- Modify: `internal/service/queue/worker_test.go`
- Modify: `internal/domain/email_provider.go`
- Modify: `internal/domain/email_provider_test.go`
- Modify: `internal/repository/message_history_postgre.go`
- Modify: `internal/repository/message_history_postgre_test.go`
- Modify: `internal/repository/delivery_postgres.go`
- Modify: `internal/repository/delivery_postgres_test.go`

- [ ] **Step 1: 写供应商边界失败测试**

覆盖：请求前 DB 更新失败则不调用 provider；provider 明确拒绝进入 transient/terminal failed；provider 返回成功后本地 confirm 失败进入 recoverable provider_accepted；网络断开且结果未知进入 unknown；Worker 重启不重发 provider_accepted/unknown。

- [ ] **Step 2: 运行 worker 测试确认失败**

```powershell
go test ./internal/service/queue -run 'Delivery|Unknown|ProviderAccepted|Lease' -count=1
```

- [ ] **Step 3: 扩展 Provider request 幂等 contract**

`SendEmailProviderRequest` 增加 `IdempotencyKey`，各 provider adapter 在原生支持时传递；统一返回 `ProviderSubmissionResult{Accepted, ProviderMessageID, DefinitiveFailure, RetryAfter}`。不支持幂等的 provider 明确声明 capability。

- [ ] **Step 4: 实现 attempt 状态推进**

Worker claim 后创建 attempt；事务提交 `submitting` 后才调用外部 provider。明确接受后先持久化 provider ID/accepted，再在同一事务写 message history 和 confirmed。成功不删除 queue row，而是写 `completed_at/status=confirmed`。

- [ ] **Step 5: 实现 unknown 安全处理**

超时、连接重置或接受后本地持久化错误记录为 unknown/provider_accepted，释放 worker lease但不进入 retry queue；发出 reconciliation job 和告警。

- [ ] **Step 6: 运行 worker 与 provider 测试**

```powershell
go test ./internal/service/queue ./internal/service ./internal/repository -run 'EmailQueue|Delivery|Provider|MessageHistory' -count=1
```

- [ ] **Step 7: 提交 Task 4**

```powershell
git add internal/service/queue/worker.go internal/service/queue/worker_test.go internal/domain/email_provider.go internal/domain/email_provider_test.go internal/repository/message_history_postgre.go internal/repository/message_history_postgre_test.go internal/repository/delivery_postgres.go internal/repository/delivery_postgres_test.go
git commit -m "feat(delivery): persist email submission state machine"
```

### Task 5: 接入 Receipt、Reconciliation 和人工处置

**Files:**

- Modify: `internal/domain/delivery_receipt.go`
- Modify: `internal/repository/delivery_receipt_postgres.go`
- Modify: `internal/repository/delivery_receipt_postgres_test.go`
- Create: `internal/service/delivery_reconciliation_service.go`
- Create: `internal/service/delivery_reconciliation_service_test.go`
- Create: `internal/http/delivery_handler.go`
- Create: `internal/http/delivery_handler_test.go`
- Modify: `internal/app/app.go`
- Modify: `console/src/services/api/permissions.ts`
- Create: `openapi/paths/deliveries.yaml`
- Create: `openapi/schemas/delivery.yaml`
- Modify: `openapi/openapi.yaml`

- [ ] **Step 1: 写 receipt 匹配和 unknown 处置测试**

优先用 provider + provider_message_id 匹配 attempt，次选 signed metadata 中的 effect key；重复 webhook 幂等。人工动作只允许 `mark_confirmed`、`mark_terminal_failed`、`retry_after_verified_not_accepted`，每次动作写 actor/reason/audit。

- [ ] **Step 2: 实现 receipt 状态映射**

accepted/delivered/opened/clicked/bounced/complained 分别更新 receipt 和 intent 的可前进状态；迟到 receipt 不得把 terminal/confirmed 状态回退。无法匹配的 receipt 保留为 orphan 并进入 reconciliation。

- [ ] **Step 3: 实现 reconciliation worker**

对 provider 支持查询的 attempt 拉取状态；不支持查询的 unknown 保持不自动重发，并进入人工队列。任务按 Workspace 分片、使用 lease、带退避和最大查询周期。

- [ ] **Step 4: 实现查询和处置 API**

新增 `GET /api/deliveries.list`、`GET /api/deliveries.get`、`POST /api/deliveries.reconcile`、`POST /api/deliveries.resolveUnknown`，使用 Message History read/write 权限并强制 Workspace 边界。

- [ ] **Step 5: 补 OpenAPI 与测试**

```powershell
go test ./internal/repository ./internal/service ./internal/http -run 'DeliveryReceipt|Reconciliation|ResolveUnknown' -count=1
make openapi-lint
```

- [ ] **Step 6: 提交 Task 5**

```powershell
git add internal/domain/delivery_receipt.go internal/repository/delivery_receipt_postgres.go internal/repository/delivery_receipt_postgres_test.go internal/service/delivery_reconciliation_service.go internal/service/delivery_reconciliation_service_test.go internal/http/delivery_handler.go internal/http/delivery_handler_test.go internal/app/app.go console/src/services/api/permissions.ts openapi/paths/deliveries.yaml openapi/schemas/delivery.yaml openapi/openapi.yaml
git commit -m "feat(delivery): reconcile receipts and unknown submissions"
```

### Task 6: 提供真实投递进度、健康检查和指标

**Files:**

- Modify: `internal/domain/broadcast.go`
- Modify: `internal/service/broadcast_service.go`
- Modify: `internal/service/broadcast_service_test.go`
- Modify: `internal/http/broadcast_handler.go`
- Modify: `internal/http/broadcast_handler_test.go`
- Modify: `internal/http/root_handler.go`
- Modify: `internal/http/root_handler_test.go`
- Create: `internal/observability/delivery_metrics.go`
- Create: `internal/observability/delivery_metrics_test.go`
- Create: `console/src/services/api/delivery.ts`
- Create: `console/src/services/api/delivery.test.ts`
- Modify: `console/src/pages/BroadcastsPage.tsx`
- Create: `console/src/pages/BroadcastsPage.test.tsx`
- Modify: `console/src/services/api/broadcast.ts`

- [ ] **Step 1: 写进度语义失败测试**

Broadcast 响应分别返回 `audience_total/planned/reserved/queued/submitting/accepted/confirmed/suppressed/deferred/failed/unknown`，`processed` 仅保留为 deprecated 聚合字段；UI 不得把 queued 显示成已发送。

- [ ] **Step 2: 实现 ledger 聚合查询和页面状态**

按 source_id + source_version 聚合 intents；列表只读轻量计数，详情按需获取完整 funnel。unknown 和 terminal failure 必须有醒目可操作入口。

- [ ] **Step 3: 扩展 readiness**

保留 `/healthz` 进程存活；新增 `/readyz` 检查 system DB、抽样 Workspace DB、Redis、RabbitMQ 连接和关键 worker 最近 heartbeat。任一必需依赖异常返回 503 和结构化原因。

- [ ] **Step 4: 增加投递指标**

按 channel/provider/status 输出 intent 数、attempt 延迟、claim lease 超时、unknown 数、reconciliation backlog；Workspace ID 只用于日志字段，不作为高基数 Prometheus label。

- [ ] **Step 5: 运行后端和前端测试**

```powershell
go test ./internal/service ./internal/http ./internal/observability -run 'Broadcast.*Delivery|Readiness|DeliveryMetrics' -count=1
Set-Location console
npm test -- --run src/services/api/delivery.test.ts src/services/api/broadcast.test.ts src/pages/BroadcastsPage.test.tsx
Set-Location ..
```

- [ ] **Step 6: 提交 Task 6**

```powershell
git add internal/domain/broadcast.go internal/service/broadcast_service.go internal/service/broadcast_service_test.go internal/http/broadcast_handler.go internal/http/broadcast_handler_test.go internal/http/root_handler.go internal/http/root_handler_test.go internal/observability/delivery_metrics.go internal/observability/delivery_metrics_test.go console/src/services/api/delivery.ts console/src/services/api/delivery.test.ts console/src/pages/BroadcastsPage.tsx console/src/pages/BroadcastsPage.test.tsx console/src/services/api/broadcast.ts
git commit -m "feat(delivery): expose truthful progress and readiness"
```

### Task 7: B1 故障注入、负载验证与运维证据

**Files:**

- Create: `tests/integration/delivery_crash_recovery_test.go`
- Create: `tests/integration/delivery_provider_unknown_test.go`
- Create: `tests/integration/delivery_concurrency_test.go`
- Create: `docs/operations/delivery-reconciliation.md`
- Create: `docs/operations/evidence/b1-delivery-consistency.md`

- [ ] **Step 1: 实现故障注入集成测试**

在 intent insert、queue insert、claim、provider call 前、provider accepted 后、history confirm 前注入一次性故障；重启 worker/orchestrator，断言 effect key 唯一、无盲目重发、最终状态可解释。

- [ ] **Step 2: 运行竞态和集成测试**

```powershell
go test -race ./internal/repository ./internal/service/queue ./internal/service/broadcast -run 'Delivery|EmailQueue|Broadcast' -count=1
$env:INTEGRATION_TESTS='true'; go test -tags integration -timeout 20m ./tests/integration -run 'DeliveryCrash|DeliveryProviderUnknown|DeliveryConcurrency' -count=1; Remove-Item Env:INTEGRATION_TESTS
```

- [ ] **Step 3: 验证调度开销**

使用 fake provider 固定 5ms 响应，至少 100,000 intents、8 workers，记录总吞吐、p95 claim-to-submit 和账本调度开销；除 provider 时间外的调度开销不得超过总处理时间 10%。

- [ ] **Step 4: 编写 reconciliation runbook 和证据**

记录 unknown 判定、provider 查询、人工确认未接受后重试、禁止直接 SQL 改状态、告警阈值和数据保留策略。证据包含测试退出码、吞吐、unknown 样例和重复 effect key 查询为零。

- [ ] **Step 5: 提交 Task 7**

```powershell
git add tests/integration/delivery_crash_recovery_test.go tests/integration/delivery_provider_unknown_test.go tests/integration/delivery_concurrency_test.go docs/operations/delivery-reconciliation.md docs/operations/evidence/b1-delivery-consistency.md
git commit -m "test(delivery): prove b1 crash consistency"
```
