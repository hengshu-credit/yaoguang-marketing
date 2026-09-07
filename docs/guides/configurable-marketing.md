# 可配置客户、事件、状态与转化漏斗

客户以 `Customer UUID` 为唯一主键。邮箱、手机号、身份证及外部用户 ID 是客户属性或身份标识；客户可以没有邮箱。授信申请、借款订单等都是工作空间中配置的业务对象类型，数据库不为这些业务名称建立专用表。

## 页面入口

- **设置 → 客户与事件配置**：管理事件、客户属性、业务对象类型、对象关系、外部字段映射；可以先用样例 JSON 校验。
- **设置 → 客户与对象状态**：设置独立状态及规则，支持满足、不满足、未知、有效期与人工覆盖。
- **客户详情**：查看事件发生记录、关联对象、状态历史及客户触达端点。
- **营销任务与自动化旅程**：选择跟踪事件、状态、目标与统计对象；事件触发器单独设置触发频率。业务条件节点支持等待、满足时退出和满足时继续。
- **数据分析 → 转化漏斗**：拖拽步骤、保存漏斗、选择时间及统计范围，并查看异步查询结果。
- **执行详情**：查看每次实际运行保存的配置、筛选条件、素材、执行证据及所选转化指标。

所有新增界面文案进入 Lingui。英文与简体中文已填写；其他支持语言的新词条保留可编辑的英文回退。目录原有未翻译条目不在本次范围内。

## 重复事件与幂等

每一次实际行为都生成新的 `report_id`。同一个客户连续登录三次，应当有三条事件；即使事件编码、数据或业务对象相同，也不合并这些行为。

只有同一工作空间内相同 `source + report_id` 的**同一份请求重试**才返回第一次接收结果。修改该请求的载荷后重用 ID 返回 `409`。第一次服务端接收时间 `reported_at` 不随重试更新。SDK 会在重试期间保留 ID 和载荷，并在下次实际行为时生成新的 ID。

```json
{
  "workspace_id": "your-workspace",
  "source": "credit-app",
  "event": {
    "report_id": "login-occurrence-001",
    "event_code": "customer.login",
    "customer_attributes": {"customer_id": "00000000-0000-4000-8000-000000000001"},
    "data": {"device": "mobile", "phone": "+8613800000000"},
    "triggered_at": "2026-09-07T08:00:00Z"
  }
}
```

将上例发送到 `POST /api/events.track`。下一次登录更换 `report_id` 和实际发生时间；网络重试原样发送。`POST /api/events.batch` 每批最多接收 100 个独立请求。

`event_name` 可省略并使用配置名称；`reported_at` 由服务端产生；`expires_at` 根据事件配置可不要求、要求显式提供，或者按持续时间计算。事件接收、保留与任务触发相互独立：过期和历史回补事件仍可用于历史分析，但不产生新的实时营销动作。

邮件、短信、WhatsApp、推送和 Webhook 的原生事件进入受保护的默认目录，可直接用于任务跟踪。各渠道是否实际产生打开、点击、投诉等回执，取决于发送适配器及服务商提供的信号；配置事件本身不会补造回执。

## 配置数据定义

`POST /api/businessDefinitions.save` 接收 `workspace_id` 和 `definition`。`kind` 可为 `customer_attribute`、`object_type`、`object_relation`、`event`、`event_mapping`、`state` 或 `funnel`。编辑时传回最新 `revision`，处理并发编辑冲突；该字段仅用于定义的一致性检查，不是 campaign 版本。

下面是一个通用合同对象事件，不包含固定的信贷表结构：

```json
{
  "workspace_id": "your-workspace",
  "definition": {
    "kind": "event",
    "key": "contract.signed",
    "name": "签署合同",
    "status": "active",
    "schema": {
      "type": "object",
      "properties": {
        "contract_id": {"type": "string"},
        "amount_minor": {"type": "integer", "minimum": 0},
        "currency": {"type": "string"}
      },
      "required": ["contract_id"]
    },
    "settings": {
      "subject_type": "contract",
      "subject_id_path": "$.data.contract_id",
      "expiry_mode": "never",
      "unmatched_customer": "pending",
      "browser_enabled": false
    }
  }
}
```

先建立 `contract` 的 `object_type` 定义。对象按 `object_type + source + external_key` 定位，每个对象归属于一个 Customer UUID。`businessObjects.upsert/get/list/relate` 管理对象及配置的关系；对象关系可以是一对一、一对多或多对多。

属性支持文本、数字、整数、布尔、对象、数组，以及嵌套字段、必填、枚举、空值、格式和范围校验。外部映射的 `settings.fields` 指定 `$.path`，将任意外部 JSON 映射为规范事件。`attribute_mappings` 的每一项指定 `attribute`、`path` 和 `action`：

- `lookup`：按配置身份查找客户。
- `bind`：为已确定的客户绑定身份。
- `update`：更新客户属性，支持 `overwrite` 或 `set_union`。
- `record`：只保留本次事件的属性上下文。

`null_policy=ignore` 保留旧值；`clear` 表示显式清空。样例预览不写事件、不修改客户。敏感属性在无相应管理权限的事件读取中脱敏。

