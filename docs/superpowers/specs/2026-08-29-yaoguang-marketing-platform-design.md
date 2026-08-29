# 瑶光营销平台总体设计

**产品名称：** 瑶光营销平台  
**英文标识：** `yaoguang-marketing`  
**品牌文案：** 观心知意，循光达客  
**设计状态：** 已完成交互确认，待书面规格复核  
**目标分支：** `main`
**设计日期：** 2026-08-29

## 1. 产品定位

瑶光营销平台是一套面向金融科技及互联网业务的开源用户营销与客户触达平台。系统通过统一 Customer Profile API 和 Event API 接收业务系统的实时用户数据，提供动态客群、事件触发、定时营销、营销 Journey、全量 Campaign、A/B 实验、频控治理以及 Email、SMS、Push、WhatsApp、Telegram、In-App、Webhook 等多渠道触达能力。

平台以当前 Notifuse Go/React 代码库为基础演进，不组合部署 Mautic 或 Laudspeaker，也不复制它们的执行引擎。Mautic 用于参考营销资产、偏好中心和频控治理；Laudspeaker 用于参考事件驱动 Journey 的产品体验；Notifuse 及本分支已完成的实时运行时继续作为实现核心。

## 2. 已确认的产品边界

- 每个 Workspace 是独立的业务租户或应用，使用独立业务数据库。
- 不同 Workspace 的客户不关联、不合并；所有身份唯一性只在所属 Workspace 内成立。
- 本期不新增营销活动审批或双人复核，保留现有发布权限与行为。
- 策略继续保留草稿、不可变发布版本、暂停、终止、审计和恢复能力。
- Customer 使用内部 UUID 作为数据库关联主键，同时生成业务可见的瑶光客户编号。
- 业务系统可以直接使用自己提供的 `external_user_id` 调用所有 Profile/Event 接口。
- 匿名身份可以显式合并到已知客户；系统不根据相似属性自动合并客户。
- 渠道采用原生适配器与通用签名 Webhook 并存的混合模式。
- 首期优先完成可生产使用的营销闭环；WhatsApp、Telegram、In-App 可先通过通用 Webhook 接入，原生适配器后续按需求补充。
- 已确认接收的 Profile、Event 和批量名单不得静默丢失。
- 批量限制提供开箱默认值，并允许系统后台和 Workspace 后台配置。

## 3. 方案选择

### 3.1 采用：在 Notifuse 上渐进演进

保留当前单仓库、Clean Architecture、每 Workspace 独立 PostgreSQL 和一个 Go 制品多运行角色的设计。新增能力先形成清晰的内部领域边界，只有在出现独立扩容或独立发布的实际需求后才拆分服务。

该方案可以复用当前 `main` 分支已经完成的事件账本、Transactional Outbox、RabbitMQ workers、Redis 频控基础、ClickHouse 投影、动态客群、外部接入、SMS/Push 模板、端点加密、渠道回执和幂等发送能力。

### 3.2 不采用：首期直接拆分微服务

首期拆分 Profile、Event、Segment、Journey、Campaign 和 Delivery 会立即引入跨服务事务、契约版本、分布式追踪和数据一致性成本，并重复实现已有能力。模块接口应支持未来拆分，但本期不扩大部署和故障面。

### 3.3 不采用：组合 Mautic、Laudspeaker 与 Notifuse

组合三个运行时会产生多套 Customer Profile、事件账本、权限和执行状态，难以满足客户身份一致、触达幂等和无丢失对账要求。Mautic 和 Laudspeaker 仅作为产品与治理参考，不进入生产依赖。

## 4. 总体架构

瑶光采用“模块化单体 + 多运行角色”架构。代码、领域模型和发布制品保持统一，API、事件转发、客群计算、Journey、Campaign、投递和分析可以作为独立进程横向扩容。

