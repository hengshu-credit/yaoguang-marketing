# 可配置营销功能验收记录

日期：2026-09-07。代码基线 `738bc6dc`，开发分支 `codex/configurable-marketing`，数据库迁移版本 V57。

## 已交付行为

| 用户要求 | 实现及验收证据 |
| --- | --- |
| 同一客户可多次登录、借款 | 每次行为独立记录。只有工作空间、来源、上报 ID 相同且载荷相同的重试去重；改变载荷返回冲突。真实并发数据库测试、SDK 队列测试及浏览器两次登录闭环通过。 |
| Customer UUID 与外部身份 | 无邮箱客户可以上报、配置触达端点、进入任务与设置状态。外部用户 ID 合并保留历史，漏斗按归并后的客户统计。 |
| 申请、订单等由配置定义 | 通用对象类型、对象实例和关系，无固定申请/借款表。合同类型与关联的申请、订单、结清链路样本均通过。兄弟对象不能混合完成同一链路。 |
| 默认、自定义事件及可配字段 | 默认事件、任意事件编码、嵌套字段校验、外部 JSON 映射、客户属性识别/绑定/更新/记录、有效期、待关联诊断均可配置；原生渠道事件受保护。 |
| 状态独立、允许重叠 | 满足、不满足、未知；规则、人工覆盖、时间区间、状态进入/退出和关联子对象条件。状态覆盖、交集、并集单独分析。 |
| 任务选择跟踪项及触发 | 自选事件、状态、目标和对象；每次事件或每对象一次入场，同对象等待/退出；历史回补、过期和延迟消费保护。 |
| Campaign 不管理版本 | 当前配置可编辑，每次运行保存完整配置、人群筛选、素材、服务商配置及实际执行证据；两次运行可以分别还原。新执行使用最新配置，旧执行保留原配置。 |
| 素材和发送内容可追溯 | 静态及渲染后动态媒体在发送前归档；内部凭据加密；实际邮件发送和渠道回执使用原执行服务商；发送确认与证据一致，重试不重复发送。 |
| 拖拽漏斗 | 步骤拖入/排序/复制/删除、键盘排序、保存重开、OR/并行/可选步骤、观察窗口、次数与对象数、对象关系、事件时点状态、归因及金额口径。异步结果保留原定义和数据水位。 |
| 可用的配置界面与翻译 | 配置目录、状态、Customer 360、任务与自动化条件、执行详情、漏斗及状态覆盖已接入。501 条新增文案全部进入九种语言目录，英文和简体中文完整，其他语言提供可编辑英文回退。 |

## 最终验证结果

| 检查 | 结果 |
| --- | --- |
| Go `./internal/... ./pkg/... ./config/...`，`-count=1` | 通过。最终代码全量复验成功。 |
| 真实 PostgreSQL / ClickHouse / HTTP 集成 | 21 个顶层测试通过，无跳过。包括 V56→V57 连续升级两次与新建库结构对账。 |
| 前端相关组件回归 | 22 个文件，77 项测试通过。 |
| 前端生产构建 | Lingui compile、TypeScript、Vite build 通过。保留既有大包体积提示。 |
| 前端 ESLint | 通过。 |
| Web Analytics SDK | 25 个文件，471 项测试通过；类型检查与 ESLint 通过；版本 57.0 的构建产物已更新。 |
| Chromium 真实浏览器 | 事件配置、无效/有效样例、两次上报及重试、实际拖拽排序、保存重开、异步查询、中英文切换、状态交集与并集全部通过。 |
| OpenAPI | 132 个路径、462 个引用、47 个文件引用校验及 Redocly bundle 通过。 |
| Lingui 新词条审计 | 九种语言各 501 条新增词条均存在；非源语言的新词条无空翻译。 |
| Git 空白检查 | `git diff --check` 通过。 |

浏览器没有替换业务 API。两次登录计算结果为：第一步 **1 个客户、2 次事件**，后续登录步骤 **1 个客户、1 次匹配**；相同上报 ID 的重试未增加次数。两个独立状态同时满足时，交集与并集均为 1。

