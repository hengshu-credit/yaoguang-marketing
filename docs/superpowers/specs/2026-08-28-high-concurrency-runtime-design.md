# Notifuse 高并发实时营销底座设计

**状态：** 待最终审阅
**目标分支：** `dev`
**目标版本：** `v40.0`
**设计日期：** 2026-08-28

## 1. 目标

将 Notifuse 从单进程、数据库轮询和“每个自动化一个数据库触发器”的执行模式，升级为可以横向扩展的实时营销运行时，同时保留现有邮件能力、工作区数据库隔离、RPC API 和 `dev` 分支已经实现的 scoped API keys、权限与受管 Webhook。

本次交付必须同时提供：

- PostgreSQL 17：唯一在线事实源；
- PgBouncer：API 与工作进程的连接池代理；
- RabbitMQ 4：耐久事件和任务消息；
- Redis 7：跨实例频控、缓存和短期协调；
- ClickHouse：可重建的事件分析投影；
- MinIO/S3：多渠道素材对象存储；
- 一个 Go 制品、多运行角色；
- 新旧链路双写、影子对比、主链切换和快速回滚；
- 事件、旅程和外部副作用的确定性幂等。

## 2. 范围

### 2.1 本次范围

- Docker Compose 高并发拓扑与健康检查；
- 配置、依赖客户端和启动角色；
- 工作区事件账本、transactional outbox、consumer inbox；
- 事件消息契约与 RabbitMQ 拓扑；
- 自动化触发规则依赖索引；
- 影子匹配、差异审计与切换开关；
- 旅程任务的原子认领、租约、版本和副作用幂等；
- Redis 频控/缓存基础能力；
- ClickHouse 事件投影；
- MinIO/S3 素材存储抽象；
- 指标、日志、故障恢复与端到端测试。

### 2.2 不在本次范围

- SMS、APNs/FCM、WhatsApp 等新渠道的完整产品页面；
- 新旅程节点的前端编辑器；
- 联系人身份图谱和匿名转实名 API 的完整产品实现；
- 用 ClickHouse 替代 PostgreSQL 在线查询；
- 引入 MongoDB、Kafka 或 Redpanda；
- 改变现有公开 RPC API 路径。

上述能力可以在本底座上继续实现，但不能扩大本次迁移的故障面。

## 3. 方案选择

### 3.1 未采用：复制 Laudspeaker 双存储主链

不采用 ClickHouse 写入后再通过 RabbitMQ 反向镜像到 PostgreSQL，也不把同一事件广播给全部活跃旅程。该方案会产生两个在线事实源，并使单事件成本随旅程数量线性增长。

### 3.2 采用：PostgreSQL 权威状态 + 专职基础设施

各组件职责固定如下：

| 组件 | 权威数据 | 职责 | 明确禁止 |
| --- | --- | --- | --- |
| PostgreSQL | 是 | 用户、事件账本、outbox/inbox、旅程状态、幂等记录 | 不把分析查询放入发送热路径 |
| PgBouncer | 否 | transaction pooling、连接数隔离 | 不使用 session 级状态依赖 |
| RabbitMQ | 否 | 事件与命令传递、背压、重试、故障隔离 | 不作为业务事实源 |
| Redis | 否 | 跨实例频控、热点规则缓存、短期去重加速 | 不保存权威旅程状态 |
| ClickHouse | 否 | 事件明细、漏斗、归因和分析投影 | 不反向同步 PostgreSQL |
| MinIO/S3 | 素材二进制 | 素材、附件和预览截图 | 不保存模板业务元数据 |

## 4. 总体架构

```text
External Systems / SDK
          |
          v
       API role
          |
          | same PostgreSQL transaction
          +--> domain mutation / contact_timeline compatibility
          +--> event_ledger
          +--> event_outbox
                    |
                    v
             outbox-relay role
                    |
             RabbitMQ confirms
                    |
        +-----------+-------------+
        |                         |
        v                         v
   rule-worker              analytics-worker
        |                         |
        v                         v
 automation match              ClickHouse
        |
        v
 contact_automations + outbox
        |
        v
 journey-worker
        |
        v
 side_effect_executions + delivery command
        |
        v
 delivery-worker --> provider --> receipt event
```

