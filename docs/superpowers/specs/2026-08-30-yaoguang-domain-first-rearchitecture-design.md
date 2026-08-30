# 瑶光营销平台领域模型优先重构设计

**日期：** 2026-08-30

**状态：** 已完成交互设计确认，待书面规格复核

**方案：** B — 领域模型重建优先

## 1. 背景与结论

瑶光营销平台已经具备多 Workspace、Customer API、联系人、名单、Broadcast、Automation、模板、Email/SMS/Push 渠道、事件账本、Outbox、队列、回执和分析等基础能力。当前主要问题不是缺少独立功能，而是 Customer、Contact、Audience、Broadcast、Automation、Message Queue 等模块没有形成统一的业务权威和可靠的端到端状态模型。

本设计以领域模型统一为先，逐步将现有能力迁移到以下闭环：

`Customer → Audience → Campaign/Journey → Delivery → Receipt/Analytics`

重构必须同时满足：

- 非技术运营人员能够完成客户接入、客群配置、内容制作、测试、发前检查、发送和复盘；
- 上传名单全部进入可对账的持久化流程，不允许静默丢失；
- 相同营销副作用在故障恢复时不得无条件重复执行；
- Automation 现有 `once` 和 `every_time` 行为保持兼容；
- 每个 Workspace 独立，不同客户的 Workspace 数据与运行状态互不关联；
- 不新增审批流程，保留现有审批和状态能力即可。

## 2. 目标与非目标

### 2.1 目标

1. 将 Customer 建立为唯一用户权威，Contact 降为兼容投影。
2. 统一静态名单、动态客群和组合受众为 Audience 领域。
3. 统一全量、立即、定时、周期活动为 Campaign 领域。
4. 将事件触发和时间触发流程统一为 Journey，并保留现有 Automation 入场语义。
5. 用 Delivery Intent、Attempt 和 Receipt 表达完整触达生命周期。
6. 建立 Workspace、Campaign、Journey、全量触达、渠道和 Customer 多层频控。
7. 建立可断点恢复、逐条确认、完整对账的大批量导入任务。
8. 重新组织控制台信息架构和配置流程，隐藏不必要的技术细节。
9. 用故障注入、迁移对账和性能基准作为生产切换门槛。

### 2.2 非目标

- 本轮不新增业务审批、多人会签或工单流程。
- 不一次性删除现有 Contact、Broadcast、Automation 或 Email Queue。
- 不要求所有 Provider 提供完全相同的能力；缺失能力必须以显式降级或 `unknown` 状态表达。
- 不以降低幂等、审计、租户隔离或同意检查换取性能。
- 不把渠道吞吐限速视为用户营销频控。

## 3. 总体原则

### 3.1 Workspace 是强租户边界

- 所有领域对象、唯一约束、幂等键、频控窗口和运行任务均限定在 Workspace 内。
- 不同 Workspace 之间不共享 Customer、外部用户 ID、Audience、Campaign、Journey、Delivery 或频控状态。
- 延续每个 Workspace 独立业务数据库的现有模式。
- 系统级数据库只保存用户、Workspace、任务路由和必要的平台配置，不承载跨 Workspace 营销数据。

### 3.2 渐进替换而非大爆炸迁移

迁移顺序固定为：

`新增结构 → 断点回填 → 数量/校验和对账 → 双读比对 → 灰度切换 → 下线旧写路径`

在完成对账和回滚验证前，不删除旧表、旧字段或公开接口。

### 3.3 单一写入权威

双写只能作为同一事务中的兼容投影，不允许两个模型分别接受业务写入后再异步相互覆盖。Customer 成为唯一写入权威；旧 Contact API 必须转换为 Customer Command。

## 4. 核心领域模型

### 4.1 Customer

Customer 表达一个 Workspace 内的唯一业务用户。

关键标识：

