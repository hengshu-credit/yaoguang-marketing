# Configurable Marketing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** 实现用户可配置的业务对象、客户属性、事件接入、多业务状态、每次营销执行快照，以及拖拽漏斗、分支总览和转化解释链。

**Architecture:** 复用 Customer UUID、Workspace 数据库、event_ledger/outbox 和现有 Campaign/Journey。配置目录使用类型化 JSON 定义与统一存储，事件发生写入既有账本。Campaign 当前配置可编辑，每次运行保存完整快照；分析读取可重建投影和历史状态。

**Tech Stack:** Go 1.25.14（go.mod 最低 1.25.4）、PostgreSQL 17、已有 ClickHouse 投影、React 18、TypeScript、Ant Design、Lingui、dnd-kit、XYFlow、Vitest、Go test/sqlmock、Playwright。

**Spec:** [完整设计](../specs/2026-09-07-configurable-customer-states-events-funnels-design.md)。用户已明确“开始开发并验证”，不在任务之间重复请求实施许可。

## 实施结果

本计划已执行并完成验收。最终接口、结果及有界支持范围见[验收记录](../reports/2026-09-07-configurable-marketing-validation.md)，操作与上报方式见[接入说明](../../guides/configurable-marketing.md)。下列条目依据最终实现调整措辞并记录完成状态；没有将通用事件撤回协议或全量 ClickHouse 生产读源列为已实现。

## Global Constraints

- Customer UUID 是客户唯一主键；external_user_id 支持定位、身份归集与合并。
- 授信申请、借款订单等是用户配置的对象，禁止固定业务类型枚举或专用申请/借款表。
- Campaign 不做版本管理；每次活动及自动化执行完整保存配置、条件、素材和执行证据。
- 默认与自定义事件统一管理；规范化字段为 report_id、customer_attributes、event_code、event_name（可选）、data、reported_at（平台自动生成）、triggered_at、expires_at。
- data 支持映射客户属性；识别、绑定身份、更新画像与仅记录分别配置。
- 事件过期不抹除历史；重复上报不刷新接收时间，不重复写画像或触发营销。
- 每个 Workspace 独立；后端鉴权、类型校验、字段权限及参数化 SQL 不依赖前端约束。
- 每个 UI 字符串从首次实现进入 Lingui，英文自然语言源 key、简体中文翻译、en/fr/es/de/ca/pt-BR/ja/it/zh-CN 全目录覆盖。
- 不删除旧 API、不覆盖用户原有修改、不部署到共享环境。新功能在隔离工作区与测试数据库验证。
- 当前基线为 738bc6dc、config.VERSION=56.0；新增迁移从 V57 顺序分配，只有集成人修改注册/初始化和版本号。

## 工作区和验证命令

代码：`E:/workspace/notifuse-worktrees/configurable-marketing`，分支 `codex/configurable-marketing`。

独立工具容器已挂载此工作区：`notifuse-configurable-go`、`notifuse-configurable-node`。它们不运行原开发服务的热更新。

```powershell
docker exec notifuse-configurable-go /usr/local/go/bin/go test ./internal/domain ./internal/database/schema -count=1
docker exec notifuse-configurable-go /usr/local/go/bin/go test ./internal/repository ./internal/service ./internal/http -run 'Business|TrackedEvent|ExecutionSnapshot|Funnel|SubjectState' -count=1
docker exec -w /workspace/console notifuse-configurable-node npm test -- --run
docker exec -w /workspace/console notifuse-configurable-node npm run build
docker exec -w /workspace/console notifuse-configurable-node npm run lint
```

集成测试使用独立 Docker 网络中的 `notifuse-configurable-postgres` 与 `notifuse-configurable-clickhouse`，不发布数据库主机端口，避免与共享开发服务冲突。设置 `INTEGRATION_TESTS=true` 且 `-tags integration`。既有测试助手可能跳过未启用测试，必须检查执行结果，不能将 SKIP 计为通过。原开发业务数据库不用于清理/重建测试。