```text
业务系统 / SDK / 文件导入
          |
          | Customer Profile API / Event API / Import API
          v
      API + 身份解析
          |
          | 同一 PostgreSQL 事务
          +--> Customer / Profile / Identity / Consent
          +--> Event Ledger
          +--> Transactional Outbox
                        |
                        v
                  Outbox Relay
                        |
                        v
                    RabbitMQ
           +------------+-------------+
           |            |             |
           v            v             v
    Segment/Rule    Journey Worker  Campaign Worker
           |            |             |
           +------------+-------------+
                        |
                  Delivery Policy Gate
       身份 / 同意 / 抑制 / 静默时间 / 四层频控
                        |
                        v
                 Delivery Worker
           +------------+-------------+
           |                          |
      原生渠道适配器               通用签名 Webhook
   Email / Twilio / FCM      WhatsApp / Telegram /
                              In-App / 自定义渠道
           |                          |
           +------------+-------------+
                        |
                 Receipt / Callback
                        |
              PostgreSQL + ClickHouse
```

### 4.1 基础设施职责

| 组件 | 权威数据 | 职责 | 禁止事项 |
| --- | --- | --- | --- |
| PostgreSQL 17 | 是 | Customer、Profile、身份、事件账本、策略版本、运行状态、幂等与审计 | 不将分析查询放入投递热路径 |
| PgBouncer | 否 | transaction pooling 与连接数隔离 | 不依赖 session 级状态 |
| RabbitMQ 4 | 否 | 事件与任务传递、削峰、重试和死信 | 不作为业务事实源 |
| Redis 7 | 否 | 并发频控预留、短期去重和可重建缓存 | 不保存权威 Journey 或 Campaign 状态 |
| ClickHouse | 否 | 事件、漏斗、归因和营销分析投影 | 不反向成为在线事实源 |
| MinIO/S3 | 文件与素材 | 导入原文件、图片、附件和渠道素材 | 不保存营销策略元数据 |

### 4.2 运行角色

- `api`：公开 HTTP、认证、同步写入和查询。
- `outbox-relay`：公平扫描 Workspace Outbox，并通过 publisher confirms 发布。
- `segment-worker`：增量重算动态客群，并执行分批全量校准。
- `rule-worker`：根据事件依赖索引筛选并匹配候选 Journey。
- `journey-worker`：认领 Journey 实例并提交版本化状态转换。
- `campaign-worker`：冻结受众、生成 Campaign Run 和分片投递任务。
- `delivery-worker`：执行投递策略门和渠道外部副作用。
- `analytics-worker`：将标准事件投影到 ClickHouse。
- `scheduler`：发布定时 Journey、Campaign、频控延迟消息和维护任务。
- `all`：仅用于本地开发和小规模兼容部署。

## 5. Workspace 隔离

- 系统数据库保存 Workspace 元数据、不可复用的四位 Workspace 序号和全局后台配置。
- 每个 Workspace 使用独立 PostgreSQL 数据库。
- 所有 API、消息、Redis key、对象存储路径和指标标签必须携带 `workspace_id`。
- API Key 绑定 Workspace；跨 Workspace 操作需要系统管理员专用权限，业务接口不提供跨 Workspace Customer 查询。
- 相同 `external_user_id`、Email 或手机号可以存在于不同 Workspace，系统不得自动建立关联。

## 6. Customer、客户编号与身份

### 6.1 内部主键与客户编号

`customers.id UUID PRIMARY KEY` 是数据库关联和运行时幂等的权威主键。系统同时生成不可变、业务可见的 `customer_no VARCHAR(53) UNIQUE NOT NULL`：

```text
U{workspace_seq:04d}{yyyyMMddHHmmss}{08}{uuid32}
```

示例：

```text
U00272026082915304508a9b4c7d21f6e4d01a9325fc80b7312ae
```

生成规则：

- `U` 是固定客户前缀。
- Workspace 序号范围为 `0001` 至 `9999`，由系统数据库永久分配，Workspace 删除后不复用。
- 日期时间按 `Asia/Shanghai` 生成，格式固定为 `yyyyMMddHHmmss`。
- `08` 是固定东八区标识。
- UUID 尾部使用完整 32 位、无连字符、小写十六进制形式。
- 达到 9999 个历史 Workspace 后，创建 Workspace 返回明确容量错误；扩大编号格式必须通过新版本迁移完成，禁止复用旧序号。

### 6.2 Customer 数据模型

