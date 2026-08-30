# Yaoguang B2 Audience Campaign Frequency Import Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付非技术人员可配置的统一客群、不可变活动受众快照、发送前预检、分层频控和百万级名单导入，确保名单完整进入系统且活动执行可复现。

**Architecture:** 在现有 List、Segment、Broadcast 和 Customer 之上增加 Audience 聚合层。Audience version 保存静态/动态/组合定义，build 物化 Customer membership；Campaign version 在启动时生成 immutable recipient snapshot。频控分为活动级、事件/时间触发级、Workspace 全量级，先持久评估再用 Redis 原子预留，结果写 Delivery Intent。文件导入先完整落地 Import Job 和逐行状态，再异步分片调用 Customer authority，任何已接收行都有终态或可恢复中间态。

**Tech Stack:** Go 1.25、PostgreSQL、Redis Lua、RabbitMQ、对象存储、CSV/XLSX streaming parser、React 18、TanStack Query/Router、Ant Design、Vitest/Playwright。

**Spec:** `docs/superpowers/specs/2026-08-30-yaoguang-domain-first-rearchitecture-design.md` 的 B2、Audience、Campaign Snapshot、Frequency Policy、Import Job 和性能目标章节。

## Global Constraints

- Audience 支持 static、dynamic、composite；组合运算明确为 union、intersection、exclusion，支持纯 Segment、多 List 和排除客群。
- Campaign 一旦启动，recipient membership、版本、variant 必须冻结；发送前仍实时检查 Identity、Consent、Suppression、Frequency。
- 频控三层独立：单活动限制、事件/时间触发限制、Workspace 全量限制；任一层 deny 即 suppress，基础设施不可用时 defer，不得 fail-open。
- Automation enrollment frequency `once/every_time` 不属于消息频控，B2 不改变它的业务语义。
- 同步 Customer batch 默认最大 10,000；异步文件 Import Job 默认最大 1,000,000 行；两者均可后台配置。
- Import Job 必须满足 `上传总数 = 待处理 + 处理中 + 成功 + 明确失败`，不允许因 Worker 崩溃丢行。
- 不修改 V48；B2 使用 V49，并覆盖全新 Workspace 初始化。
- 每项能力先完成 domain contract 和失败测试，再接 UI。

---

### Task 1: 定义 Audience/Campaign/Frequency/Import 聚合与 V49 schema

**Files:**

- Create: `internal/domain/audience.go`
- Create: `internal/domain/audience_test.go`
- Create: `internal/domain/campaign.go`
- Create: `internal/domain/campaign_test.go`
- Create: `internal/domain/frequency_policy.go`
- Create: `internal/domain/frequency_policy_test.go`
- Create: `internal/domain/import_job.go`
- Create: `internal/domain/import_job_test.go`
- Create: `internal/database/schema/marketing_tables.go`
- Create: `internal/database/schema/marketing_tables_test.go`
- Create: `internal/migrations/v49.go`
- Create: `internal/migrations/v49_test.go`
- Modify: `internal/database/init.go`
- Modify: `config/config.go`
- Modify: `internal/migrations/v48_test.go`

- [ ] **Step 1: 写四个聚合的领域失败测试**

Audience definition 必须可 canonicalize 和 version；Campaign version 激活后 immutable；Frequency policy 校验窗口、范围和优先级；Import counters 只允许守恒迁移。测试 Workspace ID 不出现在 Workspace 本地唯一键中，跨 Workspace 由数据库隔离保证。

- [ ] **Step 2: 运行 domain 测试确认失败**

```powershell
go test ./internal/domain -run 'Audience|Campaign|FrequencyPolicy|ImportJob' -count=1
```

- [ ] **Step 3: 实现 domain contract**

定义 Repository/Service 接口、请求/响应、状态枚举和 typed errors。Audience expression 用 tagged JSON union，禁止前端直接传 SQL；现有 Segment tree 作为 dynamic leaf 引用。