## 分工与依赖

| 任务包 | 文件边界 | 输出 | 依赖 |
| --- | --- | --- | --- |
| A：配置目录、客户识别与事件接入 | 新 business_config/ tracked_event/ business_object 后端文件、客户合并与 SDK | 通用目录、无邮箱事件接入、对象关联 | 既有 Customer/ledger |
| B：运行快照 | execution_snapshot 文件及 Campaign/Journey/Delivery 接入、运行详情组件 | 当前配置执行与完整历史快照 | 既有执行服务；tracking_plan 用 JSON 嵌入 |
| C：状态与漏斗 | subject_state/ funnel 后端与对应页面 | 独立状态、规则、漏斗与路径计算/交互 | 既有 ledger；配置通过 A 的接口读取 |
| D：集成与配置 UI | app.go、migrations/init/version、router/settings/permissions、business 配置组件、全部目录翻译 | 可操作的全链路、统一鉴权与回归 | A/B/C 的已确定接口 |

只有 D 修改共享 app.go、config/config.go、internal/database/init.go、权限资源、router.tsx、SettingsSidebar、WorkspaceSettingsPage、WorkspaceSectionTabs 和翻译目录。其他任务提供共享接入清单，由 D 串行接入。实现者不相互回滚修改，不在未复核的共享工作区提交整个目录。

## 跨任务接口合同

新接口名称最终必须在实现报告中列明，集成人以实现导出的签名接入，不凭空补出第二套同名服务。

配置目录采用 `kind + key` 标识，kind 允许 `customer_attribute`、`object_type`、`object_relation`、`event`、`event_mapping`、`state`、`funnel`。**这是配置类别，不是业务对象类型枚举。** 定义 document 包含 id、kind、key、name、description、schema、settings、status、revision、created_at、updated_at。schema/settings 为 JSON object，revision 仅用于定义解释及并发控制，不用作 Campaign 版本。

```json
{
  "kind": "event",
  "key": "contract.signed",
  "name": "合同签署",
  "schema": {"type":"object","required":["contract_id"],"properties":{"contract_id":{"type":"string"}}},
  "settings": {"subject_type":"contract","subject_id_path":"$.data.contract_id","expiry_mode":"duration","ttl_seconds":86400},
  "status": "active"
}
```

配置服务提供 list/get/save/archive；sample preview 纯读。状态与漏斗定义使用同一目录的存储协议，计算逻辑分别由 C 管理。A 导出 Go 配置 repository/service、事件 repository/service/handler 构造函数及 schema DDL 函数，在报告中附签名。A 与 C 在开工时先对齐接口，不共享实现文件。

统一事件执行顺序：外部 report_id 去重 → 固化首次映射 → 映射/类型验证 → 客户/对象解析 → 账本与 outbox 同事务 → 映射/状态投影 → 依赖状态的触发；过期/回补不能产生实时发送。

事件解析后 customer_id 写入既有 ledger 列；主对象使用 subject_type/subject_id，原 report_id、data、定义修订、triggered_at/reported_at/expires_at、触发身份摘要保存在稳定 envelope 内。报告与消费者不得各自重新解释外部 JSON。

## Task A1：配置与对象目录

**Files:** 新增 `internal/domain/business_config.go`、`internal/repository/business_config_postgres.go`、`internal/service/business_config_service.go`、`internal/http/business_config_handler.go`、`internal/database/schema/business_config.go` 及对应测试；对象运行逻辑拆入 `business_object` 文件。修改客户合并时限定 `internal/repository/customer_postgres.go`、`internal/service/customer_service.go` 的相关路径。

**Interfaces:** 目录提供 kind/key/revision 的 workspace 内 CRUD；对象按动态 object_type_id/source/external_key 创建与查找，最终 customer_id 始终为 UUID。