`customers` 保存内部 UUID、客户编号、可空的 `external_user_id`、合并重定向和乐观锁版本。已知客户的 `external_user_id` 在所属 Workspace 内唯一；匿名客户可以暂时没有外部用户 ID。

`customer_profiles` 以 `customer_id` 为主键，保存业务状态、语言、时区和动态 `attributes JSONB`。常用属性通过受控表达式索引或事实投影加速，不预先固化大量业务列。

`customer_identities` 支持：

- `email`
- `phone`
- `anonymous_id`
- `device_id`
- `whatsapp`
- `telegram`
- `custom`

同一 Workspace 内按“身份类型 + 标准化检索指纹”唯一。手机号规范化为 E.164；Email 使用统一标准化规则；Provider ID 保持精确大小写语义。敏感身份保存加密值、检索指纹、验证状态、主身份标记、启停状态和渠道元数据。

`customer_tags`、`customer_consents` 和 `customer_merge_log` 全部以 `customer_id` 关联。

### 6.3 外部用户 ID

业务系统可以直接使用 `external_user_id` 上报 Profile、Event、身份和端点，无需预先获取 UUID。所有成功响应同时返回：

- `customer_id`
- `customer_no`
- `external_user_id`

Customer 可以通过 UUID、客户编号、外部用户 ID或身份别名读取。API 内部必须先解析为 UUID，再执行业务逻辑。

### 6.4 匿名合并

首期只支持显式“匿名 Customer 合并到已知 Customer”：

- 新请求解析旧 UUID 时重定向到目标 UUID。
- 身份、端点、标签、同意和在线运行状态迁移到目标 Customer。
- 不物理重写大规模不可变事件历史；查询和分析通过合并映射解析到目标 UUID。
- 系统不得仅凭姓名、相似属性或共享设备自动合并两个已知客户。
- 身份唯一性冲突返回 `409`，要求调用方显式处理。

## 7. 从 Email 主键迁移

当前代码有大量表和运行流程使用 Email 作为联系人主外键。一次性替换会扩大停机、锁表和回滚风险，因此采用双轨迁移：

1. 新建 `customers`、UUID Profile/Identity/Tag/Consent 表。
2. 现有 `contacts` 增加 `customer_id`，作为邮件投递、旧名单和旧 Journey 的兼容投影。
3. 对存量联系人生成 UUID、客户编号和 Email identity，并回填 `customer_id`。
4. 新 API 只通过 Customer Service 写入，Customer 表和兼容 Contact 投影在同一事务收敛。
5. 依次将名单、动态客群、Journey、端点、消息历史和 Campaign Recipient 迁到 `customer_id`。
6. Email 字段在迁移周期保留为兼容读模型；新功能不得增加新的 Email 主键依赖。
7. 所有涉及 `contacts`、名单、事件和自动化触发器的迁移必须重新生成并验证现有 PL/pgSQL trigger functions。

## 8. Customer Profile 与 Event API

### 8.1 Profile API

`POST /api/customers.upsert` 接受 `external_user_id`、Profile、身份、标签和名单关系。Profile 属性更新明确区分：

- `set`：替换指定对象。
- `merge`：合并指定属性。
- `unset`：删除指定属性路径。

JSON `null` 不同时承担“未提供”和“删除”的语义。

`GET /api/customers.get` 支持通过 UUID、客户编号、外部用户 ID 或身份别名定位。

`POST /api/customers.merge` 执行显式匿名合并。

### 8.2 Event API

`POST /api/events.track` 接受稳定 `event_id`、Customer 定位信息、`event_name`、`occurred_at`、可选 `sequence` 和 `properties`。

- API 中的 `event_id` 是业务方提供的任意稳定字符串，在 Workspace 内是幂等键；数据库将其保存为 `external_event_id`。
- Event Ledger 继续使用平台生成的 `event_uuid UUID` 作为内部主键和消息关联键，响应同时返回 `event_uuid` 与原始 `event_id`。
- 相同 ID 和相同载荷返回原结果。
- 相同 ID 和不同载荷返回 `409 idempotency_conflict`。
- 外部用户不存在时，使用 `external_user_id` 原子创建最小 Customer。
- `occurred_at` 是业务发生时间，`received_at` 由平台生成。
- Event 不可更新；修正通过补偿事件完成。
- 跨请求不承诺严格顺序；提供递增 `sequence` 的迟到事件仍入账，但带迟到标记。