## 5. 运行角色

同一个镜像和 Go 二进制通过 `NOTIFUSE_ROLE` 选择角色：

- `api`：HTTP、SMTP bridge 和同步业务入口；
- `outbox-relay`：公平扫描工作区 outbox，带 publisher confirm 发布消息；
- `rule-worker`：消费领域事件，索引候选自动化并决定入组；
- `journey-worker`：认领旅程实例并执行纯状态节点；
- `delivery-worker`：执行邮件和 Webhook 等外部副作用；
- `analytics-worker`：批量写入 ClickHouse；
- `scheduler`：发布到期旅程和维护命令；
- `all`：在一个进程中启动所有角色，仅供开发和小规模兼容部署。

每个角色只初始化自己需要的 handler、worker 和外部依赖。`api` 不启动后台发送循环；worker 不监听公开 HTTP 端口，但提供独立健康和指标端口。

## 6. 配置模型

新增配置分组：

```text
NOTIFUSE_ROLE=all|api|outbox-relay|rule-worker|journey-worker|delivery-worker|analytics-worker|scheduler
REALTIME_MODE=legacy|shadow|primary

RABBITMQ_URL=amqp://...
RABBITMQ_PREFETCH=100
RABBITMQ_PUBLISH_CONFIRM_TIMEOUT=5s

REDIS_ADDR=redis:6379
REDIS_PASSWORD=
REDIS_DB=0

CLICKHOUSE_ADDR=clickhouse:9000
CLICKHOUSE_DATABASE=notifuse
CLICKHOUSE_BATCH_SIZE=1000
CLICKHOUSE_FLUSH_INTERVAL=1s

S3_ENDPOINT=http://minio:9000
S3_BUCKET=notifuse-assets
S3_REGION=us-east-1
S3_ACCESS_KEY=...
S3_SECRET_KEY=...
S3_FORCE_PATH_STYLE=true

JOURNEY_LEASE=60s
JOURNEY_HEARTBEAT=20s
OUTBOX_BATCH_SIZE=200
OUTBOX_LEASE=30s
```

生产环境的 `primary` 模式要求 RabbitMQ 可配置且凭证不是示例值。ClickHouse 不可用时只积压分析消息，不阻塞在线事件和触达。Redis 不可用时读取 PostgreSQL 权威规则；需要频控的外部发送必须延迟，不能绕过频控。MinIO 不可用时禁止新素材上传，但不影响已有内联邮件发送。

## 7. PostgreSQL 数据模型

所有下列表位于每个工作区数据库，延续现有租户隔离。

### 7.1 `event_idempotency`

PostgreSQL 的分区表唯一约束必须包含分区键，因此用一个非分区注册表保证 event ID 在整个工作区唯一：

- `id UUID PRIMARY KEY`；
- `received_at TIMESTAMPTZ NOT NULL`；
- `payload_hash TEXT NOT NULL`；
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`。

相同 ID 和相同 payload hash 视为幂等重试；相同 ID 但 hash 不同返回冲突，不能覆盖原事件。

### 7.2 `event_ledger`

不可变事件事实：

- `id UUID NOT NULL`：调用方 event ID 或服务端 UUIDv7；
- `event_type TEXT NOT NULL`；
- `subject_type TEXT NOT NULL`；
- `subject_id TEXT NOT NULL`；
- `contact_email TEXT NULL`：兼容现有联系人主身份；
- `source TEXT NOT NULL`；
- `schema_version INTEGER NOT NULL DEFAULT 1`；
- `occurred_at TIMESTAMPTZ NOT NULL`；
- `received_at TIMESTAMPTZ NOT NULL DEFAULT now()`；
- `sequence BIGINT NULL`；
- `properties JSONB NOT NULL DEFAULT '{}'`；
- `context JSONB NOT NULL DEFAULT '{}'`；
- `timeline_id UUID NULL UNIQUE`；
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`；
- 主键 `(received_at, id)`。

