# Yaoguang B3 Journey And Console Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Automation 升级为以 Customer 和 Event 为主键、可解释且可追踪的营销 Journey，并完成面向非技术人员的瑶光控制台信息架构、模板化配置、全渠道内容选择和移动端可用性。

**Architecture:** 保留现有 Automation JSON/node executor、Realtime ledger/outbox/inbox、trigger binding、journey lease 和审批字段；通过 V50 增加 Customer enrollment ledger、entry guard 和 Journey trace 投影。`once/every_time` 仍是自动化入场频次，和消息频控分层。前端使用一级导航和任务导向向导，复杂树/SQL/运行明细作为渐进展开，不隐藏已有专业能力。

**Tech Stack:** Go 1.25、PostgreSQL、RabbitMQ、Redis、React 18、TypeScript、TanStack Router/Query、React Flow、Ant Design、Lingui、Vitest、Playwright。

**Spec:** `docs/superpowers/specs/2026-08-30-yaoguang-domain-first-rearchitecture-design.md` 的 B3、Journey、IA、Customer 360、可观测性、易用性与移动端章节。

## Global Constraints

- Automation 入场频次必须兼容两种既有逻辑：`once` 表示每个 Customer 在该 Automation 生命周期内只进入一次；`every_time` 表示每个不同 Event 均可重新进入，并允许并行 Journey instance。
- 入场保护是独立、可选、默认关闭的第三层保护，可限制冷却时间或最大并行 instance；不得改变 `once/every_time` 的含义。
- 消息频控由 B2 Frequency Policy 负责，不得在 trigger frequency 字段中混入活动级、trigger 级或全局消息次数。
- Realtime Event 以 `event_id` 幂等；Customer 以 `customer_id` 识别，Email 仅作历史兼容和展示。
- 保留现有审批字段和行为，本阶段不增加审批流、不强制审批。
- 一级导航固定为：工作台、客户、客群、营销活动、自动化旅程、内容中心、数据分析、投递中心、设置。
- 品牌显示 logo + `瑶光营销平台` / `观心知意，循光达客`，产品说明为“面向金融科技及互联网业务的开源用户营销与客户触达平台”。
- 不修改 V49；B3 使用 V50，并覆盖全新 Workspace 初始化。
- 所有主要流程在 375px、768px、1440px 三种 viewport 验收；不允许横向页面滚动。

---

### Task 1: 固化 Automation 入场频次 contract 和 V50 Journey schema

**Files:**

- Modify: `internal/domain/automation.go`
- Modify: `internal/domain/automation_test.go`
- Create: `internal/domain/journey.go`
- Create: `internal/domain/journey_test.go`
- Create: `internal/database/schema/journey_tables.go`
- Create: `internal/database/schema/journey_tables_test.go`
- Create: `internal/migrations/v50.go`
- Create: `internal/migrations/v50_test.go`
- Modify: `internal/database/init.go`
- Modify: `config/config.go`
- Modify: `internal/migrations/v49_test.go`

- [ ] **Step 1: 写 `once/every_time` 语义失败测试**

固定以下 contract：`once` 的唯一性为 `(automation_id, customer_id)`，Automation version 更新或停用再启用都不允许该 Customer 再进入；`every_time` 的唯一性为 `(automation_id, customer_id, origin_event_id)`，同 event 重放不重复，不同 event 可并行。历史仅 Email enrollment 通过 Customer projection 解析。

- [ ] **Step 2: 写 entry guard 独立性测试**

定义 `JourneyEntryGuard{Enabled, Cooldown, MaxConcurrent}`，默认 `Enabled=false`。`every_time + guard off` 允许每个 event；`every_time + cooldown` 在窗口内 defer/suppress；`once + guard` 仍以 once 永久去重为先。验证它与 B2 message frequency decision 是不同对象。

- [ ] **Step 3: 运行 domain 测试确认 contract 缺失**

```powershell
go test ./internal/domain -run 'TriggerFrequency|JourneyEntry|EntryGuard' -count=1
```