事件继续写入现有 `event_ledger`，新增 `external_event_id`、`customer_id` 和 `external_user_id`，不建立第三套事件事实表。`event_idempotency` 改为登记外部事件 ID、内部事件 UUID、载荷哈希和接收时间；`event_ledger` 与 Transactional Outbox 在同一事务提交。

### 8.3 API 权限

- `customers:read/write`
- `events:read/write`
- `campaigns:read/write/execute`
- `journeys:read/write/execute`
- `channels:read/write`
- `receipts:write`
- `imports:read/write/execute`

每个响应返回 `request_id`。日志只记录 UUID、客户编号和脱敏身份，不记录完整手机号、设备令牌、渠道凭据或模板渲染后的敏感内容。

## 9. 无丢失批量接入与名单导入

### 9.1 默认容量

| 接入方式 | 默认单次容量 | 默认体积 | 返回方式 |
| --- | ---: | ---: | --- |
| 实时微批 | 10,000 条 | 32 MiB | 可同步返回明细 |
| 异步 Batch | 100,000 条 | 256 MiB | 返回 `batch_id` |
| 文件导入 | 10,000,000 行 | 5 GiB | 返回 `import_job_id` |

`POST /api/ingest.batch` 在 10,000 条以内允许同步处理，超过后转为异步作业。JSON/NDJSON Batch 支持 gzip 和流式解析，服务端不得将整个大请求一次性读入内存。超过异步 Batch 上限时使用 CSV、Excel 或 JSONL 文件导入。

### 9.2 后台可配置限制

系统管理员设置平台默认值与系统上限；Workspace 可以配置独立覆盖值，但不得超过系统上限。后台配置包括：

- 同步最大条数和体积。
- 异步 Batch 最大条数和体积。
- 文件导入最大行数和体积。
- 内部事务分片大小，默认 5,000。
- Workspace 并行导入数，默认 2。
- 原始文件保留天数，默认 30。
- 失败作业自动重试次数，默认 10。

配置变更只影响新提交的作业，不截断已运行作业。每次修改保存操作人、修改前后值和时间。

### 9.3 持久化确认

API 只有在以下内容持久化后才确认接收：

- 原始载荷或对象存储文件。
- SHA-256、体积、预期行数和格式信息组成的不可变 Manifest。
- `batch_id` 或 `import_job_id` 及初始状态。

同步微批在 Manifest 持久化并且每条记录进入明确终态后返回 `200 OK` 和完整结果；异步 Batch/文件导入在 Manifest 与原始载荷持久化后返回 `202 Accepted` 和作业 ID。对象存储或数据库不可用时拒绝请求，不返回伪成功。调用方可以提供稳定 Batch ID；相同 ID、相同载荷安全返回已有作业，相同 ID、不同载荷返回冲突。

### 9.4 全量对账

每行使用 `(job_id, row_number)` 作为处理幂等键。作业只有满足以下等式才能进入 `completed`：

```text
原始总行数 = 成功写入数 + 已存在且幂等收敛数
```

任何未处理、重试或永久校验错误都会使作业保持 `processing`、`retrying` 或 `needs_attention`。永久错误生成可下载逐行报告；修正后只重跑失败项。后台定时对账文件行数、暂存行数和终态行数，并对停滞任务报警。

Customer、身份、标签和名单关系在单行事务内提交；成功事务同时写 Outbox。Worker 使用租约、检查点和至少一次执行，通过数据库幂等收敛保证崩溃恢复后不漏写、不重复创建客户。

## 10. 动态客群

动态客群条件统一支持：

- Profile 固定字段和动态属性。
- 标签、名单和渠道身份。
- 同意状态和渠道可达性。
- 业务事件是否发生、次数、金额与时间窗口。
- Email、SMS、Push 等互动行为。
- Journey/Campaign 历史参与状态。