- [ ] **Step 4: 设计并测试 schema**

创建 `audiences/audience_versions/audience_builds/audience_memberships`、`campaigns/campaign_versions/campaign_runs/campaign_recipient_snapshots`、`frequency_policies/frequency_decisions`、`import_jobs/import_job_rows/import_job_checkpoints`。所有运行表使用 UUID、版本唯一约束、状态 check、稳定游标索引；snapshot 以 `(run_id, ordinal)` 和 `(run_id, customer_id, variant)` 唯一。

- [ ] **Step 5: 实现 V49 和 fresh initialization**

迁移把现有 Broadcast audience 转成一个兼容 Audience version，但不改变历史 Broadcast JSON；代码版本提升到 49.0，更新迁移 reachability 测试。

- [ ] **Step 6: 运行 schema/migration 验证**

```powershell
go test ./internal/domain ./internal/database/schema ./internal/migrations -run 'Audience|Campaign|Frequency|Import|V49' -count=1
```

- [ ] **Step 7: 提交 Task 1**

```powershell
git add internal/domain/audience.go internal/domain/audience_test.go internal/domain/campaign.go internal/domain/campaign_test.go internal/domain/frequency_policy.go internal/domain/frequency_policy_test.go internal/domain/import_job.go internal/domain/import_job_test.go internal/database/schema/marketing_tables.go internal/database/schema/marketing_tables_test.go internal/migrations/v49.go internal/migrations/v49_test.go internal/database/init.go config/config.go internal/migrations/v48_test.go
git commit -m "feat(marketing): add v49 audience campaign frequency import schema"
```

### Task 2: 实现 Audience CRUD、预览和物化构建

**Files:**

- Create: `internal/repository/audience_postgres.go`
- Create: `internal/repository/audience_postgres_test.go`
- Create: `internal/service/audience_service.go`
- Create: `internal/service/audience_service_test.go`
- Create: `internal/service/audience_build_processor.go`
- Create: `internal/service/audience_build_processor_test.go`
- Create: `internal/http/audience_handler.go`
- Create: `internal/http/audience_handler_test.go`
- Modify: `internal/app/app.go`
- Modify: `console/src/services/api/permissions.ts`
- Create: `openapi/paths/audiences.yaml`
- Create: `openapi/schemas/audience.yaml`
- Modify: `openapi/openapi.yaml`

- [ ] **Step 1: 写 expression 编译与 Workspace 隔离测试**

覆盖单 List、纯 Segment、多 List union、List ∩ Segment、Audience A 排除 Audience B、循环引用拒绝、删除被引用 Audience 拒绝、跨 Workspace ID 不可解析。预览只返回 Customer summary 和 total，不泄露 Identity 明文。

- [ ] **Step 2: 运行测试确认失败**

```powershell
go test ./internal/repository ./internal/service ./internal/http -run Audience -count=1
```

- [ ] **Step 3: 实现 Repository 和编译器**

将 expression 编译为参数化 SQL/CTE，leaf 统一输出 `customer_id`。List 使用 `customer_list_memberships`，Segment 使用带 customer_id 的 membership；exclusion 使用 `EXCEPT` 或 anti join。禁止字符串拼接用户值。

- [ ] **Step 4: 实现版本化 CRUD、preview 和 build**

保存时生成新 version；preview 不持久化 membership，限制样例 100 条；build 创建 run、按 `(customer_id)` 游标分页写 membership，断点恢复不重复。完成后原子切换 Audience active build。

- [ ] **Step 5: 实现 HTTP/OpenAPI/权限**

新增 `audiences.list/get/create/update/delete/preview/build/buildStatus/members`；read/write 权限独立，所有请求显式校验 Workspace。

- [ ] **Step 6: 运行测试和 OpenAPI lint**

```powershell
go test ./internal/repository ./internal/service ./internal/http -run Audience -count=1
make openapi-lint
```