- [x] 先建立定义与唯一键失败测试：任意 `contract` 类型可保存；同 key 同 kind 冲突；不同 Workspace 不冲突；非法 schema、关系循环、基数冲突拒绝。
- [x] 运行 `go test ./internal/domain ./internal/repository -run BusinessConfig -count=1`，确认缺少能力导致失败。
- [x] 实现目录、对象实例/关联及 PostgreSQL DDL，用户增加对象类型不执行 CREATE TABLE。
- [x] 默认事件目录通过幂等种子安装，内置投递语义不能被自定义覆盖；旧字段显示名称迁移为兼容属性定义，不继续限定只有五个自定义槽位。
- [x] 增加服务鉴权、分页/筛选、发布引用校验、归档与样例预览，明确缺失、冲突和权限错误。
- [x] 运行单元与持久化测试，报告变更文件和接口供 D 接入。

验收输入必须含非信贷对象：

```json
{"kind":"object_type","key":"contract","name":"合同","schema":{"type":"object","properties":{"contract_no":{"type":"string"}}},"settings":{"identity_fields":["contract_no"]},"status":"active"}
```

## Task A2：客户属性解析、映射与幂等上报

**Files:** 新增 `internal/domain/tracked_event.go`、`internal/service/tracked_event_service.go`、`internal/repository/tracked_event_postgres.go`、`internal/http/tracked_event_handler.go`、`internal/database/schema/tracked_events.go`、客户解析/字段映射单职责文件及测试。复用 `internal/repository/realtime_postgres.go` 的账本事务逻辑。

**Interfaces:** track/batch 接收规范化事件或已配置外部映射 JSON；返回 report_id、event_id、customer_id、reported_at、duplicate、processing_status。同请求重试返回首次结果。

- [x] 写必选字段、缺省名称、expiry 模式、嵌套字段映射、敏感属性、重复冲突测试。
- [x] 在真实 PG 测试中并发提交同一来源上报 ID，断言 receipt/ledger/outbox 各一条；改变 payload 返回冲突。
- [x] 实现按配置解析邮箱/手机号/证件/外部 ID，多个属性一致时解析到同一 UUID，不一致记录冲突；接入策略明确未命中时的创建/待关联/拒绝行为。
- [x] 为外部用户 ID 合并补别名解析与一致性，旧 UUID/外部 ID 不丢历史。
- [x] data 到客户属性动作分为 lookup/bind/update/record；定义明确清空、集合与覆盖规则，事件失败不能留下半个画像更新。
- [x] 使用统一 ledger 事务写入，内部 UUID 与外部 report_id 分开；固化时间和映射，重试不因当前配置变化重新解释。
- [x] 增加过期、排队到期、补数保护和定义修订固化；批量逐项结果不静默漏项。
- [x] 单元、SQL 并发与 HTTP 集成测试通过后报告可接入构造函数。

核心验收事件：

```json
{"report_id":"contract-sign-1","event_code":"contract.signed","customer_attributes":{"external_user_id":"customer-1"},"data":{"contract_id":"C-1"},"triggered_at":"2026-09-07T10:00:00+08:00","expires_at":"2026-09-08T10:00:00+08:00"}
```

## Task A3：浏览器/外部事件协议与 Customer 360 读模型

**Files:** 修改 `web_analytics_sdk/src/sdk.ts`、`index.ts`、`types.ts`、现有 transport；新增相应事件队列/identity 测试；提供客户事件/对象列表 API；新增 `openapi/paths/tracked-events.yaml` 与 schemas。

- [x] 写 SDK 真实队列重试测试，同一行为三次重传保持 report_id，下一次真实行为产生新 ID。
- [x] 添加 trackEvent 与受限 identify 凭据；匿名会话有证据才关联客户，触点 token 不冒充客户本人。
- [x] 配置只允许浏览器上报明确开放事件；后端事实事件不能由公开接入伪造。
- [x] 旧 identify(email,hmac)、trackGoal、custom_events.upsert 保持兼容，不把资源快照当每次发生。
- [x] 输出 Customer 360 事件/对象读模型，时间、状态、映射结果和敏感字段权限完整。
- [x] 运行 SDK 单元、类型检查、回归；接口文档样例对应真实 handler。

