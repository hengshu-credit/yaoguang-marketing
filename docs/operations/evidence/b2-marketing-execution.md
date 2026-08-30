# B2 营销执行验收证据

**日期：** 2026-08-30

**分支：** `main`

**范围：** Audience、Campaign snapshot、发送前预检、三层频控、可靠大名单导入

## 结论

B2 的正确性和容量门禁已通过。活动使用版本化 Audience 和不可变 Customer snapshot；发送前仍实时检查身份、同意、抑制和频控。批量导入完整落盘后再异步处理，计数守恒且支持租约恢复。Automation 的 `once/every_time` 入场语义未被消息频控替代。

## 真实容量结果

| 门禁 | 规模与结果 | 实测 |
| --- | --- | ---: |
| Audience + Campaign snapshot | 1,000,000 Customer；Audience membership 零重复、checkpoint 完整；Campaign snapshot 1,000,000、零重复、状态 `dispatching` | 总计 219.84 s |
| Campaign snapshot 子阶段 | 5,000/页，批量 `unnest` 写入，确定性 A/B variant | 55.33 s |
| Import staging/recovery | 1,000,000 行；2,000/批；租约重领不重复计数；取消后 1,000,000 行全部明确失败 | staging 35.82 s；总计 50.69 s |
| 三层频控并发 | 同一 Customer，campaign=17、trigger=11、workspace_global=7，100 并发 | 恰好允许 7；重放不加计数 |

容量测试使用当前本地 Docker PostgreSQL/Redis 环境。它证明同一实现路径可承载目标数量级；不同生产硬件仍需用相同命令建立自己的基线。

## 执行命令

```powershell
docker run --rm `
  -e INTEGRATION_TESTS=true `
  -e TEST_DB_HOST=host.docker.internal `
  -e TEST_DB_PORT=5433 `
  -e YAOGUANG_SCALE_CUSTOMERS=1000000 `
  -v "E:\workspace\notifuse:/workspace" `
  -w /workspace golang:1.25.4-bookworm `
  sh -lc "/usr/local/go/bin/go test ./tests/integration -run TestAudienceSnapshotScaleIntegration -count=1 -timeout 30m -v"

docker run --rm `
  -e INTEGRATION_TESTS=true `
  -e TEST_DB_HOST=host.docker.internal `
  -e TEST_DB_PORT=5433 `
  -e YAOGUANG_IMPORT_ROWS=1000000 `
  -v "E:\workspace\notifuse:/workspace" `
  -w /workspace golang:1.25.4-bookworm `
  sh -lc "/usr/local/go/bin/go test ./tests/integration -run TestImportJobRecoveryIntegration -count=1 -timeout 30m -v"

docker run --rm `
  -e INTEGRATION_TESTS=true `
  -e TEST_REDIS_ADDR=host.docker.internal:16380 `
  -e TEST_REDIS_PASSWORD=notifuse-redis `
  -v "E:\workspace\notifuse:/workspace" `
  -w /workspace golang:1.25.4-bookworm `
  sh -lc "/usr/local/go/bin/go test ./tests/integration -run TestFrequencyPolicyConcurrencyIntegration -count=1 -v"
```

实际验证命令还挂载了 Go module/build cache；这不改变被测代码和数据库路径。

## 关键不变量

- Audience build 按 Customer UUID keyset 分页，membership 与 checkpoint 同事务提交，完成后原子切换 active build。
- Campaign snapshot 在 `dispatching` 前完成，发送任务按 snapshot ordinal 读取；Audience 后续变化不影响已启动 run。
- 快照中缺少 Email 投影的 Customer 不再被 SQL 内连接丢弃，而是形成 `missing_identity` suppressed Delivery Intent。
- 快照冻结的 A/B variant 直接传入发送链路；重试使用相同 ordinal/effect key。
- `suppressed` 和 `deferred` 是合法的初始 Delivery 决策，真实 PostgreSQL 仓储不会再错误拒绝。
- 三层适用频控在一个 Redis 原子预留中完成；任何一层 deny 即不写其他窗口，基础设施故障返回 defer。
- Import Job 永远满足 `total = pending + processing + succeeded + failed`；`(job_id, ordinal)` 和 Customer 幂等键共同支持恢复。

## 回归门禁

- Go：Audience/Campaign/Broadcast/Preflight/Frequency/Import 单元与集成测试。
- Console：Audience/Import、Campaign Audience selector、Frequency Policy 表单测试，随后执行 ESLint 和生产构建。
- API：Redocly lint 通过；B2 新增路径不产生 `operation-4xx-response` 告警。
- 用户工作区中的 `internal/migrations/v46.go` 与 `v46_test.go` 不属于本次改动，不纳入提交。