- [ ] **Step 7: 提交 Task 2**

```powershell
git add internal/repository/audience_postgres.go internal/repository/audience_postgres_test.go internal/service/audience_service.go internal/service/audience_service_test.go internal/service/audience_build_processor.go internal/service/audience_build_processor_test.go internal/http/audience_handler.go internal/http/audience_handler_test.go internal/app/app.go console/src/services/api/permissions.ts openapi/paths/audiences.yaml openapi/schemas/audience.yaml openapi/openapi.yaml
git commit -m "feat(audience): add versioned audience workbench api"
```

### Task 3: 实现 Campaign version 和不可变 recipient snapshot

**Files:**

- Create: `internal/repository/campaign_postgres.go`
- Create: `internal/repository/campaign_postgres_test.go`
- Create: `internal/service/campaign_service.go`
- Create: `internal/service/campaign_service_test.go`
- Create: `internal/service/campaign_snapshot_processor.go`
- Create: `internal/service/campaign_snapshot_processor_test.go`
- Modify: `internal/domain/broadcast.go`
- Modify: `internal/service/broadcast_service.go`
- Modify: `internal/service/broadcast_service_test.go`
- Modify: `internal/service/broadcast/orchestrator.go`
- Modify: `internal/service/broadcast/orchestrator_test.go`
- Modify: `internal/repository/broadcast_postgres.go`

- [ ] **Step 1: 写 snapshot 冻结失败测试**

活动启动后更改 List、Segment、Customer email、A/B 配比：snapshot membership、variant 和 ordinal 不变；发送时使用当前可用 Identity/Consent；暂停恢复从 snapshot ordinal 继续；同一 Customer 在组合 Audience 中只出现一次。

- [ ] **Step 2: 运行测试确认当前 Broadcast 使用 live audience**

```powershell
go test ./internal/service ./internal/service/broadcast ./internal/repository -run 'CampaignSnapshot|ImmutableAudience|SnapshotResume' -count=1
```

- [ ] **Step 3: 实现 Campaign draft/version/run**

Broadcast 作为兼容 facade 调用 CampaignService。创建/更新 draft 生成版本；schedule 固化版本并创建 run；运行版本禁止 update/delete。A/B variant 使用 customer_id + run seed 的确定性 hash 分配。

- [ ] **Step 4: 实现 snapshot processor**

读取 Audience active build 或即时编译结果，按 customer_id keyset 分页，每批 5,000 insert `ON CONFLICT DO NOTHING`；checkpoint 保存 last customer ID、inserted count、source build/version。完成前 campaign run 不得进入 dispatching。

- [ ] **Step 5: 改造 Broadcast orchestrator 消费 snapshot**

只按 ordinal 扫描 snapshot；B1 effect key 使用 run/version/customer/ordinal/variant。现有单 List audience JSON 只用于兼容展示，不再作为运行时成员来源。

- [ ] **Step 6: 运行测试**

```powershell
go test ./internal/repository ./internal/service ./internal/service/broadcast -run 'Campaign|Broadcast|Snapshot' -count=1
```

- [ ] **Step 7: 提交 Task 3**

```powershell
git add internal/repository/campaign_postgres.go internal/repository/campaign_postgres_test.go internal/service/campaign_service.go internal/service/campaign_service_test.go internal/service/campaign_snapshot_processor.go internal/service/campaign_snapshot_processor_test.go internal/domain/broadcast.go internal/service/broadcast_service.go internal/service/broadcast_service_test.go internal/service/broadcast/orchestrator.go internal/service/broadcast/orchestrator_test.go internal/repository/broadcast_postgres.go
git commit -m "feat(campaign): freeze versioned recipient snapshots"
```

### Task 4: 增加统一发送前 Preflight

**Files:**

