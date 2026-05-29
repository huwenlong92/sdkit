# Worker 模块方案

## 目标

Worker 提供独立的队列消费服务，默认使用 `core/queue` 的 Asynq driver。业务侧只定义任务协议和 handler，不直接操作 Asynq 类型。

核心目标：

- 独立消费 `worker.queue` 配置的队列。
- 统一注册 `worker/event` 中的业务事件函数。
- 统一处理任务执行记录、重试间隔和限流错误。
- 自动透传 tracing、tracking、request id。
- 支持真实 DB/Redis 集成测试作为 demo。

## 目录边界

```txt
worker/
  config/      Worker 配置加载
  event/       业务事件处理
  taskdef/     任务类型、payload 和投递 helper
  infra/       Worker 私有能力接线
  tests/       跨模块真实集成测试
```

`worker/bootstrap` 负责 Queue Runtime Host 的宿主侧接线：

- `runtime.go`：调用 `core/queue.RuntimeKernel`，注入 Redis、DB 和 Gorm Outbox。
- `retry.go`：业务重试策略；通用 deadline/rate limit 退避回落到 `queue.DefaultRetryDelay`。

## 任务错误模型

worker 不再维护独立的 `system_queue_failure_log`。任务错误统一写入队列任务表：

- `system_queue_task`：任务索引、payload、当前状态、最近错误和链路字段。
- `system_queue_task_run`：每次执行尝试、attempt、耗时、错误和链路字段。
- `system_queue_task_run_log`：执行过程日志。

写入链路：

1. Asynq 任务执行失败。
2. runtime logging middleware 输出标准错误日志，不打印 payload。
3. `TaskStoreMiddleware` 结束执行记录并写入错误、attempt 和链路字段。

## Tracing

`bootstrap.Init` 会读取 `configs/tracing.yaml` 并自动初始化 OpenTelemetry。worker 不需要单独调用 tracing 初始化。

队列链路由 `core/queue` 自动创建：

- `queue enqueue <task_type>`：投递 span。
- `worker <task_type>`：消费 span。

Asynq task headers 负责透传 `traceparent`、`tracestate`、`baggage`、`X-Track-ID`、`X-Request-ID`。worker 执行记录会从 headers 提取 `track_id/request_id/trace_id/span_id`。

## 策略场景

当前真实 demo 覆盖的策略：

| 场景 | 验证点 |
|------|--------|
| 普通失败重试 | 多次失败会按 attempt 写入 `system_queue_task_run` |
| 快速重试 | `source=retry_fast` 使用 200ms 间隔，便于测试 |
| 限流错误 | `queue.RateLimited` 写入执行错误，并由后台失败列表识别 |
| Redis 限流器 | 任务注册时可通过 `workermiddleware.RateLimit(limit, window)` 接入 `queue.RateLimiter` |
| App 服务投递链路 | API/Admin 投递 `queue:mechanism_demo`，Jaeger 可看到 HTTP -> producer -> consumer -> event handler |
| 唯一任务 | `queue.Unique` 在 TTL 内重复投递返回 `ErrTaskDuplicated` |
| Outbox | 任务先写业务侧 outbox 表，再 flush 到真实队列 |
| 固定 TaskID | 失败后必须先 `DeleteTask`，再用同一 TaskID 重投 |
| tracing | enqueue/worker span 自动生成，执行记录保留 trace 字段 |

## 复盘记录

已实现：

- worker 真实 DB/Redis demo 覆盖普通重试、快速重试、显式 `queue.RateLimited`、Redis `RateLimiter`、唯一任务、固定 TaskID 删除后重投、Outbox flush、执行记录入库和 tracing。
- 任务执行记录入库记录 `track_id/request_id/trace_id/span_id`。
- Outbox 保存 context headers，flush 后 worker 仍能恢复链路。
- Worker 可通过 `worker.queue.outbox.enabled=true` 启动 Outbox 常驻 poller。
- `queue outbox flush` / `queue outbox poll` 独立 command 已实现，代码归属为 `worker/command/queue`。
- 队列限流已提供 `core/queue/runtime/middleware.RateLimit` 和 `queue.ContextChain`，worker demo 在 `worker/middleware` 定义任务级限流规则，`worker/registry.go` 只负责挂载。
- admin 已挂载 queue 管理 API，使用 query/body，不使用 URL path 参数和 DELETE 方法。
- `worker/command/queue` 已具备 queue status/queues/tasks/task/enqueue/retry/archive/cancel/delete/pause/resume；根 `command` 包只聚合注册入口。
- `core/queue` 已把唯一任务重复和 TaskID 冲突统一映射为 `queue.ErrTaskDuplicated`，调用方可 `errors.Is` 感知。
- Queue Runtime Kernel ownership 已收敛到 `core/queue.RuntimeKernel`，worker bootstrap 不再持有 outbox poller cancel 和 registry middleware pipeline。

暂未实现：

- Outbox 管理后台页面/API，例如查看 pending/failed、手动重放、清理归档。
- 队列治理能力的指标面板，例如 limiter 命中次数、outbox pending 数、flush 成功率。
- Outbox 失败记录清理/归档、按业务类型分片 flush。
- Outbox 表分区或冷热归档策略，高吞吐场景还需要按业务量评估。
- notification fanout 聚合任务模板，目前多渠道通知仍由业务自己决定写多条 outbox 还是写一个聚合任务。

## 更新记录

- 2026-05-22：移除旧 `system_queue_failure_log` 链路，任务错误统一来源改为 `system_queue_task_run` / `system_queue_task_run_log`。
- 2026-05-16：worker 任务级 tracing/rate-limit 接线收敛到 `worker/middleware`，`worker/registry.go` 只保留事件注册。
- 2026-05-15：Queue Runtime Kernel ownership 收敛，worker bootstrap 仅注入宿主依赖，outbox poller、dispatcher middleware pipeline 由 `core/queue` 持有。
- 2026-05-12：补充 Outbox 性能取舍、适用场景和剩余缺口；DB 表模型和 store 实现由业务侧提供。
- 2026-05-12：实现 Outbox 常驻 poller、独立 command 和 queue rate limit middleware，真实 demo 改为 poller 自动 flush。
- 2026-05-12：真实 demo 增加 Redis 限流器和 DB Outbox flush 覆盖，并记录剩余未实现项。
- 2026-05-12：新增 worker 真实集成 demo，覆盖重试、限流、唯一任务、固定 TaskID 删除后重投、执行记录入库和 tracing。
- 2026-05-12：任务执行记录补充 `track_id/request_id/trace_id/span_id`，便于从数据库反查链路。
- 2026-05-15：worker 注册入口收敛为 `worker.RegisterEvents`，业务目录从 `worker/handler` 调整为 `worker/event`，typed payload 解码由 `core/queue` registry 适配层负责。
- 2026-05-13：任务注册入口统一为 `worker.RegisterRouter`，handler 包只保留任务处理函数。