- `customer_id`：内部 UUID，是关系、运行时和外键权威；
- `customer_no`：不可变业务编号，格式为 `U + 四位 Workspace 序号 + yyyyMMddHHmmss + 08 + 32 位无连字符小写 UUID`；
- `external_user_id`：业务系统提供的用户 ID，同一 Workspace 内唯一；
- `identities`：Email、Phone、Anonymous ID、Device ID、WhatsApp、Telegram 和自定义身份；
- `profile`：标准属性和自定义属性；
- `consents`：按用途和渠道记录的同意状态；
- `version`：并发修改和幂等回放版本。

约束：

- Identity 指纹必须绑定 Workspace，不能跨 Workspace 关联用户；
- Customer Merge 必须保留来源、目标、原因和幂等结果；
- Contact 仅作为 Email 营销兼容投影，不能继续充当用户主键；
- 用户更换 Email 后，Customer、Audience、Journey 和频控历史不能被识别为新用户。

### 4.2 Audience

Audience 统一以下受众来源：

- `static_list`：人工维护或导入的静态名单；
- `dynamic_segment`：按 Customer 属性、身份、事件、目标、互动和时间窗口动态计算；
- `composite`：使用并集、交集和排除组合多个静态或动态受众。

核心对象：

- `AudienceDefinition`：业务名称、类型、规则和状态；
- `AudienceVersion`：不可变规则版本；
- `AudienceBuild`：一次预览或物化构建任务；
- `AudienceMembership`：Customer 级成员及进入/排除原因；
- `RecipientSnapshot`：Campaign 发布时冻结的 Customer 集合、版本、A/B 变体和资格原因。

Audience 必须支持纯 Segment、多 List、List 与 Segment 组合以及排除受众，不再强制单 List。

### 4.3 Content

Content 统一 Email、SMS、Push、WhatsApp、Telegram、In-App 和 Webhook 的内容定义，同时保留渠道专有载荷。

核心对象：

- `MessageTemplate`：名称、渠道、分类和语言；
- `TemplateVersion`：不可变内容版本；
- `TemplateVariant`：A/B 变体；
- `ContentAsset`：文件和可复用内容块。

技术 ID 默认自动生成。运营页面以名称为主，API ID、Liquid、JSON 和底层字段键仅在开发者或高级模式展示。

### 4.4 Campaign

Campaign 管理一次性和计划性群体营销：

- 全量立即发送；
- 指定时间发送；
- 周期性发送；
- A/B 实验及胜出变体发送。

核心对象：

- `Campaign`：业务身份和生命周期；
- `CampaignVersion`：不可变发布配置；
- `RecipientSnapshot`：不可变目标 Customer 集合；
- `CampaignRun`：一次实际运行；
- `CampaignPreflight`：发前检查结果。

发布时冻结 Customer 集合和变体，但在实际投递前重新检查当前有效身份、同意状态、投诉、退订、黑名单和频控。运行期间 Audience 变化不得改变已发布快照。

### 4.5 Journey

Journey 管理长生命周期、逐 Customer 运行的自动化流程：

- 事件触发；
- 时间或日期触发；
- 等待、条件、A/B、名单变更、Webhook 和渠道动作；
- 退出规则和失败恢复。

核心对象：

- `JourneyDefinition`：Automation 的领域身份；
- `JourneyVersion`：不可变已激活版本；
- `EnrollmentPolicy`：用户进入方式；
- `JourneyInstance`：一次 Customer 运行；
- `JourneyNodeExecution`：节点级输入、结果、等待、跳过和失败记录；
- `JourneyMatchAudit`：事件是否匹配及未进入原因。

现有 Automation API 和 UI 可以保留名称作为兼容层，逐步映射到 Journey 领域。

### 4.6 Delivery

Delivery 表达一次外部触达副作用的完整生命周期。

核心对象：

- `DeliveryIntent`：来源、Customer、渠道、模板版本、预期身份和稳定 Effect Key；
- `DeliveryAttempt`：Worker Claim、Provider 调用、响应和重试；
- `DeliveryReceipt`：送达、打开、点击、退信、投诉、回复等回执；
- `DeliveryReconciliation`：`unknown` 状态的自动或人工对账。

稳定 Effect Key 由以下维度构成：

`workspace + source_type + source_id + source_version + customer_id + node_or_phase + occurrence + variant`