- [ ] **Step 4: 实现领域模型和 schema**

创建 `journey_enrollments`、`journey_instances`、`journey_instance_events`、`journey_entry_decisions`；给 `contact_automations`、`automation_trigger_log`、`automation_trigger_bindings` 增加 `customer_id`。用部分唯一索引分别实现 once 和 every_time，不能依赖应用层先查后写。

- [ ] **Step 5: 实现 V50 回填**

从 B0 contacts.customer_id 回填历史 enrollment/trigger；无法解析的行保留 Email 且记录 reconciliation。迁移不能删除现有 `contact_automations` 或改变正在运行 instance 状态。版本提升至 50.0。

- [ ] **Step 6: 运行验证**

```powershell
go test ./internal/domain ./internal/database/schema ./internal/migrations -run 'Journey|Automation|V50' -count=1
```

- [ ] **Step 7: 提交 Task 1**

```powershell
git add internal/domain/automation.go internal/domain/automation_test.go internal/domain/journey.go internal/domain/journey_test.go internal/database/schema/journey_tables.go internal/database/schema/journey_tables_test.go internal/migrations/v50.go internal/migrations/v50_test.go internal/database/init.go config/config.go internal/migrations/v49_test.go
git commit -m "feat(journey): add v50 customer enrollment ledger"
```

### Task 2: 将 Realtime enrollment 改为 Customer/Event 幂等

**Files:**

- Modify: `internal/database/schema/automation_triggers.go`
- Modify: `internal/database/schema/automation_triggers_test.go`
- Modify: `internal/repository/automation_postgres.go`
- Modify: `internal/repository/automation_postgres_test.go`
- Modify: `internal/repository/realtime_postgres.go`
- Modify: `internal/repository/realtime_postgres_test.go`
- Modify: `internal/service/realtime_journey_worker.go`
- Modify: `internal/service/realtime_journey_worker_test.go`
- Modify: `internal/service/automation_trigger_generator.go`
- Modify: `internal/service/automation_trigger_generator_test.go`

- [ ] **Step 1: 写并发和重放失败测试**

两个 worker 同时处理同 event、同 Customer 两个不同 event、primary Email 变化、Automation 升版、暂停恢复、once 已存在历史 Email enrollment。断言 database unique constraint 决定最终 enrollment，而非内存锁。

- [ ] **Step 2: 运行测试确认当前 once 仍按 contact_email**

```powershell
go test ./internal/repository ./internal/service -run 'RealtimeJourney|AutomationTrigger|Enrollment' -count=1
```

- [ ] **Step 3: 替换触发函数输入和唯一性逻辑**

PostgreSQL function 接收 `p_customer_id/p_origin_event_id/p_frequency`；once 先 insert `(automation,customer)`；every_time insert `(automation,customer,event)`。函数返回 typed outcome：`enrolled/already_once/replayed_event/guard_deferred/guard_denied`。

- [ ] **Step 4: 接入 optional entry guard**

在成功获得 frequency dedupe reservation 后，同事务计算 active instance 和 cooldown；guard off 不做额外查询。defer 决策写 `journey_entry_decisions` 并由 scheduler 在 `retry_at` 再评估，deny 只记录不建 instance。

- [ ] **Step 5: 保持旧触发入口兼容**

旧调用只有 Email 时先通过 Customer identity resolver 获取 customer_id；无法解析时返回可重试 data-quality error，不创建 Email-only 新 enrollment。历史读取仍能展示 Email。

- [ ] **Step 6: 运行竞态测试**

```powershell
go test -race ./internal/repository ./internal/service -run 'RealtimeJourney|AutomationTrigger|Enrollment|EntryGuard' -count=1
```

- [ ] **Step 7: 提交 Task 2**