表达式编译为参数化、白名单受控 SQL，禁止拼接任意 SQL。发布 Segment 时生成依赖索引；Profile、Event 和互动变化只重算依赖相关字段的候选 Segment。`segment-worker` 执行增量重算，`scheduler` 触发分批全量校准。

PostgreSQL 保存在线客群资格和成员关系。ClickHouse 仅用于分析，不能进入实时资格判断或投递热路径。需要长窗口事件条件时，由后台任务将稳定聚合事实投影到 PostgreSQL，再参与在线判断。

## 11. Campaign

现有 `broadcasts` 演进为多渠道 Campaign，负责：

- 立即发送。
- 指定时间发送。
- 周期性定时营销。
- 指定名单或动态客群发送。
- Workspace 全部符合营销资格客户的全量通知。
- Email、SMS、Push 或通用 Webhook 单渠道批量触达。
- Campaign 级 A/B 实验。

核心对象：

- `campaigns`：名称、渠道、状态和当前版本。
- `campaign_versions`：受众、模板、渠道、实验和策略配置的不可变发布快照。
- `campaign_runs`：每次立即、定时或周期执行的独立实例。
- `campaign_recipients`：冻结目标客户、实验分组、投递状态和排除原因。

每个 Run 先冻结受众快照。客户在冻结后的属性变化不得悄悄改变正在发送的范围。暂停和恢复从权威检查点继续，稳定 `effect_key` 保证不重复触达。

全量 Campaign 必须先显示预估人数，执行时再次冻结实际资格。全量不绕过营销同意、退订、投诉、黑名单、静默时间或频控。

## 12. Journey

现有 `automations` 演进为营销 Journey，支持：

- Profile 或业务事件触发。
- 加入或离开客群触发。
- 固定时间、Cron 和客户本地时间触发。
- 延迟、条件、分支、A/B、Email、SMS、Push、Webhook 和名单操作。
- 后续 WhatsApp、Telegram 和 In-App 节点复用同一渠道接口。

Journey 发布后生成不可变版本。已进入 Journey 的 Customer 继续执行进入时版本；新 Customer 使用最新版本。每个实例使用原子租约、状态版本和节点副作用账本，Worker 重启或 RabbitMQ 重投不会重复执行外部副作用。

Campaign 用于批量、全量和独立 Run；Journey 用于持续事件和时间编排。两者共享 Customer、模板、渠道、Policy Gate、Receipt 和分析，不新建第三个营销执行引擎。

## 13. A/B 实验

- Campaign 和 Journey 都使用不可变版本承载实验配置。
- 客户分组通过 `customer_id + campaign/journey version` 的确定性哈希完成。
- 重试、暂停、恢复和 Worker 切换不得改变客户分组。
- 支持模板内容、标题、渠道和 Journey 路径实验。
- 指标包括送达、打开、点击、转化、转化金额和退订等保护指标。
- 自动选胜必须满足最短运行时间和最小样本量；未满足时保持等待或由操作人手动选择。

## 14. 四层频控模型

瑶光参考 Mautic 的系统默认规则、联系人覆盖规则、分渠道计数、营销消息延迟队列和交易消息排除语义，并针对本产品拆分四个独立频控桶。

### 14.1 单独活动频控

作用于指定 Campaign、Journey 或 Journey Node：

- 每名客户最多进入或接收 N 次。
- 两次触达最小间隔。
- 支持 `once`、`every_time`、每日/每周上限和冷却期。
- 超限动作可以是延迟或本次跳过。

### 14.2 事件/时间触发营销频控

独立约束事件触发 Journey、Cron、固定日期、客户本地时间和周期营销。可按渠道配置小时、日、周、月或滑动窗口上限。延迟超过事件有效期或 `max_delay` 后跳过，禁止在业务时效失效后突然补发。

### 14.3 全量 Campaign 频控

- 每名客户在周期内接收批量/全量 Campaign 的次数上限。
- Workspace 每日启动全量 Campaign 的数量上限。
- Workspace 每日允许进入投递队列的全量目标人数上限。
- 默认同一 Workspace 只允许一个全量 Campaign 处于受众冻结或发送状态。
- 全量流量不得挤占事件触发流量的独立额度。