数据库必须对 Workspace 内的 Effect Key 建立唯一约束。

### 4.7 Frequency Policy

频控与 Provider 吞吐限速分离。频控作用于 Customer 的营销触达资格；吞吐限速作用于渠道 Provider 的每秒或每分钟调用量。

Frequency Policy 支持：

- Workspace 全局营销限制；
- 单 Campaign 限制；
- Journey 事件触发保护；
- Journey 时间触发保护；
- 全量触达限制；
- 渠道限制；
- Customer 个性化限制。

所有适用策略同时生效，最严格结果优先。Customer 个性化限制不能放宽 Workspace 的绝对上限。

### 4.8 Import Job

大名单导入必须采用持久化任务：

- `ImportJob`：文件、Workspace、映射、总数、状态和校验信息；
- `ImportItem`：行号、原始数据指纹、幂等键、处理状态和错误；
- `ImportCheckpoint`：分片进度和恢复位置；
- `ImportReconciliation`：最终数量与失败文件。

始终满足：

`上传总数 = 待处理 + 处理中 + 成功 + 明确失败`

浏览器关闭、网络中断、Worker 重启或单批失败不能丢失未确认条目。

## 5. Automation 入场与分层频控

### 5.1 第一层：入场频率

保留现有两种模式：

#### `once` — 每个用户一次

- 同一 Customer 在同一 Automation 整个生命周期内最多进入一次；
- 编辑、暂停、恢复或发布新版本不重置；
- 唯一约束为 `(workspace_id, automation_id, customer_id)`；
- 如需重新执行，应复制 Automation 或使用以后单独设计的显式重置功能。

#### `every_time` — 每次事件

- 每个不同事件均创建一个 Journey Instance；
- 上一次 Journey 未结束时仍允许并行进入，保持现有行为；
- 同一事件因 Broker 或 Worker 重试不得重复进入；
- 去重约束为 `(workspace_id, automation_id, event_id)`。

现有按 `automation_id + contact_email` 保存的 `automation_trigger_log` 必须回填 `customer_id`。无法映射的历史记录保留兼容键，不能删除后让用户重新进入。

### 5.2 第二层：触发保护

可选高级配置，默认关闭，因此不改变 `once` 和 `every_time` 的兼容语义。例如：

- 每个 Customer 24 小时最多进入三次；
- 同类事件十分钟内最多触发一次；
- Journey 超过有效期不再进入。

判定顺序：

`事件幂等 → once/every_time → 可选触发保护 → 创建 Journey Instance`

未进入原因必须记录为 `once_already_entered`、`duplicate_event`、`trigger_frequency_limited` 或 `journey_expired`。

### 5.3 第三层：消息频控

Customer 进入 Journey 后，每个消息节点重新执行 Workspace、Journey、Automation、全量、渠道和 Customer 频控。`every_time` 允许重新进入不代表每次都立即发送消息；消息可进入 `deferred`。

营销消息达到频控后记录 `retry_at`，到窗口恢复后重试；超过 Campaign/Journey 有效期后转为 `frequency_expired`。事务通知默认不计入营销频控，但仍受同意、退订、渠道禁用和安全规则约束。

多策略频控必须通过一个原子操作预占所有适用窗口，并使用稳定 Reservation ID 实现重试幂等。Redis 不可用时，已启用频控的营销消息失败关闭并进入 `deferred`，不能绕过频控发送。

## 6. 端到端数据流

### 6.1 Customer 接入

1. API、Webhook 或 Import Item 转换为 Customer Command。
2. 规范化 Workspace、外部 ID、Identity、画像、同意和名单关系。
3. 校验幂等键和 Payload Hash。
4. 在同一 Workspace 事务中写 Customer、Identity、Consent、Membership 和 Outbox。
5. 返回逐条结果；批量响应遗漏任何输入项均视为失败。
6. Outbox 驱动 Contact 兼容投影、事件账本、Audience 和 Journey 后续处理。

### 6.2 Event 与 Journey

