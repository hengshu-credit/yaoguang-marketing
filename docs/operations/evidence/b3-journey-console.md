# B3 自动化旅程与控制台验收证据

验收日期：2026-08-30（Asia/Shanghai）

## 已验证结论

- `once` 在真实 PostgreSQL 中对同一 Automation + Customer 只创建一个 Journey instance。
- `every_time` 对相同 `origin_event_id` 防重，对新的事件创建新实例。
- Entry guard 与消息频控分离：进入保护产生 `guard_deferred`；Campaign、Trigger、Workspace global 三层消息频控原子评估。
- 消息被 Trigger 频控抑制后，Journey instance 仍为 active；旅程轨迹可同时看到 enrollment、节点事件和 `suppressed` Delivery Intent。
- 激活必须携带刚生成的 preflight hash；有 warning 时必须显式确认。
- Customer 360 可以进入 Journey trace，并跳转到具体 Automation node 修复。
- 375×812 与 768×1024 均保留九个一级入口；关键页面没有 document 级横向滚动；手机端主要按钮触控区域不小于 44px。

## 可重复命令

真实数据库语义测试：

```powershell
docker run --rm `
  -e INTEGRATION_TESTS=true -e TEST_DB_HOST=host.docker.internal -e TEST_DB_PORT=5433 `
  -v "${PWD}:/workspace" -v yaoguang-go-mod:/go/pkg/mod `
  -v yaoguang-go-build:/root/.cache/go-build -w /workspace `
  golang:1.25.4-bookworm sh -lc `
  "/usr/local/go/bin/go test ./tests/integration -run 'TestJourney(FrequencySemantics|TraceIncludesFrequencySuppressedDelivery)Integration' -count=1 -v"
```

结果：退出码 `0`。`JourneyFrequencySemantics` 三个子场景通过；`JourneyTraceIncludesFrequencySuppressedDelivery` 通过。

控制台 E2E：

```powershell
Set-Location console
$env:PLAYWRIGHT_CHANNEL='chrome'
npm run test:e2e -- --project=chromium e2e/marketing-journey.spec.ts e2e/marketing-mobile.spec.ts
Remove-Item Env:PLAYWRIGHT_CHANNEL
Set-Location ..
```

结果：退出码 `0`，`4 passed (21.6s)`。`PLAYWRIGHT_CHANNEL` 只用于复用本机 Chrome；CI 不设置时仍使用 Playwright 锁定的 Chromium。

## 语义与数据边界

| 语义 | 权威防重键 | 预期决策 |
| --- | --- | --- |
| 每客户一次 | `(automation_id, customer_id) WHERE frequency='once'` | 首次 `enrolled`，后续 `already_once` |
| 每个事件一次 | `(automation_id, customer_id, origin_event_id) WHERE frequency='every_time'` | 新事件 `enrolled`，重放 `replayed_event` |
| 进入保护 | `journey_entry_decisions` + active instance/cooldown 查询 | `guard_deferred` 或 `guard_denied` |
| 消息频控 | Redis 原子窗口 + `frequency_decisions.reservation_id` | allow/defer/deny；不更改 enrollment |
| 投递防重 | `delivery_intents.effect_key` | 每个业务副作用只有一个 intent |

## UI 与移动端证据

- 激活预检：`docs/operations/evidence/b3-journey-preflight.png`
- Customer 360 + Journey trace：`docs/operations/evidence/b3-customer-journey-trace.png`
- 375×812 投递中心：`docs/operations/evidence/b3-mobile-375.png`
- E2E 对 375px 和 768px 视口逐页断言 `document.documentElement.scrollWidth === clientWidth`。

截图使用掩码身份和构造数据，不包含真实客户完整身份值。

## 性能证据状态

本轮功能验收没有把开发机测试耗时冒充 8c/16GB 参考环境的容量结论。仓库已提供 `scripts/realtime-load-test.ps1`，但以下三项必须在独立 PostgreSQL/Redis/RabbitMQ、固定硬件和可观测链路下执行后才能签署容量门禁：

- 持续 500 events/s，Event accept p95 < 200ms；
- Event 到 Delivery Intent p95 < 5s；
- Customer 单条写入 p95 < 200ms。

正式压测必须记录硬件、持续时间、成功/失败数、p50/p95/p99、队列最老等待时间、数据库连接等待、Redis 延迟和应用 RSS。未达到目标时保留 profile 与瓶颈，不得放宽完整性、幂等或审计要求，也不得仅修改阈值宣称通过。