### 14.4 营销总量兜底

跨活动、触发方式和渠道设置最终营销总量上限。一次投递必须同时通过所属活动桶、流量类型桶、渠道桶和全局桶；任一桶超限即执行该策略的延迟或跳过动作。

### 14.5 策略维度

```text
scope: workspace | customer | campaign | journey | node
traffic_class: triggered | scheduled | broadcast | all
channel: email | sms | push | whatsapp | telegram | in_app | webhook | all
period: hour | day | week | month | sliding_window
action: defer | skip
max_delay
priority
```

Customer 偏好可以覆盖 Workspace 默认频率，但只能收紧，不能突破退订、投诉或安全黑名单。Redis 在外部发送前执行原子预留，PostgreSQL 保存策略和审计。明确未发送的永久失败释放预留；结果为 `unknown` 时保留计数，防止重复触达。Redis 不可用时营销投递延迟处理，不无条件放行。

## 15. Delivery Policy Gate

每个 `DeliveryCommand` 包含 Workspace、Customer、客户编号、渠道、消息目的、模板版本、集成、稳定 `effect_key`、计划时间和上下文。

固定检查顺序：

1. Customer 与渠道身份有效性。
2. Workspace、Customer 或地址黑名单/抑制名单。
3. 消息目的对应的同意状态。
4. 退订、投诉、硬退信和无效设备。
5. Customer 静默时间。
6. 单活动、触发/定时、全量和全局四层频控。
7. 模板、渠道集成和变量完整性。
8. 使用 `effect_key` 原子预留外部副作用。

消息目的包括 `marketing`、`service` 和 `transactional`。营销消息不能绕过营销退订；服务或交易通知可以按 Workspace 策略不计入营销频控，但仍不能绕过无效地址、投诉和安全黑名单。

静默时间依次使用端点时区、Customer Profile 时区和 Workspace 默认时区。命中静默时间时延迟到下一个可发送时间；超过消息最大延迟后跳过。

## 16. 渠道架构

### 16.1 Provider 接口

```go
type ChannelProvider interface {
    Send(context.Context, RenderedMessage) (ProviderResult, error)
    ValidateConfig(config map[string]any) error
    NormalizeReceipt(payload []byte) (DeliveryReceipt, error)
    Capabilities() ChannelCapabilities
}
```

模板在服务端通过统一 Liquid 渲染内核生成 channel-neutral payload，Provider 不拥有模板业务逻辑。

### 16.2 首期渠道

- 原生 Email：复用 SES、Mailgun、Postmark、Mailjet、SparkPost、SendGrid 和 SMTP。
- 原生 SMS：Twilio。
- 原生 Push：FCM HTTP v1。
- 通用 Webhook：承载 WhatsApp、Telegram、In-App、国内短信/Push 和自定义渠道。
- APNs 与 Web Push 保留已有端点模型，原生 Sender 后续补充。

### 16.3 通用签名 Webhook

Outbound Webhook 使用 HTTPS、HMAC-SHA256、时间戳、Nonce 和稳定 `effect_key`。接收端必须按 `effect_key` 幂等，并返回：

- `accepted`
- `rejected`
- `retryable`

回执通过独立签名 Receipt API 写入。密钥按 Workspace Integration 加密保存，签名验证使用恒定时间比较并拒绝超时或重复 Nonce。

### 16.4 故障分类

- 明确限流、连接前失败和 `5xx`：遵守 `Retry-After` 并指数退避。
- 参数、权限和无效地址：永久失败。
- 请求可能已被 Provider 接受但结果未知：标记 `unknown`，不得自动重发。
- Journey 可以按成功、失败、超时或未知结果分支。
- 所有重试、回执和状态变化写入客户时间线与投递审计。

## 17. 分析与审计

标准营销事件包括：

- `customer.created/updated/merged`
- `segment.joined/left`
- `journey.entered/node_completed/exited`
- `campaign.eligible/excluded/sent`
- `delivery.deferred/accepted/delivered/opened/clicked/failed`
- `frequency.reserved/exceeded`