1. Event Ledger 以 Event ID 幂等接收业务事件。
2. Outbox Relay 发布事件，Consumer Inbox 防止重复消费。
3. Journey Matcher 计算条件并写 Match Audit。
4. 按入场频率和触发保护决定是否创建 Journey Instance。
5. 节点执行产生状态提交或 Delivery Intent。
6. Journey Instance、节点结果、Side Effect Reservation 和 Outbox Command 必须在同一事务提交。

### 6.3 Campaign

1. 保存草稿和内容版本。
2. 执行 Preflight，区分阻断错误和非阻断警告。
3. 发布不可变 Campaign Version。
4. 分页物化 Recipient Snapshot，记录 Customer、Audience 原因和变体。
5. 对每个 Snapshot Item 重新检查身份、同意、黑名单和频控。
6. 创建唯一 Delivery Intent 和 Outbox Command。
7. 分别展示快照、排除、延迟、入队、Provider 接受和终态计数。

### 6.4 Delivery 与回执

Delivery 状态机：

`planned → reserved → queued → submitting → provider_accepted → confirmed`

旁路状态：

`suppressed / deferred / transient_failed / terminal_failed / unknown / cancelled`

规则：

- Delivery Intent 与 Outbox Command 同事务创建；
- Worker 通过 Claim Token 和 Lease 原子领取；
- 成功后保留终态记录，不直接删除队列证据；
- Provider 支持幂等键时透传 Effect Key；
- Provider 可能已经接受但响应丢失时进入 `unknown`；
- 不支持 Provider 查询或幂等键时，`unknown` 禁止自动重发；
- 人工重发必须显示重复风险并产生新的审计记录；
- Receipt 按 Provider Message ID 或稳定关联键更新终态和分析投影。

现有 Realtime Ledger、Outbox、Inbox、Journey Lease 和 Webhook Worker 模式应复用。Email Queue 逐步改为 Delivery 执行投影，不再独立定义“已发送”的业务真相。

## 7. 产品信息架构

控制台采用以下一级入口：

1. 工作台；
2. 客户；
3. 客群；
4. 营销活动；
5. 自动化旅程；
6. 内容中心；
7. 数据分析；
8. 投递中心；
9. 设置。

### 7.1 Customer 360

统一展示：

- 瑶光用户编号、内部 UUID、外部用户 ID；
- Email、Phone、WhatsApp、Telegram、Device 等 Identity；
- 画像、标签和同意状态；
- 静态名单与动态客群；
- Event Timeline；
- Journey 当前节点、进入和未进入原因；
- 所有消息的发送、跳过、延迟、未知和失败原因。

### 7.2 客群配置

默认业务编辑器采用：

`客户字段/行为事件 → 条件 → 值 → 且/或组合`

实时展示预计人数、样本 Customer、进入原因和排除原因。JSON、SQL 表达和底层字段键只放在高级模式。

### 7.3 Campaign 向导

固定为：

1. 活动目标与名称；
2. 选择客群；
3. 选择渠道与内容；
4. 时间、时区和频控；
5. 测试与预览；
6. 发前检查并发布。

UTM、Recipient Feed、JSON Path 和技术 ID 默认折叠到高级配置。

Preflight 页面必须展示：

- 快照受众总数；
- 无有效 Identity、退订、投诉、黑名单和频控排除人数；
- 内容、语言和 A/B 变体；
- Channel Provider；
- 发送时间及时区；
- 测试发送结果；
- 阻断错误和非阻断警告。

“立即发送”只能在 Preflight 通过后的最终确认页执行，确认按钮必须显示实际受众数量。

### 7.4 Journey 配置

新建 Journey 优先提供欢迎、提醒、唤醒、事件通知等业务模板，空白画布为高级入口。

配置顺序：

`触发条件 → 用户进入方式 → 可选触发保护 → Journey 节点 → 消息频控 → 测试事件 → 检查并启用`

现有“每个联系人一次”和“每次”选项保留，但标签改为“用户进入方式”。“每次”必须提示同一用户可能同时运行多个 Journey Instance。

启用前检查缺失模板、Channel Integration、无效分支、循环风险和频控冲突。

### 7.5 非技术用户适配