## Task B1：完整执行快照存储与素材归档

**Files:** 新增 `internal/domain/execution_snapshot.go`、`internal/repository/execution_snapshot_postgres.go`、`internal/service/execution_snapshot_service.go`、`internal/database/schema/execution_snapshots.go`、对应测试及 `openapi/paths/execution-snapshots.yaml`。

**Interfaces:** 配置快照可按 run_id/instance_id 读取；保存当时配置、筛选条件、依赖定义、模板/翻译/素材、归档地址及哈希。节点求值与渲染内容为追加证据。

- [x] 保存原配置/正文，修改原对象后再读旧快照，断言内容不变；跨 Workspace 读取拒绝。
- [x] 实现不可变快照与每运行绑定，JSON 深拷贝/序列化不引用可变模板对象；支持的媒体在发送前归档，不支持的来源明确拒绝。
- [x] 新增节点和投递实际内容证据，尚未执行不能显示为已发送；敏感参数按授权读取，公开快照不返回密钥，内部执行凭据单独加密归档。
- [x] 增加持久化和失败恢复测试，模板删除后仍能还原已归档内容。

```json
{"run_id":"run-1","kind":"campaign","configuration":{"filters":{"product":"A"}},"materials":[{"template_id":"T-1","content":{"subject":"Original"}}]}
```

## Task B2：Campaign/Journey/Delivery 使用运行快照

**Files:** 修改 `internal/domain/campaign.go`、`internal/service/campaign_service.go`、`campaign_snapshot_service.go`、`campaign_snapshot_worker.go`、`internal/repository/campaign_postgres.go`、`internal/http/campaign_handler.go`；Journey/Delivery 相关接入限于快照绑定和证据，不修改 C 的规则计算。

- [x] 写无需 campaign version 参数即可开始运行的失败测试；两次运行前修改条件/素材，各自历史保留对应内容。
- [x] 当前 Campaign 配置直接保存；Start 创建 run+snapshot 的原子绑定，旧 version 请求仅兼容，新的执行路径不创建 CampaignVersion。
- [x] 立即/定时/周期执行统一入口；暂停恢复使用同 run，重新执行生成新 run。
- [x] 自动化每次入场/匹配判断记录准确配置，等待实例不会读到后来的流程变更；重复入场和发送副作用保持原幂等。
- [x] 发送前动态资格检查、选中变体、渲染变量和最终载荷写入追加证据。
- [x] 测试重试、配置编辑并发、旧任务迁移及运行中恢复；旧历史不足的字段明确显示缺失。

## Task B3：运行详情 UI

**Files:** 新增 `console/src/components/executions/ExecutionDetails.tsx`、API 类型与测试；修改 `console/src/components/broadcasts/BroadcastStats.tsx`、`console/src/components/customers/CustomerJourneysTab.tsx` 相关入口。

- [x] 渲染两次不同配置的 fixture，切换 run 时展示对应筛选/正文/结果，不显示当前模板替代历史。
- [x] 实现配置、筛选与人群、素材、执行时间线、转化结果视图，条件与正文可直接展开查看。
- [x] 自动化节点下钻显示当时参数、命中、等待、超时、重试与实际发送；接入权限错误与历史缺失状态。
- [x] 所有文案用 Lingui 宏；翻译 key 清单交 D 统一提取，避免多进程覆盖目录。
- [x] Vitest 与类型检查通过，提供路由挂载点给 D，由 D 验证应用集成。

## Task C1：通用状态与规则计算

**Files:** 新增 `internal/domain/subject_state.go`、`internal/service/subject_state_service.go`、`internal/repository/subject_state_postgres.go`、`internal/database/schema/subject_states.go`、对应 handler/worker/test；状态定义读取统一目录。

**Interfaces:** 对象引用为 customer 或任意已配置 type+id；state current/history 带 value、valid_from/to、规则/投影修订与来源。A 完成事件接入后，通过稳定 ledger 事件触发投影。