Campaign 分析展示目标、资格、排除原因、发送、送达、互动、失败、退订、转化和 A/B 表现。Journey 分析展示进入、运行、完成、退出、失败、节点漏斗、分支和停留时间。

Customer 时间线展示 Profile/身份变化、客群变化、Campaign/Journey 参与、Policy Gate 决策、渠道请求、重试、回执和最终状态。页面使用 `customer_no`，敏感身份默认脱敏。

运营监控覆盖 API、RabbitMQ 积压、Worker 租约、导入对账、渠道质量、Receipt 延迟、四类频控和 ClickHouse 投影延迟。

## 18. 品牌与控制台

### 18.1 品牌资产

- 左上角使用本地发布的 `hengshucredit_animated.svg`，不依赖运行时外链。
- 展开状态显示 36px Logo、`瑶光营销平台` 和 `观心知意，循光达客`。
- 折叠状态只显示 Logo，并提供产品名 Tooltip。
- 登录页、初始化向导、加载页、浏览器标题、favicon、PWA Manifest、系统邮件和 OpenAPI 标题统一改名。

### 18.2 导航

- 工作台
- 客户档案
- 动态客群
- 事件管理
- 营销活动
- 营销旅程
- 内容模板
- 渠道管理
- 导入任务
- 营销分析
- 系统设置

控制台新增完整 `zh-CN` 翻译，中文浏览器默认显示中文，同时保留英文和现有语言。

### 18.3 工程标识与兼容

- Go module 迁移到 `github.com/hengshu-credit/yaoguang-marketing`。
- Docker 镜像、README 和部署示例使用 `yaoguang-marketing`。
- 新环境变量使用 `YAOGUANG_*`；原 `NOTIFUSE_*` 兼容读取一个迁移周期。
- RabbitMQ routing key、数据库表名和迁移版本等持久化内部标识不在首期强制改名，避免升级时丢失积压消息或破坏已有数据。
- 保留 Notifuse 的 AGPL-3.0 许可证和必要版权声明。

## 19. 安全与隐私

- Workspace 数据库、对象存储前缀、消息和 Redis key 强隔离。
- API Key 使用最小权限并支持撤销与轮换。
- 渠道凭据、设备令牌和敏感身份加密保存。
- 页面、日志和错误默认脱敏手机号、Email 和 Provider ID。
- 通用渠道 Webhook 与 Receipt 校验签名、时间戳和 Nonce。
- 原始导入文件按后台配置保留，到期清理前要求作业完成且对账一致。
- Customer 删除由可审计后台任务覆盖 Profile、身份、端点和分析投影。
- 查询、导出和批量操作写入审计日志。

## 20. 可靠性与恢复

- 已确认接收的 Profile/Event 已在 PostgreSQL，已确认导入文件已在对象存储和 Manifest 中，目标 RPO 为 0。
- PostgreSQL 事务同时写业务状态与 Outbox。
- RabbitMQ 消费使用 Inbox、租约、显式确认和死信。
- 死信进入后台异常中心，不自动删除。
- 定时对账 Event/Outbox、Campaign Recipient/Delivery 和 Import Manifest/Row Count。
- 所有后台任务支持暂停、恢复和按稳定幂等键重放。
- `unknown` 外部副作用必须先人工或 Provider 对账，再由明确操作决定是否重试。

## 21. 性能与验收基线

- 单 Workspace 最多 1,000 万 Customer。
- 持续 2,000 Event/s，峰值 5,000 Event/s。
- 触发事件到生成投递命令 P95 小于 3 秒，不含 Provider 响应时间。
- 单个同步微批默认 10,000 条。
- 单个异步 Batch 默认 100,000 条。
- 单个文件导入默认 1,000 万行；标准压测环境目标 60 分钟内完成。
- 单次全量 Campaign 支持 1,000 万目标 Customer。
- Worker、RabbitMQ 或 Provider 短时故障恢复后，无已确认数据丢失、无重复外部发送。
- 多 Worker 并发下，频控不得超发。
- Workspace 混合负载下，大 Workspace 不得长期饿死小 Workspace。

## 22. 测试策略

### 22.1 单元与契约测试

