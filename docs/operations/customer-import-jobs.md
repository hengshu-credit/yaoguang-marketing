# 客户名单批量导入运维手册

## 能力边界

批量导入使用 Workspace 独立的持久化 Import Job。HTTP 上传返回前，系统先把每个 CSV 数据行写入 `import_job_rows`，再创建后台任务；浏览器关闭或 Worker 重启不会让已接收行消失。

默认限制：

| 配置 | 默认值 | 说明 |
| --- | ---: | --- |
| `CUSTOMER_IMPORT_MAX_ROWS` | 1,000,000 | 单个文件最大数据行数，不含表头 |
| `CUSTOMER_IMPORT_PROCESS_CHUNK_SIZE` | 2,000 | 每次 Worker claim 与 Customer batch 大小 |
| `CUSTOMER_IMPORT_MAX_FILE_BYTES` | 1 GiB | 单个上传流最大字节数 |

三个值都可在后台环境配置修改。同步 Customer batch API 仍有自己的上限，不应代替大文件 Import Job。

## CSV 约定

- 第一行必须是非空且不重复使用的字段名。
- 当前直接识别 `external_user_id`、`email`、`phone`；每行至少要形成一个有效 Customer 标识。
- 格式不合法的物理行也会占用 ordinal，并以 `csv_parse_error` 或 `mapping_error` 进入明确失败，不会静默跳过。
- 失败明细只输出外部用户 ID 和脱敏联系方式，不输出完整敏感身份。

## 状态与守恒

Job 状态：`uploading → staged → processing → completed`，也可能进入 `rejected` 或 `cancelled`。

任意时刻必须满足：

```text
total = pending + processing + succeeded + failed
```

上传阶段按 2,000 行批量使用参数化 `unnest` 写入；`(job_id, ordinal)` 唯一键使重复提交同一分片不会新增行。Worker 使用 `FOR UPDATE SKIP LOCKED` claim，保存 claim token 和 lease。租约过期后可重新领取；Customer 幂等键为：

```text
import:<job_id>:<ordinal>:<row_checksum>
```

## 运营操作

1. 在“客群 → 批量导入”上传 CSV。
2. 收到“名单已完整接收”后可以关闭页面；后台任务继续执行。
3. 任务卡片分别显示总数、待处理、处理中、成功、明确失败，进度只使用终态行计算。
4. 失败数大于零时下载 UTF-8 CSV 明细，修正后作为新任务重新上传。
5. 取消任务会保留已成功 Customer，并在同一数据库事务中把全部待处理/处理中行转为 `cancelled_by_user` 明确失败。

## 故障恢复

- Worker 退出：等待 lease 到期，调度器会从最小未完成 ordinal 继续。
- 同一任务重复执行：行 claim 和 Customer 幂等键共同防止重复业务写入。
- Redis 或消息系统不可用：任务保留在持久化调度表，不能手工删除 `import_job_rows`。
- 上传请求中断：检查 `uploading` Job。它只代表服务端实际持久化的行；确认客户端重传时应创建新 Job，不要复用未知的半上传文件。
- 任务计数异常：先执行下面的只读对账，不直接改计数。

```sql
SELECT status, COUNT(*)
FROM import_job_rows
WHERE job_id = '<job uuid>'
GROUP BY status;
```

将结果和 `import_jobs` 的五个计数字段比较；确认差异原因后再走修复流程。

## 监控建议

- `uploading` 超过上传超时：客户端中断或网关断流。
- `processing_count > 0` 且 `updated_at` 长时间不变：Worker 不可用或 lease 重领失败。
- `pending_count` 长时间不下降：任务调度/Customer batch 故障。
- `failed / total` 激增：字段名、Identity 格式或上游数据质量变化。
- PostgreSQL 临时文件和 WAL 激增：降低 chunk size；不要降低最大行数来掩盖容量问题。

## 已验证容量

2026-08-30 本地容器参考环境使用生产同路径完成 1,000,000 行 staging，耗时 35.82 秒；随后验证 lease 重领、计数守恒、取消转明确失败和错误分页，总测试耗时 50.69 秒。该数字用于回归比较，不是不同硬件上的 SLA。