```powershell
git add internal/database/schema/automation_triggers.go internal/database/schema/automation_triggers_test.go internal/repository/automation_postgres.go internal/repository/automation_postgres_test.go internal/repository/realtime_postgres.go internal/repository/realtime_postgres_test.go internal/service/realtime_journey_worker.go internal/service/realtime_journey_worker_test.go internal/service/automation_trigger_generator.go internal/service/automation_trigger_generator_test.go
git commit -m "fix(journey): preserve once and every event semantics by customer"
```

### Task 3: 实现 Journey 激活前校验和实例追踪 API

**Files:**

- Create: `internal/service/journey_preflight_service.go`
- Create: `internal/service/journey_preflight_service_test.go`
- Create: `internal/service/journey_trace_service.go`
- Create: `internal/service/journey_trace_service_test.go`
- Modify: `internal/domain/automation.go`
- Modify: `internal/service/automation_service.go`
- Modify: `internal/service/automation_service_test.go`
- Modify: `internal/http/automation_handler.go`
- Modify: `internal/http/automation_handler_test.go`
- Create: `openapi/paths/journeys.yaml`
- Create: `openapi/schemas/journey.yaml`
- Modify: `openapi/openapi.yaml`

- [ ] **Step 1: 写 preflight 分类测试**

校验 trigger、once/every_time、entry guard、未连接节点、不可达节点、循环、模板 channel/version、Provider、变量、Audience/条件、B2 frequency policy 和 webhook secret。blocking issue 阻止 activate，warning 允许明确确认，不增加审批步骤。

- [ ] **Step 2: 写 trace 失败测试**

按 customer_id/customer_no/external_user_id/Email 查 Journey instance；返回 enrollment decision、origin event、每节点开始/完成/等待/失败时间、B1 delivery intent/attempt/receipt 链接、当前等待原因和下一次调度时间。跨 Workspace 查询必须 404。

- [ ] **Step 3: 实现 Service 与 HTTP**

新增 `POST /api/automations.preflight`、`GET /api/journeys.instances`、`GET /api/journeys.trace`。`automations.activate` 必须携带未过期 preflight hash；后端重算 blocking conditions。

- [ ] **Step 4: 补 OpenAPI 和权限清单**

Journey trace 使用 Automations read；激活使用现有 Automations write。不得要求新 approval 权限。

- [ ] **Step 5: 运行测试**

```powershell
go test ./internal/service ./internal/http -run 'JourneyPreflight|JourneyTrace|Automation.*Activate' -count=1
make openapi-lint
```

- [ ] **Step 6: 提交 Task 3**

```powershell
git add internal/service/journey_preflight_service.go internal/service/journey_preflight_service_test.go internal/service/journey_trace_service.go internal/service/journey_trace_service_test.go internal/domain/automation.go internal/service/automation_service.go internal/service/automation_service_test.go internal/http/automation_handler.go internal/http/automation_handler_test.go openapi/paths/journeys.yaml openapi/schemas/journey.yaml openapi/openapi.yaml
git commit -m "feat(journey): add activation preflight and instance tracing"
```

### Task 4: 修复 Automation 全渠道模板创建与节点校验

**Files:**

- Modify: `console/src/components/templates/TemplateSelectorInput.tsx`
- Create: `console/src/components/templates/TemplateSelectorInput.test.tsx`
- Modify: `console/src/components/templates/CreateTemplateDrawer.tsx`
- Modify: `console/src/components/automations/config/EmailConfigForm.tsx`
- Modify: `console/src/components/automations/config/ChannelConfigForm.tsx`
- Create: `console/src/components/automations/config/ChannelConfigForm.test.tsx`
- Modify: `console/src/components/automations/NodeConfigPanel.tsx`
- Modify: `internal/service/automation_node_executor.go`
- Modify: `internal/service/automation_node_executor_test.go`

- [ ] **Step 1: 写 channel 一致性失败测试**

SMS 节点只列/创建 SMS template，Push 节点只列/创建 Push template，Email 节点只列/创建 Email template；从选择器内新建或 clone 后自动选择同 channel 模板。现有 `CreateTemplateDrawer` 默认 Email 的行为必须被 channel prop 覆盖。