- API ID 默认自动生成，只在开发者区域显示和修改；
- 设置分为业务设置、渠道设置、开发者设置和系统管理；
- Integration 按 Email、SMS、Push、即时通讯、Webhook、AI 和数据服务分类；
- 统一中文术语：Bounce 为“退信”、Complaint 为“投诉”、Segment 为“动态客群”、Delivery 为“投递”；
- 所有用户文本进入 Lingui，CI 禁止未包装的可见字符串；
- 低于 768px 使用抽屉式导航和单栏表单；
- 所有图标按钮有可访问名称；
- Drawer 支持 Escape，存在未保存内容时进行离开确认。

## 8. 异常模型

统一异常状态：

| 状态 | 行为 |
|---|---|
| `validation_failed` | 阻断并定位字段，不重试 |
| `suppressed` | 因同意、退订、投诉、黑名单或无有效 Identity 永久跳过 |
| `deferred` | 记录 `retry_at`，频控窗口或依赖恢复后重试 |
| `transient_failed` | 指数退避并受最大尝试次数限制 |
| `unknown` | 停止自动重发，进入 Provider 对账或人工处理 |
| `terminal_failed` | 明确拒绝或重试耗尽 |
| `cancelled` | 不再产生新的 Delivery Intent |

公开 API 错误统一返回：

- `request_id`；
- `error_code`；
- `field_path`；
- `retryable`；
- `user_action`。

控制台优先显示可执行修复建议，技术详情和关联 ID 折叠展示。

## 9. 可观测性与运维

### 9.1 健康检查

- `livez`：进程是否存活；
- `readyz`：实例当前角色是否可以接收流量；
- Role Readiness：分别覆盖 API、Event Relay、Journey Worker、Delivery Worker 和 Analytics Worker。

依赖检查包括 System DB、Workspace DB 路由、RabbitMQ、Redis、Outbox Relay 和 Worker Heartbeat。

### 9.2 指标

- Event、Outbox、Journey、Delivery 的积压量与最老等待时间；
- Import Job 总数、成功、失败、待处理和停滞时间；
- Provider 接受率、确认率、失败率和 `unknown` 数量；
- 频控允许、延迟和拒绝数量；
- 防止的重复入场和重复投递数量；
- Worker Lease 超时和数据库连接等待；
- 每个 Workspace 的公平调度和资源占用。

调用链必须能够按以下关系追踪：

`request_id → event_id/import_job_id → journey_instance_id/campaign_run_id → delivery_id → provider_receipt_id`

日志默认屏蔽完整 Email、Phone、Identity Value 和消息内容。

## 10. 性能与容量基准

参考环境：8 核、16GB 应用节点，独立 PostgreSQL、Redis 和 RabbitMQ。

- 同步 Customer Batch 默认最大 10,000 条，由 `CUSTOMER_SYNC_MAX_BATCH_SIZE` 配置；
- 文件 Import Job 默认支持 1,000,000 条，后台可配置；
- 文件必须完整进入持久化任务后再开始 Customer 处理；
- 百万 Recipient Snapshot 分页物化，不得整体加载内存；
- Customer 单条写入 p95 小于 200ms；
- 单节点持续 500 events/s 时，Event 接收 p95 小于 200ms；
- Event 到 Delivery Intent p95 小于 5 秒；
- 百万 Customer Workspace 的常用列表、搜索和 Audience Preview p95 小于 2 秒；
- 单一高流量 Workspace 不得阻塞其他 Workspace；
- 渠道吞吐由 Provider 限速控制，平台调度额外开销不超过 10%。

这些数字是第一轮容量验收目标，允许通过实测调整硬件基线，但不允许放宽完整性、幂等和审计要求。

## 11. 测试与验收

### 11.1 必须覆盖的故障注入

1. Delivery 入队提交后、旧任务游标保存前杀进程，不得重复投递。
2. Provider 接受后、数据库更新前杀 Worker，必须进入 `unknown` 或通过 Provider 幂等安全恢复。
3. Claim 后杀 Worker，Lease 到期后只能由一个 Worker 恢复。
4. Redis 不可用时，已启用频控的营销消息必须进入 `deferred`。
5. Import Job 任意分片中断后恢复，逐条结果和总数完全对账。
6. Event 重放和 Broker 重复投递不得重复创建 Journey 或 Delivery。