- Create: `internal/domain/marketing_preflight.go`
- Create: `internal/service/marketing_preflight_service.go`
- Create: `internal/service/marketing_preflight_service_test.go`
- Modify: `internal/http/broadcast_handler.go`
- Modify: `internal/http/broadcast_handler_test.go`
- Modify: `console/src/services/api/broadcast.ts`
- Create: `console/src/components/broadcasts/PreflightSummary.tsx`
- Create: `console/src/components/broadcasts/PreflightSummary.test.tsx`
- Modify: `console/src/components/broadcasts/SendOrScheduleModal.tsx`
- Modify: `console/src/components/broadcasts/UpsertBroadcastDrawer.test.tsx`

- [ ] **Step 1: 写 preflight 失败测试**

结果必须分类：目标总数、可触达、无有效 Identity、无 Consent、已 Suppressed、频控预计 deny、模板/Provider 未配置、变量样例失败、Audience build 过期。blocking issue 阻止 schedule；warning 需要用户明确确认，不新增审批人。

- [ ] **Step 2: 实现后端 preview API**

新增 `POST /api/broadcasts.preflight`，输入 draft/version，输出基于同一 Audience/Campaign compiler 的结果，禁止前端自行估算。大客群使用统计查询和有界样例，不全量加载内存。

- [ ] **Step 3: 改造 Send Now/Schedule 流程**

点击发送先打开 preflight modal；展示业务语言和修复链接；只有 `blocking_count=0` 且用户确认 summary hash 后，schedule 请求才接受。后端重新验证 hash，防止页面停留期间配置漂移。

- [ ] **Step 4: 运行前后端测试**

```powershell
go test ./internal/service ./internal/http -run Preflight -count=1
Set-Location console
npm test -- --run src/components/broadcasts/PreflightSummary.test.tsx src/components/broadcasts/UpsertBroadcastDrawer.test.tsx
Set-Location ..
```

- [ ] **Step 5: 提交 Task 4**

```powershell
git add internal/domain/marketing_preflight.go internal/service/marketing_preflight_service.go internal/service/marketing_preflight_service_test.go internal/http/broadcast_handler.go internal/http/broadcast_handler_test.go console/src/services/api/broadcast.ts console/src/components/broadcasts/PreflightSummary.tsx console/src/components/broadcasts/PreflightSummary.test.tsx console/src/components/broadcasts/SendOrScheduleModal.tsx console/src/components/broadcasts/UpsertBroadcastDrawer.test.tsx
git commit -m "feat(campaign): require truthful send preflight"
```

### Task 5: 实现三层 Frequency Policy 与原子多策略预留

**Files:**

