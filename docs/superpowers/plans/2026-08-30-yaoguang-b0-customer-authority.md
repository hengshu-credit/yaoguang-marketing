# Yaoguang B0 Customer Authority Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 `Customer` 确立为 Workspace 内唯一客户写模型，让现有 Contact API、列表、标签、画像与触点通过兼容适配层读写同一客户，同时补齐可检索的客户列表和 Customer 360 基础页面。

**Architecture:** 保留每个 Workspace 独立数据库和现有 `customers/customer_profiles/customer_identities/customer_consents/customer_list_memberships` 表。新写入统一进入 `CustomerService`，`contacts` 只作为 Email 兼容投影；迁移采用可空 `customer_id`、后台回填、双读校验、最后收紧约束的渐进方式。旧 API 不删除，响应继续兼容 Email 联系人调用方。

**Tech Stack:** Go 1.25、PostgreSQL、`net/http`、React 18、TypeScript、TanStack Router、TanStack Query、Ant Design、Vitest、Go test/sqlmock。

**Spec:** `docs/superpowers/specs/2026-08-30-yaoguang-domain-first-rearchitecture-design.md` 的 B0、Customer、Workspace 隔离、身份与兼容迁移章节。

## Global Constraints

- 每个 Workspace 独立存储；`customer_no`、`external_user_id`、Identity 唯一性只在同一 Workspace 数据库内生效。
- 内部主键为 UUID；客户编号格式保持 `U + 四位 Workspace 序号 + yyyyMMddHHmmss + 08 + 32 位无连字符 UUID`。
- 继续支持直接使用外部系统提供的 `external_user_id` 定位和幂等更新客户。
- 不修改已发布的 V46 迁移；B0 使用 V47，并同时覆盖全新 Workspace 初始化路径。
- 不删除 Contact API，不改变旧调用方的 Email 主键响应；所有旧写接口改为 Customer 命令适配器。
- 批量请求默认最大 10,000 条，读取 `CUSTOMER_SYNC_MAX_BATCH_SIZE`；逐条结果与输入下标严格对应。
- 未经授权不引入审批流；Workspace 现有审批能力保持原状。
- 每个任务先写失败测试，再写最小实现，再运行指定验证命令。

---

### Task 1: 建立 V47 Customer 兼容引用与迁移可观测性

**Files:**

- Create: `internal/database/schema/customer_authority.go`
- Create: `internal/database/schema/customer_authority_test.go`
- Create: `internal/migrations/v47.go`
- Create: `internal/migrations/v47_test.go`
- Modify: `internal/database/init.go`
- Modify: `config/config.go`
- Modify: `internal/migrations/v41_test.go`
- Modify: `internal/migrations/v42_test.go`
- Modify: `internal/migrations/v43_test.go`
- Modify: `internal/migrations/v44_test.go`
- Modify: `internal/migrations/v45_test.go`
- Modify: `internal/migrations/v46_test.go`

- [ ] **Step 1: 写 schema contract 失败测试**

断言 `CustomerAuthorityTableDefinitions()` 为 `contact_lists`、`contact_segments`、`custom_events`、`contact_timeline`、`contact_automations`、`automation_trigger_log`、`message_history`、`email_queue` 增加可空 `customer_id UUID`、Workspace 本地外键和按业务访问顺序建立的部分索引；同时创建 `customer_projection_reconciliation` 表，记录实体、缺失数量、最近扫描时间和最近错误。

- [ ] **Step 2: 运行测试确认失败**

```powershell
go test ./internal/database/schema -run CustomerAuthority -count=1
```

Expected: `CustomerAuthorityTableDefinitions` 未定义或 contract 不满足。

- [ ] **Step 3: 实现共享 schema 定义并接入全新 Workspace 初始化**

所有 DDL 使用 `ADD COLUMN IF NOT EXISTS`、命名外键和 `WHERE customer_id IS NOT NULL` 的部分索引。将 schema 定义追加到 `internal/database/init.go` 的 Customer 表之后、Channel Send 表之前执行。

- [ ] **Step 4: 写 V47 失败测试并实现迁移**

