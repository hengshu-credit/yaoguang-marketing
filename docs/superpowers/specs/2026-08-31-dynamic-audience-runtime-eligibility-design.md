# Dynamic Audience Runtime Eligibility Design

## Purpose

客群划分从“维护名单”扩展为直接配置客户属性、客户状态、名单状态、行为事件和目标事件条件。客群配置阶段只保存筛选逻辑和版本，不持续维护成员；营销任务真正启动时使用客群最新版本生成候选客户快照；每次实际触达前再使用启动时记录的规则版本和客户最新事实判断资格。

本设计同时覆盖群发活动和自动化流程。群发和自动化只能消费已经完成的候选快照，自动化中的每个触达节点都独立复核；复核失败只跳过本次触达，旅程继续。

## Confirmed Product Semantics

- 客群定义保存后不产生持续入群、退群或成员物化任务。
- 条件编辑期间，每次得到完整可查询的条件树后自动试算整棵树的当前匹配人数。
- 预约任务在计划真正开始执行时圈选，而不是在预约或发布时圈选。
- 任务启动时解析客群当时最新的 `active_version`，并把实际版本写入运行记录。
- 候选快照一旦完成便不可变；之后客户资料或客群定义变化不增删候选人。
- 每次触达前使用运行记录中的客群版本重新读取客户当前事实。
- 复核失败的触达记录为终态跳过，原因是 `audience_no_longer_matched`；重试不会补发该次触达。
- 自动化复核失败只跳过当前邮件、短信或 Push 节点，然后继续执行后续节点；后续触达再次复核。

## Existing Capabilities to Reuse

- `domain.TreeNode`、`service.QueryBuilder` 和 Console `TreeNodeInput` 已覆盖客户字段、外部画像状态与属性、标签、名单状态、时间线事件和目标事件的 AND/OR 条件树。
- `segments.preview` 已证明完整树和编辑中草稿树的自动试算交互，包括条件不完整时保留并弱化最后有效人数。
- `Audience` 已有不可变版本、结构化定义、预览、构建和成员表。
- `CampaignRun` 与 `campaign_recipient_snapshots` 已有候选快照状态机、稳定顺序和 A/B 变体分配。
- Broadcast 队列和 Automation 节点执行器已有幂等消息意图与节点输出，可承载资格跳过结果。

## Domain Model

### Audience definitions

`AudienceExpression` 增加一种内嵌条件叶子：

```json
{
  "condition": {
    "kind": "branch",
    "branch": {
      "operator": "and",
      "leaves": []
    }
  }
}
```

一个表达式节点必须且只能是以下三者之一：

1. 引用叶子：`leaf_type + ref_id`，继续兼容名单、旧 Segment 和其他 Audience；
2. 条件叶子：`condition`，内容是完整 `TreeNode`；
3. 组合节点：`operator + children`，继续支持并集、交集和排除。

新建条件客群默认 `kind=dynamic`。保存新版本只写 `audience_versions` 并推进 `audiences.active_version`，不创建 `audience_builds`。

### Runtime resolution

活动配置只引用 `audience_id`。任务实际启动时按以下顺序解析：

1. 锁定活动运行，防止并发启动重复创建快照；
2. 读取 Audience 当前 `active_version`；
3. 把 `audience_id`、已解析版本和冻结开始时间写入运行记录；
4. 创建本次运行专属的 Audience build；
5. 在数据库事务内将当前匹配的 `customer_id` 写入 `audience_memberships`；
6. 记录候选人数并将 build 标记为 completed；
7. 下游从该精确 `build_id` 生成或消费运行快照。

每个营销运行都拥有独立 build。运行不得通过“最新 completed build”推断来源，因为这会把并发或后续运行的成员混入当前任务。

### Candidate snapshot versus eligibility

候选快照回答“任务开始时谁属于目标客群”；触达资格回答“这个候选人在此刻是否仍符合启动时采用的规则”。两者独立记录：