- Create: `internal/repository/frequency_policy_postgres.go`
- Create: `internal/repository/frequency_policy_postgres_test.go`
- Create: `internal/service/frequency_policy_service.go`
- Create: `internal/service/frequency_policy_service_test.go`
- Modify: `internal/service/frequency_cap.go`
- Modify: `internal/service/frequency_cap_test.go`
- Modify: `pkg/realtimecache/redis.go`
- Modify: `pkg/realtimecache/redis_test.go`
- Modify: `internal/service/broadcast/queue_message_sender.go`
- Modify: `internal/service/broadcast/queue_message_sender_test.go`
- Modify: `internal/service/realtime_delivery_worker.go`
- Modify: `internal/service/realtime_delivery_worker_test.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: 写策略优先级和分类失败测试**

活动级 policy 只计同一 campaign/run；trigger 级 policy 按 automation + trigger kind 计数；global policy 按 Workspace + Customer + channel 计数。一次评估同时命中多 policy 时必须全有额度才 allow，任何 Redis 错误整体 defer，不能部分占用。

- [ ] **Step 2: 运行测试确认当前 limiter 只支持单 policy**

```powershell
go test ./internal/service ./pkg/realtimecache -run 'Frequency|MultiPolicy|Reservation' -count=1
```

- [ ] **Step 3: 实现持久策略和解析服务**

策略 scope 为 `campaign/trigger/workspace_global`，包含 channel、max events、sliding/calendar window、timezone、deny action、priority、enabled。解析结果带具体 policy/version，便于审计复现。

- [ ] **Step 4: 实现 Redis Lua 原子预留**

一个 Lua 调用检查全部 ZSET/计数窗口并一次性写入相同 reservation ID；已有 reservation 重放返回相同决策且不重复计数；deny 不写任何窗口。key 必须含 Workspace、Customer、channel、policy version。

- [ ] **Step 5: 接入 Campaign 和 Realtime Delivery**

在创建 B1 Delivery Intent 前评估；allow 保存 decision/reservation，deny 创建 `suppressed` intent 并记录命中层级，defer 创建 `deferred` intent 供 scheduler 重试。Automation enrollment `once/every_time` 检查仍在触发层，不能被 limiter 替换。

- [ ] **Step 6: 运行集成测试**

```powershell
go test ./internal/repository ./internal/service ./internal/service/broadcast ./pkg/realtimecache -run Frequency -count=1
```

- [ ] **Step 7: 提交 Task 5**

```powershell
git add internal/repository/frequency_policy_postgres.go internal/repository/frequency_policy_postgres_test.go internal/service/frequency_policy_service.go internal/service/frequency_policy_service_test.go internal/service/frequency_cap.go internal/service/frequency_cap_test.go pkg/realtimecache/redis.go pkg/realtimecache/redis_test.go internal/service/broadcast/queue_message_sender.go internal/service/broadcast/queue_message_sender_test.go internal/service/realtime_delivery_worker.go internal/service/realtime_delivery_worker_test.go internal/app/app.go
git commit -m "feat(frequency): enforce campaign trigger and global caps"
```

### Task 6: 实现百万级 Import Job 完整落地和断点恢复

**Files:**

- Create: `internal/repository/import_job_postgres.go`
- Create: `internal/repository/import_job_postgres_test.go`
- Create: `internal/service/import_job_service.go`
- Create: `internal/service/import_job_service_test.go`
- Create: `internal/service/import_job_worker.go`
- Create: `internal/service/import_job_worker_test.go`
- Create: `internal/http/import_job_handler.go`
- Create: `internal/http/import_job_handler_test.go`
- Modify: `internal/app/app.go`
- Modify: `config/config.go`
- Create: `openapi/paths/import_jobs.yaml`
- Create: `openapi/schemas/import_job.yaml`
- Modify: `openapi/openapi.yaml`

- [ ] **Step 1: 写“先完整接收、后处理”的失败测试**

上传 10,001、500,000、1,000,000 行时先创建 job，并把每一行以 ordinal + raw payload + checksum 写入 staging；解析错误也必须成为明确失败行。模拟第 37 批崩溃后重启，所有 ordinal 只处理一次或幂等重放。

- [ ] **Step 2: 运行测试确认失败**

```powershell
go test ./internal/repository ./internal/service ./internal/http -run ImportJob -count=1
```

- [ ] **Step 3: 实现上传和 staging**

配置 `CUSTOMER_IMPORT_MAX_ROWS` 默认 1,000,000、`CUSTOMER_IMPORT_PROCESS_CHUNK_SIZE` 默认 2,000、`CUSTOMER_IMPORT_MAX_FILE_BYTES` 默认 1 GiB。上传流写对象存储并计算 checksum；解析流逐批 insert staging。超过限制时 job 进入 rejected，已上传文件保留到清理周期并给出明确原因。

- [ ] **Step 4: 实现 Worker claim 和 Customer batch**

原子 claim pending rows，按配置 chunk 调 `CustomerService.UpsertCustomerBatch`；逐条保存 customer_id/action/error_code。Worker lease 过期可恢复，相同 row 使用 `import:<job_id>:<ordinal>:<row_checksum>` 幂等键。

- [ ] **Step 5: 实现守恒计数与错误导出**

计数从行状态聚合或同事务增量更新并定期校正；job 只有全部行 terminal 才 completed。提供失败 CSV 导出，包含 ordinal、外部 ID、display identity、错误码、用户可理解说明，不包含敏感明文。

- [ ] **Step 6: 实现 API/OpenAPI**

新增 `imports.create/upload/commit/get/list/cancel/errors`；上传完成前不启动 worker；commit 校验对象 checksum 和 staged total。取消只阻止未处理行，并将其变为明确失败 `cancelled_by_user`，保持守恒。

- [ ] **Step 7: 运行测试**

```powershell
go test ./internal/repository ./internal/service ./internal/http -run ImportJob -count=1
make openapi-lint
```

- [ ] **Step 8: 提交 Task 6**

```powershell
git add internal/repository/import_job_postgres.go internal/repository/import_job_postgres_test.go internal/service/import_job_service.go internal/service/import_job_service_test.go internal/service/import_job_worker.go internal/service/import_job_worker_test.go internal/http/import_job_handler.go internal/http/import_job_handler_test.go internal/app/app.go config/config.go openapi/paths/import_jobs.yaml openapi/schemas/import_job.yaml openapi/openapi.yaml
git commit -m "feat(import): add durable million-row customer jobs"
```

### Task 7: 上线 Audience、Preflight、Frequency 和 Import 的业务 UI

**Files:**

- Create: `console/src/services/api/audience.ts`
- Create: `console/src/services/api/audience.test.ts`
- Create: `console/src/services/api/frequency_policy.ts`
- Create: `console/src/services/api/import_job.ts`
- Create: `console/src/pages/AudiencesPage.tsx`
- Create: `console/src/pages/AudiencesPage.test.tsx`
- Create: `console/src/components/audiences/AudienceBuilderDrawer.tsx`
- Create: `console/src/components/audiences/AudienceBuilderDrawer.test.tsx`
- Create: `console/src/components/frequency/FrequencyPolicyForm.tsx`
- Create: `console/src/components/frequency/FrequencyPolicyForm.test.tsx`
- Create: `console/src/components/customers/CustomerImportWizard.tsx`
- Create: `console/src/components/customers/CustomerImportWizard.test.tsx`
- Modify: `console/src/components/broadcasts/UpsertBroadcastDrawer.tsx`
- Modify: `console/src/components/broadcasts/UpsertBroadcastDrawer.test.tsx`
- Modify: `console/src/router.tsx`
- Modify: `console/src/layouts/WorkspaceLayout.tsx`
- Modify: `console/src/i18n/catalogInventory.ts`
- Modify: `console/src/i18n/locales/zh-CN.po`

- [ ] **Step 1: 写业务流程失败测试**

非技术用户能选择“满足任一/满足全部/排除”、List、Segment 并实时预览；Campaign 选择一个 Audience 而不是必须一个 List；频控表单分“本活动、事件/定时触发、全局”三张卡；导入 Wizard 显示上传、字段映射、校验、后台处理、完成五步。

- [ ] **Step 2: 实现 Audience 工作台**

复用现有 Segment tree 组件作为“条件客群”leaf 编辑器，但默认 UI 使用业务术语；Generated SQL 只在可折叠诊断区展示。列表显示类型、预估人数、最近构建、使用中的 Campaign。

- [ ] **Step 3: 改造 Campaign audience 与 preflight UI**

Broadcast drawer 使用 Audience selector，提供“从当前 List/Segment 组合创建客群”快捷入口。发送确认展示 snapshot 生成阶段和 preflight 分类，不允许绕过。

- [ ] **Step 4: 实现频控表单**

默认提供关闭状态和推荐模板；活动级、trigger 级、global 级分别保存。表单用“每位客户在 X 小时/自然日最多 Y 次”句式，timezone 只在 calendar window 时出现。

- [ ] **Step 5: 实现 Import Wizard**

浏览器仅抽样解析前 100 行用于映射预览，完整文件直接流式上传；上传/commit 后页面可关闭，任务在后台继续。进度以总数守恒显示，失败行可下载，不使用虚假的 100% 进度。

- [ ] **Step 6: 运行前端测试与构建**

```powershell
Set-Location console
npm test -- --run src/services/api/audience.test.ts src/pages/AudiencesPage.test.tsx src/components/audiences/AudienceBuilderDrawer.test.tsx src/components/frequency/FrequencyPolicyForm.test.tsx src/components/customers/CustomerImportWizard.test.tsx src/components/broadcasts/UpsertBroadcastDrawer.test.tsx
npm run lint
npm run build
Set-Location ..
```

- [ ] **Step 7: 提交 Task 7**

```powershell
git add console/src/services/api/audience.ts console/src/services/api/audience.test.ts console/src/services/api/frequency_policy.ts console/src/services/api/import_job.ts console/src/pages/AudiencesPage.tsx console/src/pages/AudiencesPage.test.tsx console/src/components/audiences/AudienceBuilderDrawer.tsx console/src/components/audiences/AudienceBuilderDrawer.test.tsx console/src/components/frequency/FrequencyPolicyForm.tsx console/src/components/frequency/FrequencyPolicyForm.test.tsx console/src/components/customers/CustomerImportWizard.tsx console/src/components/customers/CustomerImportWizard.test.tsx console/src/components/broadcasts/UpsertBroadcastDrawer.tsx console/src/components/broadcasts/UpsertBroadcastDrawer.test.tsx console/src/router.tsx console/src/layouts/WorkspaceLayout.tsx console/src/i18n/catalogInventory.ts console/src/i18n/locales/zh-CN.po
git commit -m "feat(console): add audience frequency and import workflows"
```

### Task 8: B2 百万量级、频控并发和完整性验收

**Files:**

- Create: `tests/integration/audience_snapshot_scale_test.go`
- Create: `tests/integration/frequency_policy_concurrency_test.go`
- Create: `tests/integration/import_job_recovery_test.go`
- Create: `docs/operations/customer-import-jobs.md`
- Create: `docs/operations/evidence/b2-marketing-execution.md`

- [ ] **Step 1: 运行百万客户规模测试**

生成 1,000,000 Customer，构建组合 Audience、snapshot 和 Import Job；验证全程 keyset 分页、内存不随总量线性增长、重复 customer 为零、snapshot 可恢复。

- [ ] **Step 2: 运行频控并发测试**

100 并发 goroutine 对同一 Customer/三层 policy 预留，实际 allow 数不得超过最严格上限；相同 reservation 重放不增加计数；Redis 故障全部 defer。

- [ ] **Step 3: 运行导入 crash recovery**

在 upload、staging、claim、Customer batch、row completion 五个边界重启；每次验证守恒公式、无丢失 ordinal、明确失败可下载、成功 Customer 可查询。

- [ ] **Step 4: 运行命令并记录性能目标**

```powershell
$env:INTEGRATION_TESTS='true'; go test -tags integration -timeout 30m ./tests/integration -run 'AudienceSnapshotScale|FrequencyPolicyConcurrency|ImportJobRecovery' -count=1; Remove-Item Env:INTEGRATION_TESTS
```

记录 1m Audience preview/list p95 < 2s、event accept p95 < 200ms、snapshot/build 吞吐、1m import 完成时间和峰值 RSS；未达标不得将 B2 标记完成。

- [ ] **Step 5: 提交 B2 证据**

```powershell
git add tests/integration/audience_snapshot_scale_test.go tests/integration/frequency_policy_concurrency_test.go tests/integration/import_job_recovery_test.go docs/operations/customer-import-jobs.md docs/operations/evidence/b2-marketing-execution.md
git commit -m "test(marketing): prove b2 scale and import integrity"
```