- [ ] **Step 2: 运行前端测试确认失败**

```powershell
Set-Location console
npm test -- --run src/components/templates/TemplateSelectorInput.test.tsx src/components/automations/config/ChannelConfigForm.test.tsx
Set-Location ..
```

- [ ] **Step 3: 让 TemplateSelector 传递 channel**

为 Create/Clone drawer 增加受控 `forceChannel`，selector 的 list/get/create/clone 共用相同 channel query key。切换 nodeType 时清除不匹配的 selected template，而不是保留错误 ID。

- [ ] **Step 4: 增加后端最终校验**

保存和执行节点都验证 template.channel == node channel、template version 存在、integration type 匹配；错误返回节点 ID、字段和可操作说明。

- [ ] **Step 5: 运行测试**

```powershell
go test ./internal/service -run 'AutomationNode.*Channel|TemplateChannel' -count=1
Set-Location console
npm test -- --run src/components/templates/TemplateSelectorInput.test.tsx src/components/automations/config/ChannelConfigForm.test.tsx
Set-Location ..
```

- [ ] **Step 6: 提交 Task 4**

```powershell
git add console/src/components/templates/TemplateSelectorInput.tsx console/src/components/templates/TemplateSelectorInput.test.tsx console/src/components/templates/CreateTemplateDrawer.tsx console/src/components/automations/config/EmailConfigForm.tsx console/src/components/automations/config/ChannelConfigForm.tsx console/src/components/automations/config/ChannelConfigForm.test.tsx console/src/components/automations/NodeConfigPanel.tsx internal/service/automation_node_executor.go internal/service/automation_node_executor_test.go
git commit -m "fix(journey): create templates in the selected channel"
```

### Task 5: 重构一级导航、品牌壳和响应式布局

**Files:**

- Modify: `console/src/layouts/WorkspaceLayout.tsx`
- Modify: `console/src/__tests__/WorkspaceLayout.test.tsx`
- Modify: `console/src/router.tsx`
- Modify: `console/src/index.css`
- Modify: `console/src/App.tsx`
- Modify: `console/index.html`
- Create: `console/src/components/brand/YaoguangBrand.tsx`
- Create: `console/src/components/brand/YaoguangBrand.test.tsx`
- Add: `console/public/hengshucredit_animated.svg`
- Modify: `console/src/i18n/catalogInventory.ts`
- Modify: `console/src/i18n/locales/zh-CN.po`

- [ ] **Step 1: 写导航映射与移动端失败测试**

固定一级导航和旧路由映射：Dashboard→工作台，Contacts/Customer→客户，Lists/Segments/Audience→客群，Broadcast→营销活动，Automation→自动化旅程，Templates/File Manager→内容中心，Analytics/Web Analytics→数据分析，Logs/Delivery→投递中心，其余 Workspace 配置→设置。旧 URL 继续可访问且高亮正确入口。

- [ ] **Step 2: 写品牌 contract 测试**

左上角渲染 Hengshu animated SVG、`瑶光营销平台`、`观心知意，循光达客`；折叠态保留可访问名称和 logo；登录页/标题使用 `Yaoguang Marketing`，不得残留用户可见 Notifuse 品牌。

- [ ] **Step 3: 实现导航和品牌组件**

大屏使用 9 个一级入口；同域子页面用页面内 tabs，不再把 debug 页面暴露为主导航。保留旧 route aliases。Settings 权限不足时隐藏对应页但不移动其他菜单顺序。

- [ ] **Step 4: 修复响应式 shell**

`<768px` sidebar 变为 overlay Drawer，不占主内容宽度；内容容器 `min-width:0`，表格使用自身 horizontal scroll；header actions 可换行。加入 viewport tests 断言 `document.documentElement.scrollWidth <= innerWidth`。

- [ ] **Step 5: 运行前端测试**