V47 对每个 Workspace 执行共享 DDL，并用确定性 SQL 从 `contacts.customer_id` 与 Email 关联回填兼容表。冲突行不得覆盖，必须写入 `customer_projection_reconciliation`。迁移结束统计每张表的 `customer_id IS NULL` 行数，不因历史脏数据阻断启动。

- [ ] **Step 5: 将代码版本提升到 47.0 并验证注册可达**

更新 `config.VERSION` 和所有断言当前代码版本的迁移测试；`v47_test.go` 必须断言 `GetRegisteredMigration(47.0)` 可达、V46 文件未被修改。

- [ ] **Step 6: 运行 schema 与迁移测试**

```powershell
go test ./internal/database/schema ./internal/migrations -run 'CustomerAuthority|V47|RegisteredMigrations' -count=1
```

- [ ] **Step 7: 提交 Task 1**

```powershell
git add internal/database/schema/customer_authority.go internal/database/schema/customer_authority_test.go internal/migrations/v47.go internal/migrations/v47_test.go internal/database/init.go config/config.go internal/migrations/v41_test.go internal/migrations/v42_test.go internal/migrations/v43_test.go internal/migrations/v44_test.go internal/migrations/v45_test.go internal/migrations/v46_test.go
git commit -m "feat(customer): add v47 authority compatibility schema"
```

### Task 2: 补齐 Customer 列表、搜索与游标分页

**Files:**

- Modify: `internal/domain/customer.go`
- Modify: `internal/domain/customer_test.go`
- Modify: `internal/repository/customer_postgres.go`
- Modify: `internal/repository/customer_postgres_test.go`
- Modify: `internal/service/customer_service.go`
- Modify: `internal/service/customer_service_test.go`
- Modify: `internal/http/customer_handler.go`
- Modify: `internal/http/customer_handler_test.go`
- Modify: `internal/domain/mocks/mock_customer_repository.go`
- Modify: `internal/domain/mocks/mock_customer_service.go`
- Modify: `openapi/openapi.yaml`
- Create: `openapi/paths/customers_list.yaml`
- Create: `openapi/schemas/customer_list.yaml`

- [ ] **Step 1: 定义列表 contract 与失败测试**

新增 `CustomerListRequest`、`CustomerListCursor`、`CustomerSummary`、`CustomerListResponse`。支持按 `customer_no`、`external_user_id`、Email/Phone display hint、姓名属性搜索，按 `created_at,id` 稳定游标分页，默认 50、最大 200；默认排除已合并客户，显式参数可包含。

- [ ] **Step 2: 运行 domain 测试确认失败**

```powershell
go test ./internal/domain -run CustomerList -count=1
```

- [ ] **Step 3: 实现 Repository 查询**

在同一 Workspace DB 内查询 `customers`，用 `EXISTS` 子查询筛选 Identity，避免多 Identity 导致重复分页。搜索字符串先规范化；Identity 精确匹配走 fingerprint，模糊搜索只使用 `display_hint`，不得解密全表。

- [ ] **Step 4: 实现 Service 权限与 HTTP 路由**

新增 `GET /api/customers.list`；使用 Customers read 权限；严格校验 `workspace_id/limit/cursor/search/include_merged`，错误响应沿用统一 API error envelope。

- [ ] **Step 5: 补 OpenAPI 并重新生成 mocks**

```powershell
go generate ./internal/domain
make openapi-lint
```

- [ ] **Step 6: 运行分层测试**

```powershell
go test ./internal/domain ./internal/repository ./internal/service ./internal/http -run 'CustomerList|CustomersList' -count=1
```

- [ ] **Step 7: 提交 Task 2**

```powershell
git add internal/domain/customer.go internal/domain/customer_test.go internal/repository/customer_postgres.go internal/repository/customer_postgres_test.go internal/service/customer_service.go internal/service/customer_service_test.go internal/http/customer_handler.go internal/http/customer_handler_test.go internal/domain/mocks/mock_customer_repository.go internal/domain/mocks/mock_customer_service.go openapi/openapi.yaml openapi/paths/customers_list.yaml openapi/schemas/customer_list.yaml
git commit -m "feat(customer): add searchable customer list api"
```

### Task 3: 将旧 Contact 写入口收口到 CustomerService

**Files:**