- `candidate_count`：冻结的候选人数；
- `eligible_count`：触达前仍匹配并进入消息意图的次数；
- `skipped_count`：触达前不再匹配而跳过的次数；
- 跳过原因固定为 `audience_no_longer_matched`。

## Query Compilation

条件编译必须复用 QueryBuilder 的字段白名单、参数绑定和时间/事件语义，不能存储或接受客户端 SQL。

QueryBuilder 增加两个公共能力：

```go
BuildCustomerIDSQL(tree *domain.TreeNode, placeholderOffset int) (string, []interface{}, error)
MatchesCustomer(ctx context.Context, db *sql.DB, tree *domain.TreeNode, customerID string) (bool, error)
```

前者从 `contacts.customer_id` 返回去重的权威 Customer ID，并支持嵌入 Audience 的组合 SQL 时正确偏移占位符。后者使用同一条件编译结果并限定单个 Customer，用于触达前复核。

旧 Segment 的邮件地址结果和持续物化流程保持兼容；新 Audience 不调用 Segment build/recompute。

## Snapshot Consistency

条件型 Audience build 使用单个数据库事务内的 `INSERT ... SELECT` 生成成员，确保所有候选来自同一个 PostgreSQL 事务快照。不得用跨事务分页重新执行动态条件，否则长任务期间的资料变化会产生既非开始时也非结束时的混合名单。

引用已物化名单或旧 Segment 的表达式继续参与集合运算；引用另一个 Audience 时解析其当前 active build 仅用于兼容旧组合定义。新条件型活动总是创建运行专属 build。

## Broadcast Flow

预约或立即发送时不再提前生成 Audience 快照。`send_broadcast` 任务第一次真正执行时：

1. 若 Broadcast 选择名单，保持现有名单快照行为；
2. 若选择 Audience 且没有 `campaign_run_id`，解析最新版本并创建运行专属 build；
3. Campaign Run 明确保存 `audience_id`、`audience_version` 和 `audience_build_id`；
4. 仅从该 build 生成 `campaign_recipient_snapshots`；
5. 快照完成后进入 dispatching。

每个候选人在写入 Delivery Intent / Email Queue 之前调用资格检查。匹配时沿用现有发送路径；不匹配时写入幂等的跳过决策并把该候选计为已处理，不创建外部发送意图。

资格检查失败与不匹配不同：数据库错误属于可重试错误，不能静默跳过；明确返回 false 才是业务跳过。

## Automation Flow

自动化增加“从客群启动”入口。启动任务解析 Audience 最新版本、生成运行专属 build，然后按快照顺序为候选客户创建旅程实例。每个实例保存来源 Audience run/build 和已解析版本，使后续节点无需读取 Audience 的最新版本。

Email、SMS 和 Push 节点在建立消息意图前调用公共资格检查：

- 匹配：正常创建幂等消息意图并继续；
- 不匹配：写入成功完成的节点执行结果，包含 `skipped=true` 和 `skip_reason=audience_no_longer_matched`，然后返回该节点正常的 `NextNodeID`；
- 检查错误：节点执行失败并按现有策略重试。

非触达节点，包括延迟、分支、过滤、名单变更和 Webhook，不执行该资格检查。

## Transaction and Race Boundary

资格查询和本地消息意图写入应处于同一数据库事务或同一仓储原子操作中。这样，本数据库已收到的还款状态变化不会在“检查通过、写意图之前”插入。

发送队列 worker 仍可能晚于意图创建；如果业务要求直到调用外部渠道 API 的最后一刻都阻止发送，队列条目必须携带 Audience 运行版本并在 provider 调用前再次检查。第一阶段在消息意图/入队前检查，并在队列条目保留资格上下文，为 worker 最终闸门提供接口。外部还款系统尚未同步到本数据库的状态不可能由本系统判断，必须监控同步延迟。