全局 event ID 唯一性由 `event_idempotency` 保证。同一来源需要顺序控制时先进入非分区顺序注册表，再写 ledger；在线索引覆盖 `(event_type, received_at)`、`(subject_type, subject_id, occurred_at)`。ledger 按 `received_at` 月分区，迁移同时创建当前月、前一月和未来两个月分区。

### 7.3 `event_outbox`

- `id UUID PRIMARY KEY`；
- `event_id UUID NOT NULL`；
- `topic TEXT NOT NULL`；
- `routing_key TEXT NOT NULL`；
- `payload JSONB NOT NULL`；
- `headers JSONB NOT NULL DEFAULT '{}'`；
- `status TEXT NOT NULL`：`pending|claimed|published|dead`；
- `attempts INTEGER NOT NULL DEFAULT 0`；
- `available_at TIMESTAMPTZ NOT NULL DEFAULT now()`；
- `claimed_by TEXT NULL`；
- `claim_token UUID NULL`；
- `claim_expires_at TIMESTAMPTZ NULL`；
- `published_at TIMESTAMPTZ NULL`；
- `last_error TEXT NULL`；
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`。

`(event_id, topic, routing_key)` 唯一。认领使用一个 `UPDATE ... FROM (SELECT ... FOR UPDATE SKIP LOCKED) RETURNING` 语句，不能先查后改。

### 7.4 `consumer_inbox`

- `consumer TEXT NOT NULL`；
- `message_id UUID NOT NULL`；
- `status TEXT NOT NULL`：`processing|completed|failed`；
- `attempts INTEGER NOT NULL DEFAULT 0`；
- `claim_token UUID NOT NULL`；
- `claim_expires_at TIMESTAMPTZ NOT NULL`；
- `processed_at TIMESTAMPTZ NULL`；
- `last_error TEXT NULL`；
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`；
- 主键 `(consumer, message_id)`。

消费者在业务事务内插入 inbox、执行状态变化并标记完成。重复消息读取已有完成记录后直接 ACK；过期 processing 记录可以被新 claim token 接管。

### 7.5 `automation_trigger_bindings`

自动化发布时编译：