交叉代码审查发现并修复了：跨步骤覆盖已绑定订单、合并后的历史归属、缺少画像行的 UUID 客户不能设置状态、待关联历史事件错误消费、过期事件的首次动作、活动服务商配置漂移、证据失败后的发送重试、敏感嵌套字段及客户独有媒体清理。对应反例均有回归测试。

## 复现

环境使用独立测试容器及唯一临时数据库；未迁移原开发数据库，未发送真实服务商消息。

```powershell
docker exec notifuse-configurable-go /usr/local/go/bin/go test ./internal/... ./pkg/... ./config/... -count=1

docker exec -e INTEGRATION_TESTS=true -e TEST_DB_HOST=notifuse-configurable-postgres -e TEST_DB_PORT=5432 -e TEST_CLICKHOUSE_ADDR=notifuse-configurable-clickhouse:9000 -e SNAPSHOT_POSTGRES_ADMIN_DSN=postgres://notifuse_test:test_password@notifuse-configurable-postgres:5432/postgres?sslmode=disable notifuse-configurable-go /usr/local/go/bin/go test -tags integration ./internal/database ./internal/repository ./tests/integration -run 'TestConfigurableWorkspaceUpgradeMatchesFreshSchema|TestConfigured.*|TestExecutionSnapshotPostgresImmutableRunsAndJourney|TestTrackedEventPersistence.*|TestBusinessConfigPersistence.*|TestNativeBusinessEventCataloguePersistence|TestCustomerErasureRemovesOnlyUnreferencedRenderedAssets|TestBrowserEventPersistence.*|TestSubjectStateFunnelPostgresIntegration|TestFunnelPostgresClickHouseParity|TestConfigurableMarketingHTTPIntegration' -count=1 -v

docker exec -w /workspace/console notifuse-configurable-node npx vitest run src/components/business src/components/business_states src/components/funnels src/components/executions src/components/customers/CustomerBusinessActivity.test.tsx src/components/customers/CustomerEndpointsTab.test.tsx src/components/automations/config/TriggerConfigForm.test.tsx src/components/automations/JourneyCreateWizard.test.tsx src/components/automations/NodeConfigPanel.test.tsx src/components/automations/AutomationFlowViewer.test.tsx src/components/automations/nodes/StatNode.test.tsx
docker exec -w /workspace/console notifuse-configurable-node npm run build
docker exec -w /workspace/console notifuse-configurable-node npm run lint

docker exec -w /workspace/web_analytics_sdk notifuse-configurable-node npm test
docker exec -w /workspace/web_analytics_sdk notifuse-configurable-node npm run type-check
docker exec -w /workspace/web_analytics_sdk notifuse-configurable-node npm run lint
```

浏览器代理、凭据夹具及 Playwright 的启动步骤见[接入说明](../../guides/configurable-marketing.md#升级与验证)。测试凭据文件在临时 HTTP 服务结束时删除，不进入代码提交。

## 当前边界

- 漏斗默认从 PostgreSQL 读取；ClickHouse 已验证当前秒级样本与重复投影一致性，未将其切为默认生产分析读源。
- 每次查询最多处理 100,000 条事件/候选和五百万次匹配操作，异步计算最多两分钟。超界明确失败，不返回截断后的“完整结果”。
- 事件接入提供明确校验的 JSON Schema 与字段路径子集；不支持的关键字和路径会拒绝。原始事件的通用撤回/替换协议不在本次实现中。
- 配置回执事件不会生成渠道未提供的信号。信贷模板是可编辑配置，需要发布或替换其中的业务事件及对象定义。
- 历史上从未捕获的执行内容明确显示缺失。媒体支持范围及发送前拒绝规则见[接入说明](../../guides/configurable-marketing.md#每次实际运行的历史)。
- 本轮代码保留在开发分支，未合并或部署到共享环境。