- [x] 编写已实名+会员有效同时存在、全部未知、手工覆盖、组内单选冲突的测试。
- [x] 实现事件/属性/窗口/关联对象聚合规则，类型化条件树，不将当前状态字符串替换为唯一阶段。
- [x] 状态历史保持左闭右开且不重叠；并发/迟到按对象重算，定时到期不依赖新事件。
- [x] 写两笔借款均逾期、还清其中一笔仍存在逾期的通用对象聚合测试；类型名替换成其他业务对象仍成立。
- [x] 衔接状态进入/退出与客群，回放不触发发送；状态依赖图拒绝循环。
- [x] 增加当前/历史 API、手工 set 和原因审计，越权写入拒绝。

## Task C2：任务条件、对象等待与漏斗计算

**Files:** 新增 `internal/domain/funnel.go`、`internal/service/funnel_service.go`、`internal/repository/funnel_postgres.go`、`internal/http/funnel_handler.go`、对应测试；新增通用条件/关联助手，调整 Journey 规则接入时先与 B 对齐文件边界。

**Interfaces:** query 输入 saved definition 或 preview definition + 时间/水位/filters；输出每步对象数、逐步/累计率、pending/expired、解释链与投影水位。

- [x] 先写如下确定性事件测试：C1 的申请 A 在第1步，申请 B 在第2步，按申请统计转化=0；同一对象较早 A 超窗、后来 A→B 有效，转化=1。
- [x] 实现多候选起点顺序匹配、对象一致性、明确窗口、同时刻策略、无序完成组、OR/可选路径；不得仅按每类 MIN 时间拼接。
- [x] 状态过滤查询事件时点有效区间；成熟与等待队列分开，零分母返回空率；分支交集/并集明确。
- [x] 支持客户、任意配置对象及关联子链的计数，不混合人/申请/订单单位；金额按交易/分配和币种去重。
- [x] 从 PG 参数化读取并支持异步查询/取消/明细；扩展现有 ClickHouse 投影并对比同定义/水位结果。
- [x] 任务自选跟踪项不会关闭其他事件采集；事件/状态触发、同对象等待、每对象一次、过期/回补/循环保护使用统一条件。
- [x] 注册事件关联触点、直接关联和末次有效点击归因；无归因照常纳入全站分析，不将所有后续行为强归活动。

测试序列：

```json
{"window_seconds":3600,"events":[{"subject_id":"A","code":"start","at":0},{"subject_id":"B","code":"finish","at":10},{"subject_id":"C","code":"start","at":0},{"subject_id":"C","code":"start","at":7200},{"subject_id":"C","code":"finish","at":7210}],"expected":{"entered":2,"converted":1}}
```

## Task C3：状态和拖拽分析 UI

**Files:** 新增 `console/src/components/business_states/`、`console/src/components/funnels/`、`console/src/pages/FunnelsPage.tsx`、对应 API 和测试；Customer 状态入口由 D 集成。

- [x] 用真实配置对象 fixture 渲染选择器，contract 类型无需代码枚举即可选择。
- [x] 实现状态配置表单、规则条件、来源历史和有效期；独立状态以多卡片展示，不强制互斥。
- [x] 使用已安装 dnd-kit 实现步骤拖入/排序/复制/删除和键盘替代操作，保存重开配置不丢失。
- [x] 实现 OR 分支、完成组、可选路径和全链路总览；状态覆盖单独展示，不能伪装漏斗。
- [x] 结果提供窗口、对象、归因、水位、pending、流失、人数/金额和解释链；请求失效/权限/取消有正确反馈。
- [x] 默认信贷模板作为可编辑配置，与非信贷合同 fixture 同样工作；所有文字经 Lingui。
- [x] 完成组件测试并交 D 接入路由/导航/目录翻译。

## Task D1：统一接入、迁移与配置控制台