- Create: `internal/service/legacy_contact_adapter.go`
- Create: `internal/service/legacy_contact_adapter_test.go`
- Modify: `internal/service/contact_service.go`
- Modify: `internal/service/contact_service_test.go`
- Modify: `internal/service/ingest_service.go`
- Modify: `internal/service/ingest_service_test.go`
- Modify: `internal/app/app.go`
- Modify: `internal/http/contact_handler_test.go`

- [ ] **Step 1: 写旧请求到 Customer 命令的映射测试**

覆盖 `contacts.upsert`、`contacts.import`、ingest profile 三个入口：Email 映射为 primary email identity，旧 profile 字段进入 `CustomerProfile`，tags/lists 保持语义；幂等键为 `legacy-contact:<operation>:<normalized-email>:<canonical-payload-sha256>`，相同负载重放不新增客户，负载变化生成新命令。

- [ ] **Step 2: 运行测试确认 ContactService 仍直接写 ContactRepository**

```powershell
go test ./internal/service -run 'LegacyContactAdapter|ContactService.*CustomerAuthority|Ingest.*CustomerAuthority' -count=1
```

- [ ] **Step 3: 实现 `LegacyContactAdapter`**

适配器只负责规范化和组装 `UpsertCustomerRequest/CustomerBatchUpsertRequest`，不复制 Customer 冲突、客户编号或加密逻辑。批量请求按 `CUSTOMER_SYNC_MAX_BATCH_SIZE` 校验，返回项保持输入顺序和输入下标。

- [ ] **Step 4: 注入并替换旧写路径**

`ContactService` 的读方法继续走 Contact 兼容投影；Upsert/Import 经适配器调用 CustomerService。`IngestService` 不再直接 `BulkUpsertContacts`。应用启动时显式注入同一个 CustomerService 实例，避免循环依赖。

- [ ] **Step 5: 验证旧 HTTP contract 不变**

更新 handler 测试，确认旧请求/响应字段、状态码和权限不变；同时断言 Customer 冲突被转换为旧 API 可识别的 409，而不是 500。

- [ ] **Step 6: 运行回归测试**

```powershell
go test ./internal/service ./internal/http -run 'Contact|Ingest|Customer' -count=1
```

- [ ] **Step 7: 提交 Task 3**

```powershell
git add internal/service/legacy_contact_adapter.go internal/service/legacy_contact_adapter_test.go internal/service/contact_service.go internal/service/contact_service_test.go internal/service/ingest_service.go internal/service/ingest_service_test.go internal/app/app.go internal/http/contact_handler_test.go
git commit -m "refactor(customer): route legacy contact writes through authority"
```

### Task 4: 使列表、标签、分群和事件以 customer_id 为主关联

**Files:**

- Modify: `internal/domain/contact_list.go`
- Modify: `internal/domain/segment.go`
- Modify: `internal/repository/contact_list_postgres.go`
- Modify: `internal/repository/contact_list_postgres_test.go`
- Modify: `internal/repository/segment_postgres.go`
- Modify: `internal/repository/segment_postgres_test.go`
- Modify: `internal/repository/custom_event_postgres.go`
- Modify: `internal/repository/custom_event_postgres_test.go`
- Modify: `internal/repository/contact_timeline_postgre.go`
- Modify: `internal/repository/contact_timeline_postgre_test.go`
- Modify: `internal/repository/customer_postgres.go`
- Modify: `internal/repository/customer_postgres_test.go`

- [ ] **Step 1: 写 customer_id 优先、Email 回退的仓储测试**

覆盖：Customer 有 UUID 时 membership/event/timeline 同时写 `customer_id`；历史记录只有 Email 时仍可读取；同一客户更换 primary Email 后 membership 和 timeline 不丢失；merge 后查询自动解析到目标 Customer。

- [ ] **Step 2: 运行 repository 测试确认失败**

```powershell
go test ./internal/repository -run 'CustomerID|CustomerAuthority|PrimaryEmailChange|CustomerMergeProjection' -count=1
```

- [ ] **Step 3: 扩展领域参数和仓储 SQL**

新写 SQL 以 `customer_id` 为稳定键，并在兼容期同步保存 Email 展示值。读 SQL 使用 `COALESCE(explicit_customer_id, contacts.customer_id)` 解析历史行；业务去重键不得继续只依赖 Email。

