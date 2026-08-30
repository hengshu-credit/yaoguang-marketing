# B1 统一投递一致性验收证据

验收日期：2026-08-30（Asia/Shanghai）

## 已验证结论

- 相同 effect key 重放只保留一个 intent 和一个 queue row。
- 4 个并发 worker 使用 `FOR UPDATE OF queue SKIP LOCKED` 领取 40 条任务，无重复领取。
- provider 已接受后发生崩溃，任务不会被盲目重新领取。
- 连接在请求写出后中断会进入 `unknown`、生成 reconciliation，且不会自动重发。
- PostgreSQL 部分唯一索引冲突目标和 outer join 锁范围已由真实数据库验证。
- Go race 检测覆盖 repository、queue worker 和 broadcast orchestration，退出码为 0。

## 可重复命令与结果

```powershell
docker run --rm -v "${PWD}:/src" -w /src `
  -v yaoguang-go-mod:/go/pkg/mod -v yaoguang-go-cache:/root/.cache/go-build `
  golang:1.25.4 go test -race ./internal/repository ./internal/service/queue ./internal/service/broadcast `
  -run "Delivery|EmailQueue|Broadcast" -count=1
```

结果：退出码 `0`；三个 package 均为 `ok`。

```powershell
docker run --rm --network container:tests-mailpit-1 -v "${PWD}:/src" -w /src `
  -v yaoguang-go-mod:/go/pkg/mod -v yaoguang-go-cache:/root/.cache/go-build `
  -e INTEGRATION_TESTS=true -e TEST_DB_HOST=host.docker.internal -e TEST_DB_PORT=5433 `
  -e TEST_DB_USER=notifuse_test -e TEST_DB_PASSWORD=test_password `
  golang:1.25.4 go test -tags integration -timeout 10m ./tests/integration `
  -run "Delivery(CrashRecovery|ProviderUnknown|Concurrency)Integration" -count=1
```

结果：退出码 `0`，耗时 `16.393s`。

## Unknown 样例

故障：provider 请求已经写出，连接在响应前 reset。

期望并已验证：attempt 和 intent 进入 `unknown`；创建一条 pending reconciliation；queue 不再被 `ClaimPending` 返回。只有 `mark_confirmed`、`mark_terminal_failed`、`retry_after_verified_not_accepted` 三种带操作人和原因的处置被接受。

## 防重查询

```sql
SELECT effect_key, COUNT(*)
FROM delivery_intents
GROUP BY effect_key
HAVING COUNT(*) > 1;
```

集成用例同时断言同一 effect key 的 intent 数和关联 queue 数都为 `1`。查询预期结果为 0 行；生产发布后应按 Workspace 再执行一次只读核验。

## 性能证据状态

本轮已证明并发领取正确性，但尚未把“100,000 intents、8 workers、fake provider 5ms、调度开销小于总处理时间 10%”作为已通过结论。该项必须在接近生产的独立 PostgreSQL/Redis 环境完成，记录硬件、数据规模、总吞吐、claim-to-submit p95 和调度占比后才能签署；不得用单元测试耗时替代。