无法定位客户的事件可以进入 `pending_identity`。`events.resolvePending` 需要客户写权限并记录原因，使用第一次上报时冻结的定义和字段映射进行关联；并发处理只释放一次符合条件的实时事件，已经过期的记录仅保留历史。

## 浏览器事件

只有开启 `browser_enabled` 并配置精确 `allowed_browser_origins` 的事件可通过 `/track/events` 接收。业务权威事件应由受认证后端上报。

应用后端验证登录身份后，调用 `browserEvents.issueCredential`，绑定 Customer UUID、会话、精确 Origin 和允许的事件编码。签名凭证有效期为 15 分钟，提供给 SDK 的是受限浏览器凭证；工作空间 API 密钥不下发浏览器。没有有效身份凭证的允许事件作为匿名待关联记录保留。`/track/identify` 关联历史匿名记录时不重新启动营销。

## 状态、任务与漏斗

状态是可独立重叠的事实，不强制组成全集或互斥状态机。事件规则支持时间窗口、最少次数、撤销事件；属性、状态和子对象条件可以通过 `all`、`any`、`not` 组合。人工覆盖记录服务端当前 `valid_from`、可选的结束时间 `valid_to` 与原因，操作人由认证上下文记录。

任务的 `tracking_plan` 结构为：

```json
{
  "subject_type": "contract",
  "events": ["contract.signed", "contract.settled"],
  "states": ["contract.active"],
  "goal_events": ["contract.settled"],
  "window_seconds": 2592000
}
```

任务的“每次事件”或“每个对象仅一次”控制营销进入频率，不改变事件存储。同一对象的等待节点只接受与该次旅程绑定的对象；另一个申请或订单的事件不能满足当前等待。

漏斗以客户或配置对象作为入口队列，按发生时间串联。步骤支持 OR、无序并行、可选步骤、步骤窗口、事件属性条件和事件发生时的状态条件。转换到另一对象类型时通过配置关系绑定，后续步骤沿用该对象，避免拼接不同订单的过程。界面提供可编辑的获客、申请、支用、还款、催收和关联链路示例；示例中的事件编码、对象及关系必须替换或发布为当前工作空间定义。

“次数”保留重复行为；“对象数”去重统计业务对象。尚未观察满窗口的对象单独列为等待，不提前当作流失。统计范围可选择全站、某次运行实际接触客户的后续行为、已证明归因于该运行的转化。金额按币种分开并使用最小货币单位。

异步查询通过 `funnels.start/get/cancel` 运行，保存定义和数据水位。完成结果保持不变；失败或中断任务明确显示失败，用户可重新运行。

分析页的“状态覆盖”模式单独统计所选状态的满足、不满足、未知，以及这些状态的交集与并集。对象没有相应证据时计为未知；重叠状态不要求加总为 100%。对应接口为 `POST /api/subjectStates.coverage`，传入 `subject_type`、`selected_state_keys`、`as_of` 和 `watermark`，且观察时点不晚于数据水位。

## 每次实际运行的历史

Campaign 编辑当前配置；每次运行分别创建 `execution_id`。历史详情读取该次运行的快照和追加证据，不用当前配置替代历史。素材内容、依赖、模板与实际渲染/服务商结果通过 `executions.get/list/asset/observations` 查看。内部执行所需凭证加密归档，公开快照不返回秘密值。

静态 HTTP(S) 媒体在准备阶段归档；包含 Liquid 变量的媒体地址在实际渲染后、进入发送队列前归档，包括图片、多分辨率图片、海报、CSS 与字体依赖。历史预览使用归档字节并阻止外部网络加载。相对地址、不支持的协议、活动文档或流媒体播放列表会在进入发送队列前明确失败。客户删除会清除其独有媒体，同时保留仍被其他客户或公共素材引用的内容。

发送节点及邮件队列使用该次执行保存的服务商配置。修改工作空间服务商后，新执行读取新配置；已经创建的执行保留原配置和回执验证凭据。

客户手机号等端点使用 Customer UUID，邮件不是必填项。营销许可需要显式设置，当前撤销状态在发送时仍然生效。

## 升级与验证

本功能使用 V57 迁移，同时支持新建工作空间。迁移按依赖建立定义、规范事件、对象、状态历史、执行快照、运行队列及客户渠道结构。升级测试将 V56 数据库运行 V57 两次，并与新建数据库的列、约束、触发器和函数进行比较；保留旧客户及外部身份数据。

真实 HTTP 浏览器验收使用独立 PostgreSQL，不使用共享开发数据库。运行 `TestConfigurableMarketingHTTPIntegration` 时设置 `CONFIGURABLE_BROWSER_FIXTURE` 为输出 JSON 路径，测试将临时应用代理开放在 18580，最长保持 15 分钟，也可 `POST /__fixture/stop` 提前结束。该钩子只存在于测试代码。

前端在隔离环境使用 `CONFIGURABLE_API_URL` 启动 `vite --config vite.configurable.ts`，再设置同一个 `CONFIGURABLE_BROWSER_FIXTURE` 运行 `playwright test --config playwright.configurable.config.ts`。如系统浏览器不在 Playwright 默认位置，设置 `PLAYWRIGHT_EXECUTABLE_PATH`。测试通过真实接口创建定义、验证字段、重复上报、拖拽并保存漏斗、重新加载和计算结果；未替换业务 API。