**Files:** 修改 `internal/app/app.go`、`config/config.go`、`internal/database/init.go`、`internal/domain/workspace.go`、权限配套；新增 `internal/migrations/v57.go` 起的增量迁移与测试。新增 `console/src/components/business/` 配置页面、`console/src/services/api/business.ts`，修改 router/settings/CustomerDrawer；OpenAPI 主文件同步注册。

- [x] 预先记录 A/B/C 导出构造函数、DDL 与权限资源；只有本任务更新共享入口。
- [x] 先写新旧 workspace schema 一致、升级可重复执行及版本注册测试；按功能已就绪顺序添加迁移，不修改旧 V56 及之前迁移。
- [x] 接入目录、事件、对象、状态、快照、分析 handler/worker；Workspace 权限默认策略继承已有相关资源，不扩大普通 token 权限。
- [x] 实现客户属性、业务对象、事件、外部字段映射五类表单/目录/样例预览；数据结构使用 API 返回值，不能只做本地假保存。
- [x] Customer 360 接入事件、身份、对象、状态和运行记录；客户合并后的查询统一 UUID 解析。
- [x] 整合 B/C 页面与任务跟踪表单，导航/页面标题遵守现有骨架，目录就地引用。
- [x] 统一 Lingui extract，逐项简体中文翻译，所有现有语言目录覆盖，执行 compile/build/lint。

## Task D2：端到端验收、迁移对账与交付

**Files:** 新增 `tests/integration/configurable_marketing_test.go`、`console/e2e/features/configurable-marketing.spec.ts`、测试夹具和 `docs/` 接入说明；完善客户删除/投影回补路径及测试。

- [x] 在隔离数据库安装最新 schema；验证从原 schema 升级，不触碰原本地业务数据。
- [x] 真实 HTTP 创建并验证 contract 对象、属性映射、客户时间线和状态；浏览器完成事件字段配置、样例校验、保存与分析页面闭环。
- [x] 同一客户邮箱/手机号上报去重/关联，外部 ID 合并，多对象反例、过期/重试/权限反例完整执行。
- [x] 运行活动/自动化，修改配置/素材再运行，查看两次历史完整且独立；暂停、恢复、重复事件不重复触达。
- [x] 拖拽保存漏斗并验证 fixture 结果，含 OR 重叠、观察期、跨对象、属性过滤、归因、实际事件时刻状态。
- [x] 旧 custom_events、Customer、Broadcast、Journey、投递、订阅单元回归与相关真实集成验收；客户删除后原始/投影数据不可重新关联。
- [x] 针对新 API/状态/漏斗做独立 PG 持久化、并发恢复与 PG/CH 重复投影样本对账，验证查询边界和迟到修订；默认分析读源保留 PG。
- [x] 汇总单测、真实集成、前端构建、浏览器证据与所有未解决问题；完成独立代码审查并修复，再提交可审阅变更。

## 需求覆盖与完成门槛

| Spec 章节 | 落地任务 |
| --- | --- |
| 4 客户/对象/匿名识别 | A1/A2/A3/D1/D2 |
| 5 默认/自定义事件、字段映射、时效 | A1/A2/A3/D1/D2 |
| 6 多状态、历史与规则 | C1/C2/C3/D2 |
| 7 运行快照、任务跟踪和触发 | B1/B2/B3/C2/D1/D2 |
| 8 漏斗、分支、对象口径与归因 | C2/C3/D2 |
| 9 可配置信贷示例 | A1/C3/D2 |
| 10 存储/可靠性/接口 | A2/B1/C1/C2/D1/D2 |
| 11 页面/权限/Lingui | B3/C3/D1/D2 |
| 12 兼容迁移及删除 | A2/B2/D1/D2 |
| 13 全部验收样本 | D2 汇总并逐项记录证据 |

部分页面、顺序漏斗或单个 API 完成均不能作为整体完成。每个任务先观察失败测试，再实现并验证；实现报告包含接口、测试命令/结果及限制。集成人在共享文件接入后再次验证真实闭环。
