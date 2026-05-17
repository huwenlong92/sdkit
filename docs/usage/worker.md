# Worker 使用说明

Worker 是 Queue Runtime Host，负责注册业务事件、消费 Redis/Asynq 队列任务，并把失败尝试记录到数据库。

## 启动

```bash
go run ./cmd/sdkitgo serve worker -c configs/config.yaml
```

启动链路：

1. `bootstrap.Init` 读取 `configs/tracing.yaml` 并自动初始化 OpenTelemetry。
2. `worker/config.Load` 读取 `worker.queue`、filesystem、eventbus、websocket 发布配置。
3. `worker.Server.Start` 初始化 `queue.RuntimeKernel`，注册 event registry，并启动消费者。
4. `worker.Server.Shutdown` 关闭消费者、queue kernel、flush 失败日志，并关闭 tracing。

## 失败日志入库

worker 队列任务返回 error 时会写两类日志：

- 标准日志：`Worker队列任务失败`，带 `task_id`、`queue`、`type`、`retry_count`、`max_retry`、`rate_limited`、`err`，并从 `ctx` 自动补充 `trace_id/span_id`。
- 数据库日志：写入 `system_queue_failure_log`，保留任务 payload、错误、重试次数，以及 `track_id/request_id/trace_id/span_id`。

失败表用于排查和后台重投。查询示例：

```sql
SELECT id, task_id, queue, type, retry_count, max_retry, rate_limited, trace_id, error
FROM sd_system_queue_failure_log
WHERE task_id = 'your-task-id'
ORDER BY id DESC;
```

## 重试策略

`worker/bootstrap/retry.go` 负责按任务类型定制重试间隔：

- `user:sync` + `source=retry_fast`：200ms，供真实集成测试快速覆盖多次重试。
- 普通 `user:sync`：按重试次数分钟级退避。
- `queue.RateLimited(...)`：使用错误里的 `RetryIn`，并标记 `rate_limited=true`。

业务事件只需要返回 error，不直接依赖 Asynq 的错误类型。

## Event Registry

Worker 业务逻辑放在 `worker/event`，事件协议和默认投递参数放在 `worker/taskdef`。注册入口统一在 `worker/registry.go`，由 `queue.Registry` 记录事件，`queue.Dispatcher` 负责 handler lookup、middleware pipeline 和 typed payload 适配。

```go
import workermiddleware "sdkitgo/worker/middleware"

func RegisterEvents(r *queue.Registry) error {
    r.Use(workermiddleware.Tracing())
    return r.RegisterAll(
        queue.Register(taskdef.TypeUserSync,
            workermiddleware.RateLimit(10, time.Minute),
            event.HandleUserSync,
        ),
    )
}
```

事件函数使用 typed payload：

```go
func HandleUserSync(ctx context.Context, payload *taskdef.UserSyncPayload) error {
    return service.SyncUser(ctx, payload.UserID)
}
```

`core/queue` 支持普通 `func(next HandlerFunc) HandlerFunc`，也支持 `c.Next()` 风格。`c.Next()` 返回后续 middleware 或 event handler 的 error，middleware 可以记录、包装或直接返回：

```go
func Trace() queue.Middleware {
    return queue.ContextChain(func(c *queue.HandlerContext) error {
        err := c.Next()
        if err != nil {
            // 这里能感知下游 event handler/middleware 的错误。
            return err
        }
        return nil
    })
}
```

`core/queue` 只提供 registry、middleware 机制和限流扩展点，具体按哪个任务、哪个业务 key、哪个渠道限流，由 worker 自己定义。

## Outbox

需要业务事务和队列投递一致时，使用 `pkg/queue/outbox/gorm`：