本次实施同时包含入队前检查和邮件 worker 的 provider 前最终闸门。队列负载只携带 Audience ID、启动时版本、build ID 和权威 Customer ID；worker 在渠道调用前重新查询当前事实。明确不匹配会把已排队 Delivery Intent 原子转为 `suppressed` 并记录 `audience_no_longer_matched`，查询错误则释放队列租约并重试，绝不降级为发送。

## Console Experience

恢复独立 `/audiences` 页面，侧边栏“客群划分”指向该页面；`/lists` 继续作为名单管理页面。

客群页面包含：

- 已保存客群列表：名称、说明、类型、当前版本；不显示伪造的持久成员数；
- 创建/编辑抽屉：名称、说明、时区和 `TreeNodeInput` 条件树；
- 实时人数：完整条件树或完整草稿树变化后防抖调用 Audience preview；
- 不完整条件：不请求后端，保留最后有效人数并弱化显示；
- 预览错误：保留最后有效人数并显示可重试错误；
- 保存：仅创建/更新定义，不显示“构建客群”或触发成员构建。

群发配置支持在“名单”和“动态客群”之间二选一。选择动态客群只提交 `audience_id`，版本与 build 在任务启动时解析；界面明确提示“执行开始时使用最新规则圈选，发送前再次判断资格”。

自动化页面增加从已发布流程发起客群运行的入口，展示客群选择、候选人数预览和启动确认。运行详情展示候选、已触达、资格跳过和失败数。

所有用户可见文本使用 Lingui。由于当前中文目录存在用户改动，实施时只增加源字符串与针对性翻译，不做会覆盖无关变更的机械目录重写。

## Permissions and Security

- 客群定义的读取、预览和运行继续使用 Segments 权限。
- 预览和资格查询还必须具备 Contacts read，防止把条件接口变成客户数据计数侧信道。
- 启动群发还需 Broadcasts write；启动自动化客群运行还需 Automations write。
- 条件树严格验证字段、操作符、值数量、事件类型和时间范围。
- 所有值参数化；字段名只能来自服务器白名单。

## Failure and Recovery

- 快照失败：运行状态 failed，不启动下游；保留 Audience 版本、错误和已创建 build 供审计和重试。
- 空快照：运行正常完成，候选人数为 0，不发送也不创建旅程。
- 资格不匹配：业务跳过，不计为失败，不重试该触达。
- 资格查询错误：保持待处理/失败状态并按既有任务策略重试。
- 重复任务投递：复用同一运行和 build；不得创建第二份候选或重复旅程/消息。
- 客群被归档：已启动运行仍使用记录版本；尚未启动的预约任务在执行时明确失败并提示客群不可用。

## Testing Strategy

- Domain：表达式三选一验证、条件树验证、版本哈希、运行版本固定。
- QueryBuilder：Customer ID SQL、占位符偏移、单客户匹配、属性/状态/事件/目标事件。
- Repository：条件预览、事务内 build、精确 build 消费、运行解析版本持久化。
- Service：配置不 build、任务启动解析最新版本、重复启动幂等、空客群。
- Broadcast：预约时不冻结、任务启动冻结、资格 false 终态跳过、错误重试、重投不补发。
- Automation：快照入旅程、每个触达节点复核、跳过后继续、后续触达重新判断。
- HTTP/OpenAPI：条件定义、预览、启动及运行状态契约。
- Console：条件树编辑中人数、最后有效人数、保存不 build、群发选择动态客群、自动化启动确认。
- Integration：客户启动时未还款进入候选，发送前写入已还款状态，最终没有消息意图且记录资格跳过。

## Non-goals

- 不把动态客群成员同步回名单。
- 不为客群持续产生 joined/left 事件。
- 不在运行中自动切换到后来发布的客群版本。
- 不在资格失败时退出整个自动化旅程。
- 不保证尚未同步到本系统的外部业务状态能够阻止发送。