- `automation_id TEXT NOT NULL`；
- `automation_version INTEGER NOT NULL`；
- `event_type TEXT NOT NULL`；
- `subject_type TEXT NOT NULL`；
- `dependency_keys TEXT[] NOT NULL DEFAULT '{}'`；
- `condition_hash TEXT NOT NULL`；
- `compiled_condition JSONB NOT NULL`；
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`；
- 主键 `(automation_id, automation_version, event_type, subject_type)`。

规则 worker 先用 `(event_type, subject_type)` 查候选，再只评估属性依赖有交集的规则。Redis 缓存键包含工作区、事件类型和自动化版本；缓存失效只影响性能，不影响正确性。

### 7.6 旅程租约

在现有 `contact_automations` 增加：

- `origin_event_id UUID NULL`；
- `automation_version INTEGER NOT NULL DEFAULT 1`；
- `state_version BIGINT NOT NULL DEFAULT 0`；
- `claim_token UUID NULL`；
- `claimed_by TEXT NULL`；
- `claimed_at TIMESTAMPTZ NULL`；
- `claim_expires_at TIMESTAMPTZ NULL`。

认领必须是单条原子更新。每次状态写入同时校验 `id + claim_token + state_version + status='active'`，成功后递增 `state_version`。租约心跳只能延长自己的 claim token。

### 7.7 `side_effect_executions`

- `effect_key TEXT PRIMARY KEY`；
- `contact_automation_id TEXT NOT NULL`；
- `automation_version INTEGER NOT NULL`；
- `node_id TEXT NOT NULL`；
- `execution_version BIGINT NOT NULL`；
- `channel TEXT NOT NULL`；
- `status TEXT NOT NULL`：`reserved|submitted|confirmed|failed|unknown`；
- `provider_message_id TEXT NULL`；
- `request_hash TEXT NOT NULL`；
- `attempts INTEGER NOT NULL DEFAULT 0`；
- `last_error TEXT NULL`；
- `created_at`、`updated_at`。

`effect_key = workspace_id + contact_automation_id + automation_version + node_id + execution_version + channel` 的稳定哈希。重试和消息重放必须复用同一个 key。

### 7.8 `automation_match_audit`

保存 `legacy` 与 `realtime` 两套引擎的逐事件判断：

- `event_id`、`automation_id`、`engine`；
- `matched BOOLEAN`；
- `decision_hash TEXT`；
- `contact_automation_id TEXT NULL`；
- `reason JSONB`；
- `created_at`；
- 唯一 `(event_id, automation_id, engine)`。

影子模式不产生外部副作用，只写 `realtime` 审计结果。旧触发函数写入入组记录时同步写 `legacy` 审计结果，使差异可以按同一 `event_id` 精确比较。

## 8. 兼容事件桥

现有大量业务变化通过数据库触发器写 `contact_timeline`。为避免一次重写所有业务仓库，v40 创建一个固定、O(1) 的 `contact_timeline -> event_ledger + event_outbox` 兼容桥：

1. `contact_timeline` 新增 `origin_event_id UUID NOT NULL DEFAULT gen_random_uuid()`；
2. 单一固定触发器把 timeline 行规范化为 event ledger；
3. 同一数据库事务内写 outbox；
4. event idempotency、`timeline_id` 与 outbox 唯一约束阻止重复，桥接写入使用 `ON CONFLICT DO NOTHING`；
5. 新版显式事件 API 可以先写 ledger/outbox，再以同一 `origin_event_id` 写兼容 timeline；
6. 该桥不包含任何自动化规则，因此单次写入成本不随自动化数量增加。

迁移改变 `contact_timeline` 后，必须按项目规范重新生成所有存量自动化触发函数。切到 `primary` 后删除每自动化触发器，仅保留兼容事件桥和分群等固定系统触发器。

## 9. 消息契约

统一消息 envelope：

```json
{
  "id": "uuid",
  "type": "contact.updated",
  "schema_version": 1,
  "workspace_id": "workspace-id",
  "subject": {"type": "contact", "id": "subject-id"},
  "source": "notifuse.api",
  "occurred_at": "RFC3339Nano",
  "received_at": "RFC3339Nano",
  "correlation_id": "uuid",
  "causation_id": "uuid-or-null",
  "trace_id": "trace-id",
  "data": {}
}
```

消息 ID 等于 outbox ID；业务事件 ID 在 envelope 和 payload 中保持不变。消费者不能依赖 RabbitMQ delivery tag 作为业务幂等标识。

RabbitMQ 拓扑：

- topic exchange `notifuse.events`；
- direct exchange `notifuse.commands`；
- direct exchange `notifuse.retry`；
- direct exchange `notifuse.dlx`；
- quorum queues：`events.rule`、`events.analytics`、`commands.journey`、`commands.delivery.email`、`commands.delivery.webhook`；
- 固定 TTL 重试队列：5 秒、30 秒、5 分钟、30 分钟；到期通过 DLX 回原目标；
- 每个最终死信进入对应 `.dead` quorum queue；
- publisher 使用 confirm channel，confirm 超时保持 outbox 为 pending；
- consumer `noAck=false`，prefetch 可配置，业务事务完成后 ACK。

不依赖 RabbitMQ delayed-message 插件，保证官方镜像即可启动。

## 10. 事件到旅程的数据流

### 10.1 接入

API 完成验证后，在工作区事务内写业务变化、event ledger 和 outbox。返回成功表示事件已经耐久写入 PostgreSQL，不表示旅程已执行。相同 event ID 重试返回原接收结果。

### 10.2 Outbox Relay

Relay 维护持久公平工作区游标，按工作区轮转认领 outbox。发布成功并收到 broker confirm 后，使用 claim token 把记录标为 published。进程在 confirm 后、数据库更新前崩溃会产生重复消息，由 inbox 去重。

### 10.3 规则匹配

`rule-worker`：

1. 认领 inbox；
2. 从 Redis 或 PostgreSQL 获取候选 binding；
3. 仅评估候选规则；
4. 在 `shadow` 模式写审计；
5. 在 `primary` 模式原子创建 contact automation、节点初始状态和 journey command outbox；
6. 完成 inbox 后 ACK。

一个事件的计算量与候选规则数量有关，不与工作区全部活跃自动化数量相关。

### 10.4 旅程执行

RabbitMQ command 只起唤醒作用。`journey-worker` 必须先在 PostgreSQL 原子取得租约，再读取并执行状态。纯状态节点在同一事务内提交下一状态和下一条 outbox。等待节点只写 `scheduled_at`，由 scheduler 到期后发布命令，不占 worker goroutine。

### 10.5 外部副作用

邮件/Webhook 节点先在 PostgreSQL reserve `effect_key`，然后发送。状态语义：

- 服务商明确接受：`submitted`，保存 provider message ID；
- 服务商明确拒绝：按策略重试或 `failed`；
- 请求超时且结果未知：`unknown`，优先用服务商幂等键/查询接口确认，不能生成新 effect key；
- 服务商回执：更新为 `confirmed` 并产生回执事件。

## 11. ClickHouse 投影

ClickHouse 表按月分区：

```text
PARTITION BY toYYYYMM(received_at)
ORDER BY (workspace_id, event_type, subject_type, subject_id, occurred_at, id)
```

表引擎使用 `ReplacingMergeTree(projected_at)`，字段包括 envelope 核心列和 JSON `data`。`analytics-worker` 批量插入，并以 event ID 作为逻辑去重键；面向最终一致性的报表使用聚合视图或带去重语义的查询，不能假设 MergeTree 写入时立即物理去重。ClickHouse 数据可以从 PostgreSQL event ledger 或 RabbitMQ 重放重建。删除 ClickHouse 数据库不能影响事件接收、旅程入组或渠道发送。

## 12. Redis 语义

Redis 键均带工作区 namespace 和明确 TTL：

- 规则缓存：版本化 key，发布自动化时主动失效；
- 频控：Lua 脚本或事务实现滑动窗口/令牌桶；
- 热点幂等：只减少 PostgreSQL 查询，不替代 inbox/effect 唯一约束；
- 分布式短锁：只用于重建缓存或维护任务，不用于旅程权威锁。

Redis 故障时：规则读取回源 PostgreSQL；缓存去重回源 PostgreSQL；发送频控无法确认时延迟任务并报警，避免超发。

## 13. MinIO/S3 语义

新增 `ObjectStore` 小接口：`Put`、`Get`、`Delete`、`PresignGet`。业务元数据仍在 PostgreSQL，二进制进入 MinIO/S3。对象键必须包含工作区 ID、素材 ID 和版本；下载使用短期签名 URL。删除工作区时通过任务异步清理 prefix，并保留失败重试记录。

## 14. 模式切换

### 14.1 `legacy`

- 现有每自动化数据库触发器负责入组；
- 新 event ledger/outbox 可持续写入；
- realtime worker 不创建旅程，只做基础设施健康验证。

### 14.2 `shadow`

- 旧触发器继续生产真实入组和发送；
- 新 matcher 写 `automation_match_audit`，不创建真实旅程；
- 差异指标按 event ID、自动化版本和原因聚合。

### 14.3 `primary`

- 新 matcher 创建真实旅程；
- 旧每自动化触发器停用；
- 固定 timeline 兼容桥保留；
- 回滚到 shadow/legacy 不删除任何新数据。

模式是进程启动配置，变更需要滚动重启，避免不同 API/worker 实例同时使用不同主链。启动时将模式和运行角色写入指标与结构化日志。

## 15. 切换验收门槛

从 shadow 切换 primary 前同时满足：

- 连续 24 小时影子流量；
- 可解释匹配一致率不低于 99.99%；
- 不可解释漏入组为 0；
- 重放同一事件不会重复创建 contact automation；
- worker 在发送前、发送后、ACK 前崩溃均不产生重复外部副作用；
- 事件接收 p95 小于 200ms；
- 事件到入组 p95 小于 2s、p99 小于 5s；
- 任一租户持续高流量时其他租户没有饥饿；
- RabbitMQ、Redis、ClickHouse、MinIO 和单个 worker 故障的行为符合本规格；
- rollback 演练通过，旧链路可以在一次滚动重启内恢复。

## 16. 安全与租户隔离

- 沿用 `dev` 分支 scoped API keys；新增事件接入只接受对应写 scope；
- RabbitMQ routing key 不包含邮箱或其他直接个人数据；
- ClickHouse 行包含 workspace ID，所有查询必须由服务端注入 workspace filter；
- Redis key、S3 object key 和指标标签均包含稳定工作区 ID；
- 基础设施凭证只能从环境变量/secret 文件注入；Compose 不提供可用于生产的固定凭证；
- 管理端口默认只暴露到 Docker 内部网络；
- 消息和日志不得记录 API key、SMTP 密码、完整授权头或未脱敏个人数据。

## 17. 可观测性

新增指标：

- event ingest、outbox pending/oldest age/publish latency；
- RabbitMQ publish confirm、redelivery、retry、dead letter；
- rule candidate count、match latency、shadow mismatch；
- journey queue lag、claim conflict、lease expiry、state transition；
- side-effect duplicate suppressed、unknown outcome、provider latency；
- Redis fallback、ClickHouse batch/lag、S3 error；
- 按工作区的公平调度 lag，但不把工作区 ID作为无限基数公共指标标签；详细租户信息进入结构化日志。

所有 envelope 传播 `correlation_id`、`causation_id` 和 `trace_id`。

## 18. 故障与恢复

| 故障 | 在线行为 | 恢复方式 |
| --- | --- | --- |
| RabbitMQ 不可用 | API 继续写 PostgreSQL；outbox 积压 | relay 自动重连并重放 |
| Relay confirm 后崩溃 | 可能重复投递 | inbox 去重 |
| Rule/Journey worker 崩溃 | 消息重新投递；租约到期 | claim token 接管 |
| Redis 不可用 | 规则回源；发送延迟 | 恢复后继续，不超发 |
| ClickHouse 不可用 | 分析队列积压 | projector 批量追平 |
| MinIO 不可用 | 禁止上传新素材 | 已有内联发送继续 |
| PostgreSQL 不可用 | API 返回可重试错误；worker 不 ACK | 数据库恢复后重放 |

## 19. 测试策略

所有生产代码遵循测试先行：先看到测试因能力缺失而失败，再写最小实现。

- domain：消息 envelope、状态机、幂等 key、配置校验；
- repository：outbox/inbox 原子认领、租约、版本冲突、唯一约束、迁移幂等；
- service：relay confirm、消费者 ACK/重试、规则候选、shadow/primary、Redis fallback；
- integration：真实 PostgreSQL、PgBouncer、RabbitMQ、Redis、ClickHouse、MinIO；
- concurrency：多 worker 同时认领同一任务，只有一个副作用；
- fault injection：在事务提交、publish confirm、provider 返回和 ACK 边界终止进程；
- migration：从 v39 数据库升级 v40、重复执行、存量自动化触发器再生成和回滚开关；
- load：租户混合负载下验证延迟、公平性、积压恢复和连接数。

## 20. 交付顺序

1. 基础设施 Compose、配置和运行角色；
2. v40 数据表、固定 timeline 事件桥和迁移测试；
3. RabbitMQ 拓扑、outbox relay、inbox 与 DLQ；
4. 规则 binding 编译、shadow matcher 和差异审计；
5. 旅程原子租约、调度命令和 side-effect ledger；
6. Redis 频控/缓存、ClickHouse projector、MinIO object store；
7. 端到端故障测试、负载测试和 primary 切换；
8. 删除每自动化触发器的 primary 路径，保留回滚配置。

每一步必须保持仓库可构建、相关测试通过，并且不能破坏 `dev` 分支现有权限、Webhook、邮件和控制台行为。