```go
import gormoutbox "github.com/huwenlong92/sdkit/pkg/queue/outbox/gorm"

runtime := queue.Runtime(ctx)
if runtime == nil {
    return queue.ErrNotInitialized
}

outbox := gormoutbox.NewGormOutbox(tx, runtime.Client())
err := outbox.Save(ctx,
    queue.NewTask(taskdef.TypeUserSync, payload),
    queue.Queue("critical"),
    queue.TaskID("biz-unique-id"),
)
```

事务提交后由后台任务调用：

```go
runtime := workerbootstrap.Runtime()
outbox := gormoutbox.NewGormOutbox(database.DB, runtime.Client())
err := outbox.Flush(ctx, 100)
```

真实 demo 会验证：Outbox 记录写入 DB、flush 后进入 Redis 队列、worker 消费失败后写入 `system_queue_failure_log`，并保留 trace/tracking 字段。

Worker 可通过配置启动常驻 poller：

```yaml
worker:
  queue:
    outbox:
      enabled: true
      batch_size: 100
      flush_interval: 5s
```

也可以用独立命令：

```bash
go run ./cmd/sdkitgo queue outbox flush --limit=100
go run ./cmd/sdkitgo queue outbox poll --limit=100 --interval=5s
```

## 唯一任务和固定 TaskID

短时间防重复投递使用 `queue.Unique(ttl)`：

```go
_, err := queue.Enqueue(ctx,
    taskdef.NewUserSyncTask(1001, "admin", false),
    queue.Queue(queue.DefaultQueueName),
    queue.Unique(5*time.Minute),
)
```

有明确业务唯一 ID 时使用固定 `queue.TaskID(id)`。如果任务失败，已有任务仍可能停留在 retry 或 archived 状态，再用同一个 TaskID 入队会冲突。后台重投必须先删队列内已有任务：

```go
row := loadFailureLog(ctx, taskID)

runtime := queue.Runtime(ctx)
if runtime == nil {
    return queue.ErrNotInitialized
}

if err := runtime.Operations().DeleteTask(ctx, row.Queue, row.TaskID); err != nil {
    return err
}

_, err := queue.Enqueue(ctx,
    queue.NewTask(row.Type, row.Payload),
    queue.Queue(row.Queue),
    queue.TaskID(row.TaskID),
    queue.MaxRetry(3),
)
```

## 真实 Demo 测试

真实 DB/Redis 集成 demo 位于：

- `worker/tests/worker_queue_demo_test.go`

运行：

```bash
SDKITGO_INTEGRATION=1 go test ./worker/tests -run TestWorkerQueueDemoCoversRealStrategies -count=1 -v
```

如果要从 app HTTP 服务入口验证 Jaeger 全链路，运行：

```bash
SDKITGO_INTEGRATION=1 go test ./worker/tests -run TestWorkerQueueMechanismDemoFromAPIExportsFullTrace -count=1 -v
```

该测试通过 API router 投递 `queue:mechanism_demo` 任务，并在同一个 trace 中覆盖：

- `POST /api/queue/mechanism/demo` HTTP server span。
- `producer::queue:mechanism_demo` 投递 span。
- `consumer::queue:mechanism_demo` 消费 span。
- `handler::queue:mechanism_demo` event handler span。
- `trace_probe` 场景中的 PostgreSQL 和 Redis span。
- `retry_fast` 多次重试、`rate_limit` 直接限流。

测试日志会打印 `trace_id`，可以直接在 Jaeger 中按 trace ID 查询。

该测试覆盖：

- retry_fast 多次重试，并检查 `system_queue_failure_log` 中的 retry_count。
- rate_limit 错误入库，并检查 `rate_limited=true`。
- Redis RateLimiter middleware 先允许一次，再限制同 key 第二次任务。
- `queue.Unique` 重复投递冲突。
- Outbox 保存后由常驻 poller 自动 flush 到队列，并验证 worker 消费和链路字段。
- 固定 TaskID 失败后先 `DeleteTask`，再用同一个 TaskID 重投。
- 队列 producer/consumer span 自动生成，并验证失败日志中的 trace 字段。