```powershell
Set-Location console
npm test -- --run src/__tests__/WorkspaceLayout.test.tsx src/components/brand/YaoguangBrand.test.tsx src/__tests__/webAnalyticsRoutes.test.tsx
npm run build
Set-Location ..
```

- [ ] **Step 6: 提交 Task 5**

```powershell
git add console/src/layouts/WorkspaceLayout.tsx console/src/__tests__/WorkspaceLayout.test.tsx console/src/router.tsx console/src/index.css console/src/App.tsx console/index.html console/src/components/brand/YaoguangBrand.tsx console/src/components/brand/YaoguangBrand.test.tsx console/public/hengshucredit_animated.svg console/src/i18n/catalogInventory.ts console/src/i18n/locales/zh-CN.po
git commit -m "feat(console): apply yaoguang navigation and responsive brand shell"
```

### Task 6: 上线模板化 Journey 创建和可理解的频次配置

**Files:**

- Create: `console/src/components/automations/JourneyCreateWizard.tsx`
- Create: `console/src/components/automations/JourneyCreateWizard.test.tsx`
- Create: `console/src/components/automations/JourneyPreflightPanel.tsx`
- Create: `console/src/components/automations/JourneyPreflightPanel.test.tsx`
- Modify: `console/src/components/automations/UpsertAutomationDrawer.tsx`
- Modify: `console/src/components/automations/config/TriggerConfigForm.tsx`
- Modify: `console/src/components/automations/config/TriggerConfigForm.test.tsx`
- Modify: `console/src/components/automations/AutomationFlowEditor.tsx`
- Modify: `console/src/services/api/automation.ts`
- Modify: `console/src/pages/AutomationsPage.tsx`

- [ ] **Step 1: 写向导和频次文案失败测试**

新建 Journey 先选目标模板：事件欢迎、定时唤醒、流失召回、生日关怀、名单通知、空白流程。Trigger frequency 用两张业务卡准确显示：`每个联系人一次/每个联系人只进入一次自动化`、`每次/每次事件发生时联系人都会重新进入`。不把频控次数放在这两张卡中。

- [ ] **Step 2: 写 entry guard 展开测试**

默认显示“入场保护：关闭”；展开后可设冷却和最大并行，旁边明确说明它只限制 Journey 入场，不等于消息频控。B2 trigger frequency policy 在独立“触达频控”步骤配置。

- [ ] **Step 3: 实现 Journey Create Wizard**

步骤为目标与模板 → 触发条件 → 入场频次/保护 → 流程内容 → 触达频控 → 激活前检查。每一步保存 draft，关闭后可继续；高级 flow editor 保留完整能力。

- [ ] **Step 4: 接入激活 preflight**

激活按钮打开 JourneyPreflightPanel；blocking issue 点击可定位到具体 node/field；warning 需确认；不显示审批人或审批状态的新流程。

- [ ] **Step 5: 运行测试**

```powershell
Set-Location console
npm test -- --run src/components/automations/JourneyCreateWizard.test.tsx src/components/automations/JourneyPreflightPanel.test.tsx src/components/automations/config/TriggerConfigForm.test.tsx src/pages/AutomationsPage.test.tsx
Set-Location ..
```

- [ ] **Step 6: 提交 Task 6**

```powershell
git add console/src/components/automations/JourneyCreateWizard.tsx console/src/components/automations/JourneyCreateWizard.test.tsx console/src/components/automations/JourneyPreflightPanel.tsx console/src/components/automations/JourneyPreflightPanel.test.tsx console/src/components/automations/UpsertAutomationDrawer.tsx console/src/components/automations/config/TriggerConfigForm.tsx console/src/components/automations/config/TriggerConfigForm.test.tsx console/src/components/automations/AutomationFlowEditor.tsx console/src/services/api/automation.ts console/src/pages/AutomationsPage.tsx
git commit -m "feat(journey): add template first creation and preflight"
```

### Task 7: 完成 Customer 360、Journey Trace 和投递中心

**Files:**