- [ ] **Step 4: 在 Customer 事务内同步兼容投影**

Customer upsert/merge 同一事务更新 `contacts`、`contact_endpoints`、customer list membership 及相应兼容行；merge 将 source 的 UUID 引用迁移到 target，并按目标状态优先级消解列表状态冲突。

- [ ] **Step 5: 运行仓储和集成测试**

```powershell
go test ./internal/repository -run 'ContactList|Segment|CustomEvent|Timeline|Customer' -count=1
$env:INTEGRATION_TESTS='true'; go test -tags integration ./tests/integration -run 'CustomerProfile|CustomerMerge' -count=1; Remove-Item Env:INTEGRATION_TESTS
```

- [ ] **Step 6: 提交 Task 4**

```powershell
git add internal/domain/contact_list.go internal/domain/segment.go internal/repository/contact_list_postgres.go internal/repository/contact_list_postgres_test.go internal/repository/segment_postgres.go internal/repository/segment_postgres_test.go internal/repository/custom_event_postgres.go internal/repository/custom_event_postgres_test.go internal/repository/contact_timeline_postgre.go internal/repository/contact_timeline_postgre_test.go internal/repository/customer_postgres.go internal/repository/customer_postgres_test.go
git commit -m "refactor(customer): key audience activity by customer id"
```

### Task 5: 增加投影核对、修复命令与切换开关

**Files:**

- Create: `internal/domain/customer_reconciliation.go`
- Create: `internal/repository/customer_reconciliation_postgres.go`
- Create: `internal/repository/customer_reconciliation_postgres_test.go`
- Create: `internal/service/customer_reconciliation_service.go`
- Create: `internal/service/customer_reconciliation_service_test.go`
- Create: `internal/http/customer_reconciliation_handler.go`
- Create: `internal/http/customer_reconciliation_handler_test.go`
- Modify: `internal/app/app.go`
- Modify: `console/src/services/api/permissions.ts`
- Create: `docs/operations/customer-v47-authority-cutover.md`

- [ ] **Step 1: 写 reconciliation contract 测试**

定义 `scan` 只读核对和 `repair` 显式修复两个命令。统计 contacts 无 Customer、Customer 无 Contact 投影、Identity/Email 不一致、各兼容表缺失 UUID、跨客户冲突；所有结果按 Workspace 保存并带运行 ID。

- [ ] **Step 2: 实现后台分片扫描与幂等修复**

每批 2,000 行、按稳定主键游标推进；repair 只补缺失引用和投影，不覆盖冲突数据。运行锁以 Workspace + job type 唯一，进程重启可从 checkpoint 恢复。

- [ ] **Step 3: 提供受 Customers write 权限保护的运维 API**

新增 `POST /api/customers.reconciliation.scan`、`GET /api/customers.reconciliation.get`、`POST /api/customers.reconciliation.repair`。不允许一次请求跨 Workspace。

- [ ] **Step 4: 编写切换 runbook**

runbook 固定顺序：部署 V47 → 等迁移完成 → scan → repair → 再 scan 为零 → 启用 Customer-authority write → 观察 24 小时 → 保留 Contact fallback。包含回退为兼容双读的步骤，禁止回滚已写 Customer 数据。

- [ ] **Step 5: 运行测试**

```powershell
go test ./internal/repository ./internal/service ./internal/http -run CustomerReconciliation -count=1
```

- [ ] **Step 6: 提交 Task 5**

```powershell
git add internal/domain/customer_reconciliation.go internal/repository/customer_reconciliation_postgres.go internal/repository/customer_reconciliation_postgres_test.go internal/service/customer_reconciliation_service.go internal/service/customer_reconciliation_service_test.go internal/http/customer_reconciliation_handler.go internal/http/customer_reconciliation_handler_test.go internal/app/app.go console/src/services/api/permissions.ts docs/operations/customer-v47-authority-cutover.md
git commit -m "feat(customer): add authority reconciliation and cutover controls"
```

### Task 6: 上线 Customer 列表与 Customer 360 基础页

**Files:**