- 客户编号格式、Workspace 序号不复用和 UUID 尾部。
- 外部用户 ID、身份规范化、冲突和匿名合并。
- Profile `set/merge/unset` 语义。
- Event 幂等、载荷冲突和迟到标记。
- Segment 条件编译与依赖索引。
- Campaign 受众冻结、暂停恢复和 A/B 稳定分组。
- Journey 版本、租约、状态版本和节点副作用幂等。
- 四类频控桶、Customer 收紧覆盖和并发预留。
- Webhook 签名、Nonce、回执和错误分类。
- Import Manifest、分片重试、检查点和全量对账。

### 22.2 集成与故障测试

- PostgreSQL 提交与 Outbox 原子性。
- RabbitMQ publish confirm、redelivery、DLQ 和恢复。
- Redis 故障时 Policy Gate fail-closed/defer。
- ClickHouse 中断后的可重建投影。
- Worker 在外部请求前、请求后和确认前崩溃。
- 10 万条 Batch 中断重试无缺失、无重复 Customer。
- 1,000 万行导入分片失败后最终对账一致。
- 全量 Campaign 暂停、恢复和 Worker 切换不重复发送。
- Workspace 隔离和权限越权测试。

### 22.3 负载测试

- 2,000 Event/s 稳态与 5,000 Event/s 峰值。
- 1,000 万 Customer 客群全量校准。
- 1,000 万目标 Campaign 冻结与分片。
- 多 Workspace 混合流量、公平扫描和积压恢复。
- 多 Worker 同时命中同一 Customer 的频控竞争。

## 23. 分阶段交付顺序

每个阶段必须保持仓库可构建、迁移可滚动、相关单元与集成测试通过，并提供独立可验收结果。

1. **品牌与工程身份：** Logo、产品名、中文语言包、导航、module path、镜像和环境变量兼容。
2. **Customer UUID 与身份层：** Workspace 序号、客户编号、Customer/Profile/Identity/Consent 表、Email 兼容投影和双写。
3. **Profile/Event API：** 外部用户 ID 直用、Event Ledger UUID 化、幂等冲突和兼容 API。
4. **无丢失接入：** 大 Batch、文件导入、对象存储 Manifest、分片 Worker、后台配置、对账和错误报告。
5. **动态客群 UUID 化：** 条件扩展、依赖索引、增量重算、全量校准和成员迁移。
6. **Campaign 多渠道化：** Broadcast 迁移、版本、Run、受众冻结、全量模式、周期调度和 Recipient 状态。
7. **Journey 增强：** UUID 实例、事件/客群/时间触发、不可变版本和渠道节点统一。
8. **四层频控与 Policy Gate：** 活动、事件/定时、全量、全局频控，静默时间、同意和抑制。
9. **通用渠道 Webhook：** 配置、签名发送、回执、重试和 Journey 分支。
10. **分析与运营：** 标准事件、ClickHouse 投影、Campaign/Journey 报表、客户时间线和异常中心。
11. **容量验收与主链切换：** 故障注入、负载测试、影子比对、分 Workspace 切换和回滚验证。

## 24. 明确不在首期范围

- 新增营销活动审批、双人复核或创建人与审核人分离。
- 自动合并两个已知 Customer。
- 跨 Workspace Customer 画像或统一身份图谱。
- 将 Mautic 或 Laudspeaker 作为运行时依赖。
- 首期实现全部 WhatsApp、Telegram、In-App、APNs 和 Web Push 原生 Provider；这些渠道先通过通用 Webhook 或已有端点能力接入。
- 用 ClickHouse 替代 PostgreSQL 在线资格与运行状态查询。
- 在完成兼容迁移前删除所有 Email 字段和旧 API。

## 25. 关键验收结论

本设计完成后，瑶光应具备以下闭环：业务系统可以使用 Workspace 内唯一的外部用户 ID 实时或批量写入 Customer Profile 和 Event；系统以 UUID 和瑶光客户编号维护统一身份；动态客群、Campaign 和 Journey 使用同一数据与执行底座；所有投递经过同意、静默时间和四层频控；原生渠道与通用 Webhook 共享幂等、回执和分析；任何被系统确认接收的数据或名单都有可核对的持久化状态，不会静默丢失。