- Modify: `console/src/components/customers/CustomerDrawer.tsx`
- Modify: `console/src/components/customers/CustomerDrawer.test.tsx`
- Create: `console/src/components/customers/CustomerTimelineTab.tsx`
- Create: `console/src/components/customers/CustomerJourneysTab.tsx`
- Create: `console/src/components/customers/CustomerDeliveriesTab.tsx`
- Create: `console/src/pages/DeliveryCenterPage.tsx`
- Create: `console/src/pages/DeliveryCenterPage.test.tsx`
- Create: `console/src/components/automations/JourneyTraceDrawer.tsx`
- Create: `console/src/components/automations/JourneyTraceDrawer.test.tsx`
- Modify: `console/src/services/api/automation.ts`
- Modify: `console/src/services/api/delivery.ts`
- Modify: `console/src/router.tsx`

- [ ] **Step 1: 写 360 和 trace 失败测试**

Customer 360 tabs 为概览、身份与同意、客群、行为时间线、Journey、投递；从 Journey/Delivery 可返回同一 Customer。Trace 用按时间排序的节点状态展示，默认解释“为什么等待/为什么未发送”，原始 JSON 放在诊断折叠区。

- [ ] **Step 2: 实现 Customer 360 聚合查询**

各 tab 独立 query 和错误边界，打开 Drawer 不一次拉取全部历史。Identity 只显示 hint；merged customer 自动定位 target 且保留来源提示。

- [ ] **Step 3: 实现投递中心**

筛选 channel、source、status、provider、时间、Customer；默认突出 unknown/terminal_failed/reconciliation backlog。只有有权限用户可执行 B1 unknown 处置，操作必须填写原因。

- [ ] **Step 4: 实现 Journey Trace Drawer**

连接 origin event、entry decision、instance nodes 和 delivery intents；节点错误提供可复制 trace ID 和修复入口，不要求用户理解 queue/lease 术语。

- [ ] **Step 5: 运行测试**

```powershell
Set-Location console
npm test -- --run src/components/customers/CustomerDrawer.test.tsx src/pages/DeliveryCenterPage.test.tsx src/components/automations/JourneyTraceDrawer.test.tsx
Set-Location ..
```

- [ ] **Step 6: 提交 Task 7**

```powershell
git add console/src/components/customers/CustomerDrawer.tsx console/src/components/customers/CustomerDrawer.test.tsx console/src/components/customers/CustomerTimelineTab.tsx console/src/components/customers/CustomerJourneysTab.tsx console/src/components/customers/CustomerDeliveriesTab.tsx console/src/pages/DeliveryCenterPage.tsx console/src/pages/DeliveryCenterPage.test.tsx console/src/components/automations/JourneyTraceDrawer.tsx console/src/components/automations/JourneyTraceDrawer.test.tsx console/src/services/api/automation.ts console/src/services/api/delivery.ts console/src/router.tsx
git commit -m "feat(console): connect customer journey and delivery traces"
```

### Task 8: 统一中文术语、错误反馈和无障碍

**Files:**

- Modify: `console/src/i18n/catalogInventory.ts`
- Modify: `console/src/i18n/locales/zh-CN.po`
- Modify: `console/src/i18n/locales/zh-CN.js`
- Modify: `console/src/i18n/zhCN.priority.test.ts`
- Create: `console/src/components/errors/ActionableError.tsx`
- Create: `console/src/components/errors/ActionableError.test.tsx`
- Modify: `console/src/services/api/errors.ts`
- Modify: `console/src/index.css`

- [ ] **Step 1: 写术语 contract 测试**

核心术语固定：Customer=客户、Audience=客群、Campaign=营销活动、Journey=自动化旅程、Delivery=投递、Identity=身份标识、Consent=授权同意、Suppression=触达抑制、Frequency Cap=频控。页面中不得混用联系人/客户表达同一 Customer authority；仅 Contact 兼容页保留“联系人”。

- [ ] **Step 2: 实现 ActionableError**