### 11.2 Automation 兼容验收

1. `once` Customer 更换 Email 后仍不能再次进入。
2. `once` Automation 更新版本、暂停和恢复后仍不能再次进入。
3. `every_time` 两个不同 Event 创建两个 Journey Instance。
4. `every_time` 同一 Event 重试只创建一个 Journey Instance。
5. `every_time` 在已有活动 Journey 时仍允许不同 Event 创建并行实例。
6. 默认关闭触发保护时，行为与当前控制台描述一致。

### 11.3 数据与迁移验收

- 每个 Workspace 独立统计 Customer、Identity、Contact 投影和 Trigger Log 回填数量；
- 外部用户 ID、Identity 和 Customer Number 冲突必须在切换前阻断；
- 迁移前后按总量、关键状态和校验和对账；
- 所有回填任务可断点恢复并幂等重跑；
- 读取切换具有 Workspace 级 Feature Flag 和回滚路径；
- 旧 API 与新 API 在兼容期通过 Contract Test；
- OpenAPI 与公开运行时路由双向一致。

### 11.4 产品验收

- 非技术运营人员能够按页面指引完成 Customer 接入、Audience、模板、测试、Preflight 和发布；
- 立即发送前显示准确快照人数、排除原因、渠道、内容和时区；
- Customer 360 能解释用户进入/未进入 Journey 及消息发送/未发送原因；
- 375px 宽度下能够完成 Customer、Audience、Campaign 和 Journey 核心操作；
- 中文界面不出现未翻译文本和错误业务术语；
- 图标按钮、表单、Drawer 和画布关键操作满足键盘访问要求。

## 12. 实施分解与切换门槛

方案 B 拆为四个顺序子项目，每个子项目分别形成实施计划：

### B0：Customer 权威与兼容层

- 完成 Customer/Contact 写入收敛；
- 增加 Customer 360 基础查询；
- 将 List、Segment、Automation 逐步引用 `customer_id`；
- 完成历史数据回填、对账和兼容 API。

**切换门槛：** Customer/Contact 对账通过，旧 API 行为兼容，Workspace 可独立回滚。

### B1：Delivery 一致性

- 建立 Delivery Intent、Attempt、Receipt 和 Reconciliation；
- 修复 Broadcast 入队与游标边界；
- 将 Email Queue 改为可恢复 Claim/Lease 状态机；
- 明确 `unknown` 处理和 Provider 幂等。

**切换门槛：** 六类故障注入通过，无静默丢失和无条件重复发送。

### B2：Audience、Campaign、Frequency 与 Import

- 上线正式 Audience 工作台和组合受众；
- 建立 Campaign Version、Recipient Snapshot 和 Preflight；
- 接入三层 Automation/消息频控；
- 建立百万级持久化 Import Job。

**切换门槛：** 受众快照、频控原子预占、导入完整对账和容量测试通过。

### B3：Journey 与新控制台

- 将 Automation 映射到 Journey Definition/Version/Instance；
- 补齐 Customer 级 Journey Explainability；
- 重构导航、配置向导、设置分类、中文术语和响应式布局；
- 补齐 OpenAPI、操作文档和运行看板。

**切换门槛：** Automation 兼容测试、运营 E2E、移动端、可访问性和 OpenAPI Contract Test 全部通过。

## 13. 完成定义

方案 B 只有在以下条件同时满足时才视为完成：

- Customer 是所有新写入的唯一用户权威；
- Audience 能表达静态、动态和组合受众；
- Campaign 和 Journey 均通过 Delivery 产生渠道副作用；
- `once` 和 `every_time` 行为保持兼容；
- 分层频控在所有营销渠道生效且失败关闭；
- 批量上传可以逐条对账且支持中断恢复；
- 任意投递可以解释为发送、延迟、跳过、失败或未知；
- 核心流程适合非技术运营人员使用；
- 所有迁移、故障、容量和端到端验收通过。