- Create: `console/src/services/api/customer.ts`
- Create: `console/src/services/api/customer.test.ts`
- Create: `console/src/pages/CustomersPage.tsx`
- Create: `console/src/pages/CustomersPage.test.tsx`
- Create: `console/src/components/customers/CustomerDrawer.tsx`
- Create: `console/src/components/customers/CustomerDrawer.test.tsx`
- Modify: `console/src/router.tsx`
- Modify: `console/src/layouts/WorkspaceLayout.tsx`
- Modify: `console/src/__tests__/WorkspaceLayout.test.tsx`
- Modify: `console/src/i18n/catalogInventory.ts`
- Modify: `console/src/i18n/locales/zh-CN.po`

- [ ] **Step 1: 写 API client 与页面交互失败测试**

覆盖：搜索客户编号/外部 ID/手机号/Email、游标翻页、空状态、错误重试、点击行打开详情；详情展示客户编号、外部 ID、画像、Identity display hint、标签、列表和 consent，不向浏览器返回 identity 密文。

- [ ] **Step 2: 实现 Customer API client 与 Query keys**

Query key 必须包含 Workspace、search、cursor、includeMerged；切换 Workspace 时清理上一 Workspace 的客户缓存。

- [ ] **Step 3: 实现列表和基础 360 Drawer**

桌面端表格、窄屏卡片；可复制客户编号和外部用户 ID；合并客户清楚显示跳转目标。旧 `/contacts` 路由保留并在页面顶部说明其为 Email 兼容视图，不自动重定向，避免破坏书签。

- [ ] **Step 4: 增加一级导航“客户”并完成中文文案**

Customer 作为一级入口；375px 下导航折叠为 Drawer，页面 `document.scrollWidth` 不得大于 viewport。使用 Lingui message，不写双语硬编码。

- [ ] **Step 5: 运行前端测试与构建**

```powershell
Set-Location console
npm test -- --run src/services/api/customer.test.ts src/pages/CustomersPage.test.tsx src/components/customers/CustomerDrawer.test.tsx src/__tests__/WorkspaceLayout.test.tsx
npm run build
Set-Location ..
```

- [ ] **Step 6: 提交 Task 6**

```powershell
git add console/src/services/api/customer.ts console/src/services/api/customer.test.ts console/src/pages/CustomersPage.tsx console/src/pages/CustomersPage.test.tsx console/src/components/customers/CustomerDrawer.tsx console/src/components/customers/CustomerDrawer.test.tsx console/src/router.tsx console/src/layouts/WorkspaceLayout.tsx console/src/__tests__/WorkspaceLayout.test.tsx console/src/i18n/catalogInventory.ts console/src/i18n/locales/zh-CN.po
git commit -m "feat(console): add customer list and customer 360 foundation"
```

### Task 7: B0 全量验证与验收证据

**Files:**

- Create: `docs/operations/evidence/b0-customer-authority.md`
- Modify: `docs/operations/customer-v47-authority-cutover.md`

- [ ] **Step 1: 运行后端全量测试**

```powershell
go test ./internal/domain ./internal/database/schema ./internal/migrations ./internal/repository ./internal/service ./internal/http ./internal/app -count=1
```

- [ ] **Step 2: 运行 Customer 集成测试**

```powershell
$env:INTEGRATION_TESTS='true'; go test -tags integration -timeout 20m ./tests/integration -run 'Customer|Contact|Ingest' -count=1; Remove-Item Env:INTEGRATION_TESTS
```

- [ ] **Step 3: 运行前端质量门禁**

```powershell
Set-Location console
npm test -- --run
npm run lint
npm run build
Set-Location ..
```

- [ ] **Step 4: 记录验收证据**

记录测试命令、退出码、版本 47.0、一个 Workspace 内 Customer 编号样例、跨 Workspace 相同 external ID 成功、同 Workspace 重复 external ID 冲突、10,000 条批量逐项守恒、reconciliation 零缺口和 375px 无横向滚动截图路径。

- [ ] **Step 5: 提交 B0 验收证据**

```powershell
git add docs/operations/evidence/b0-customer-authority.md docs/operations/customer-v47-authority-cutover.md
git commit -m "docs(customer): record b0 authority acceptance evidence"
```