统一展示发生了什么、影响范围、可重试动作、修复入口、trace ID；表单 field errors 定位到字段。网络失败、409 conflict、429/频控、503/defer、unknown delivery 分别使用业务文案。

- [ ] **Step 3: 完成键盘和屏幕阅读器支持**

Drawer/Modal focus trap、关闭后焦点归位；Journey canvas 提供可键盘访问的节点列表替代视图；状态不只靠颜色；所有 icon button 有 aria-label。

- [ ] **Step 4: 重新提取/编译 Lingui catalog**

```powershell
Set-Location console
npm run lingui:extract
npm run lingui:compile
npm test -- --run src/i18n/zhCN.priority.test.ts src/components/errors/ActionableError.test.tsx
Set-Location ..
```

- [ ] **Step 5: 提交 Task 8**

```powershell
git add console/src/i18n/catalogInventory.ts console/src/i18n/locales/zh-CN.po console/src/i18n/locales/zh-CN.js console/src/i18n/zhCN.priority.test.ts console/src/components/errors/ActionableError.tsx console/src/components/errors/ActionableError.test.tsx console/src/services/api/errors.ts console/src/index.css
git commit -m "fix(console): standardize chinese ux and actionable errors"
```

### Task 9: B3 E2E、移动端、性能与最终验收

**Files:**

- Create: `console/e2e/marketing-journey.spec.ts`
- Create: `console/e2e/marketing-mobile.spec.ts`
- Create: `tests/integration/journey_frequency_semantics_test.go`
- Create: `tests/integration/journey_trace_delivery_test.go`
- Create: `docs/operations/journey-frequency-semantics.md`
- Create: `docs/operations/evidence/b3-journey-console.md`

- [ ] **Step 1: 实现核心 E2E**

业务用户完成：导入客户 → 创建客群 → 创建 Campaign → preflight → 发送 → 查看投递；创建 event Journey → 选择 `once` → 触发两次只入场一次；改为新 Automation `every_time` → 两个 event 建两次 instance；配置 trigger/global 频控后第二条消息 suppressed 但 enrollment 仍存在。

- [ ] **Step 2: 实现移动端 E2E**

在 375×812 和 768×1024 检查九个一级入口、Customer list/360、Audience builder、Campaign preflight、Journey wizard、Delivery center；每页断言无 document 横向滚动，主要按钮可见且触控区域至少 44px。

- [ ] **Step 3: 运行后端语义测试**

```powershell
$env:INTEGRATION_TESTS='true'; go test -tags integration -timeout 20m ./tests/integration -run 'JourneyFrequencySemantics|JourneyTraceDelivery' -count=1; Remove-Item Env:INTEGRATION_TESTS
```

- [ ] **Step 4: 运行前端全量门禁**

```powershell
Set-Location console
npm test -- --run
npm run lint
npm run build
npm run test:e2e -- marketing-journey.spec.ts marketing-mobile.spec.ts
Set-Location ..
```

- [ ] **Step 5: 验证实时性能目标**

参考 8c/16GB app、独立 PostgreSQL/Redis/RabbitMQ，压测单节点 500 events/s；记录 event accept p95 < 200ms、event-to-intent p95 < 5s、Customer write p95 < 200ms。任何未达目标项目记录 profile、瓶颈和修复 commit，不能只放宽阈值。

- [ ] **Step 6: 编写频次语义 runbook 与验收证据**

用三个并列示例解释 Automation frequency、entry guard、message frequency cap，记录数据库唯一键、API payload、UI 截图路径、E2E 退出码、移动端 scrollWidth 和性能结果。

- [ ] **Step 7: 提交 B3 证据**

```powershell
git add console/e2e/marketing-journey.spec.ts console/e2e/marketing-mobile.spec.ts tests/integration/journey_frequency_semantics_test.go tests/integration/journey_trace_delivery_test.go docs/operations/journey-frequency-semantics.md docs/operations/evidence/b3-journey-console.md
git commit -m "test(journey): prove b3 semantics ux and performance"
```
