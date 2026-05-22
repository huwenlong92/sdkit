# Queue 模块方案

## 目标

`core/queue` 统一封装异步任务基础设施，业务侧只依赖项目内抽象，不直接创建或传递 driver 对象。

目标能力：

- 统一任务投递入口
- 统一 worker 注册和运行边界
- 支持多队列和队列权重
- 支持重试、延迟执行、固定任务 ID、唯一任务和任务保留
- 支持统一 `Capability`、`TaskState`、`QueueState`
- 支持统一 `Manager` 查询和管理任务
- 支持 Admin API 与 CLI command 通过 `OperationsRuntime` 复用同一套队列操作能力
- 支持 Redis/Memory 业务锁、幂等、限流接口
- 支持 DB Outbox，业务事务内先落库，再异步 flush 到队列
- 支持限流错误的专用重试语义
- 支持任务执行记录、执行日志和后台重投
- 统一接入 `core/logger`
- 任务记录和执行记录保留 track/request/trace/span 字段，便于从数据库回溯链路
- 通过 runtime orchestrator 统一 handler execution、task state、runtime event、lifecycle hook 和 observability observer

当前默认 driver 是 Asynq + Redis，同时提供 NATS JetStream driver 用于持久化投递和消费。运行时 driver 位于 `pkg/queue/asynq`、`pkg/queue/nats`，业务代码和 `core/queue` 公共接口禁止暴露具体 driver SDK 类型。

## 模块边界

`core/queue` 负责：

- 读取队列配置并创建投递端、管理端或完整队列运行实例
- 提供 `core/queue/facade/producer` 和 `core/queue/facade/operations` 作为 runtime capability 接入点，其中 producer 只表示投递端，operations 表示投递端 + 管理端，不启动 worker
- 持有 Queue Runtime Kernel 的初始化、重试判断、限流状态和 Outbox poller 生命周期
- 通过 `Driver` / `RunnerDriver` 注册表按 `Config.Driver` 选择实现
- 暴露 provider-agnostic 的 `Client`、`Worker`、`Manager` 接口
- 将 `queue.Task` 编码为 provider 任务
- 将 provider 的 worker 消息转换为 `queue.Message`
- 通过 `Registry` 和 `Dispatcher` 管理事件注册、handler lookup、payload bind 和 middleware 执行链
- 提供投递选项、重试间隔和失败统计判断
- 提供 `DeleteTask` 用于删除队列内已有任务后重投
- 提供 driver-neutral 的 `TaskStore` / `TaskSubmissionStore` 生命周期，统一记录 `submitting`、`pending/scheduled`、`submit_failed`、执行记录和执行日志
- 提供 driver-neutral 的 `TaskScheduleStore` 和 schedule poller，使初始延迟任务不依赖具体 driver 的 delay 能力
- 通过 `Orchestrator` 编排 task lifecycle、stage middleware、runtime event 和 observer

`core/queue` 不负责：

- 注册具体业务 handler
- 在 producer capability 中启动 worker/consumer
- 定义业务任务类型和 payload
- 直接绑定业务表结构
- 运行 cron/scheduler
- 重写 GORM 或业务服务能力

定时任务放在 `crontab` 或 scheduler 模块中，由定时入口触发 `queue.Enqueue(...)`。不要在多个服务实例里重复运行同一套 cron。

## 当前目录

```txt
core/queue/
  capability.go
  client.go
  config.go
  dispatcher.go
  driver_registry.go
  error.go
  task_store.go
  locker.go
  option.go
  operations.go
  queue.go
  registry.go
  retry.go
  runtime_kernel.go
  runtime_instance.go
  runtime_metadata.go
  runtime_status.go
  runtime.go
  state.go
  task.go
  facade/
    client.go
    config.go
    default.go
    use.go
  runtime/
    dispatcher/
    event/
    lifecycle/
    middleware/
    observability/
    orchestrator/
    state/
  retry/
```

标准类型、接口、错误、配置、状态、Option 和 RuntimeOption 的真实定义位于 `core/queue` 根包。业务代码统一从 `core/queue` 导入，不使用内部路径或具体 driver 路径。

业务示例目录：

```txt
worker/
  server.go
  taskdef/
  event/
  registry.go
  infra/
  bootstrap/
    runtime.go
    retry.go
```

## 核心接口

```go
type Client interface {
    Enqueue(ctx context.Context, task Task, opts ...Option) (*TaskInfo, error)
    BatchEnqueue(ctx context.Context, tasks []Task, opts ...Option) ([]*TaskInfo, error)
    Close() error
}

type Worker interface {
    Handle(pattern string, handler HandlerFunc)
    Use(middlewares ...Middleware)
    Run(ctx context.Context) error
    Shutdown(ctx context.Context) error
}

type Manager interface {
    Supports(cap Capability) bool
    Capabilities() map[Capability]bool
    ListQueues(ctx context.Context) ([]*QueueInfo, error)
    GetQueue(ctx context.Context, queue string) (*QueueInfo, error)
    ListTasks(ctx context.Context, query TaskQuery) ([]*TaskInfo, error)
    GetTask(ctx context.Context, queue, taskID string) (*TaskInfo, error)
    DeleteTask(ctx context.Context, queue string, taskID string) error
    RetryTask(ctx context.Context, queue string, taskID string) error
    ArchiveTask(ctx context.Context, queue string, taskID string) error
    CancelTask(ctx context.Context, queue string, taskID string) error
    PauseQueue(ctx context.Context, queue string) error
    ResumeQueue(ctx context.Context, queue string) error
}

type QueueRunner interface {
    Client
    Worker
    Manager
}
```

业务投递入口通过当前 context 中注入的 queue runtime 执行：

```go
queue.Enqueue(ctx, task, opts...)
queue.Push(ctx, taskType, payload, opts...)
queue.Delay(ctx, taskType, payload, delay, opts...)
```

`ctx` 必须来自已注入 queue runtime 的入口，例如 HTTP request context、worker runtime context，或显式 `queue.ContextWithRuntime(...)`。如果 context 中没有 queue runtime，投递会返回 `queue.ErrNotInitialized`。

`core/queue` 不再提供全局默认实例和包级管理函数。业务代码投递任务优先使用 `queue.Enqueue(ctx, task, opts...)`，队列管理操作使用 `queue.Runtime(ctx).Operations()` 或显式注入 `OperationsRuntime`。`queue.From(app)` 只用于 runtime wiring、provider startup 和 bootstrap lifecycle。

投递端只需要 `queue.NewClient(cfg)`，不会再为了投递创建完整 `QueueRunner`。后台管理入口可以使用 `queue.NewManager(cfg)`。Worker/runtime 入口使用 `queue.NewRunner(cfg, opts...)` 创建 `Client + Worker + Manager` 完整实例，或使用 `queue.InitRuntimeInstance` 接入 runtime kernel。

复杂场景也可以通过 `queue.New(cfg, opts...)` 创建独立 runner，并在业务中显式注入 `queue.Client`。可返回错误的 `NewClient`、`NewManager`、`NewRunner` 适合需要在初始化阶段区分 driver 配置错误的入口。

## Runtime Producer Facade

`core/queue/facade/producer` 只负责把队列 producer 接入主 runtime：

```go
import queueproducer "github.com/huwenlong92/sdkit/core/queue/facade/producer"

capability := queueproducer.Use(queueproducer.WithConfig(cfg.Queue))
```

该 capability 默认名称是 `queue.producer`，也支持通过 `WithName(ctx.LocalName(queueproducer.Name))` 注册为 `api.queue.producer` 这类服务本地 producer。它绑定的是 `queue.Client`，不会创建 `queue.Worker` 或 `queue.Manager`。服务 router 使用 `queueproducer.RuntimeFromServiceContext(ctx)` 从 `ServiceContext.Capabilities` 读取本地 producer，再交给 `app/middleware.QueueRuntime` 注入 request context。

`bootstrap` 不再注册 queue marker。需要投递任务的服务自己声明 producer capability；Worker 是队列消费者运行时，不加载 producer capability。

## Runtime Operations Facade

`core/queue/facade/operations` 负责把投递端和管理端一起接入主 runtime：

```go
import queueops "github.com/huwenlong92/sdkit/core/queue/facade/operations"

capability := queueops.Use(queueops.WithConfig(
    queueops.NewConfig(cfg.Name, cfg.Type, cfg.Queue),
))
```

它创建 `queue.Client`、`queue.Manager` 和 `queue.RuntimeInstance`，不创建 `queue.Worker`，不启动消费者。Admin 通过 `queueops.RuntimeFromServiceContext(ctx)` 读取该 capability，并由 Admin 自己注册 `/admin/v1/queue/*` HTTP 路由。

## Runtime Instance

Queue Runtime Platform 的新入口是显式 runtime instance：

```go
runtime := queue.Runtime(ctx)
ops := runtime.Operations()
status, err := ops.RuntimeStatus(ctx)
metadata := runtime.Metadata()
```

`queue.Enqueue(ctx, task, opts...)` 面向 handler 执行链，从 context 读取当前队列实例并投递任务。`queue.Runtime(ctx)` 用于读取 metadata、operations 或 registry 等 runtime 对象。`queue.From(app)` 保留给 provider、server、command 等启动层读取 runtime-managed app 内的队列实例；业务 handler 不直接调用。

Runtime instance 包装：

- `Client`：投递任务
- `Worker`：注册和运行 consumer
- `Manager`：队列查询、retry、archive、cancel、delete、pause、resume
- `RuntimeKernel`：失败处理、重试判断、限流、Outbox poller lifecycle
- `RegistryRuntime`：handler metadata、payload metadata、dispatcher lookup、middleware metadata
- `OperationsRuntime`：admin API 和 queue command 复用的运行时操作入口
- `RuntimeMetadata`：driver、service、worker、queue weight、retry、timeout、delay、priority、trace、middleware、concurrency、rate limit
- `TaskStore`：投递任务、执行记录、执行日志的 driver-neutral 持久化接口
- `TaskSubmissionStore`：投递前和投递结果的持久化接口，用于避免“队列已投递但后台无记录”或“投递失败无记录”
- `TaskScheduleStore`：初始延迟任务的持久化和到期抢占接口

这是 Runtime API Boundary。`core/queue` 只暴露 runtime、operations、metadata、status、metrics 和标准 task/manager/client 接口；不再暴露 Gin、Cobra、HTTP route 或 CLI command 注册函数。

Worker host 启动时通过 `worker/bootstrap.EnsureQueueRuntime` 创建 runtime instance。该 runtime 只属于 worker consumer 生命周期，不写入 `ServiceContext.Capabilities`。

Admin 的队列投递和管理能力属于 service local capability，入口位于：

```txt
core/queue/facade/operations
```

该 capability 负责创建 admin 侧 producer、manager、`RuntimeInstance` 和 `OperationsRuntime`。Runtime wiring 会把实例以 `admin.queue.operations` 写入 `ServiceContext.Capabilities`。Admin router 通过 `queueops.RuntimeFromServiceContext(ctx)` 读取 runtime，交给 `app/middleware.QueueRuntime` 绑定中间件并注册 queue routes，不直接初始化 driver。

业务注册使用 runtime instance 创建 registry：

```go
registry := runtime.NewRegistry()
worker.RegisterEvents(registry)
```

## Registry Runtime

`RegistryRuntime` 是 queue handler 注册和 dispatcher metadata 的平台边界。它负责保存 pattern、handler、payload、queue、retry、timeout、delay、priority、trace 和 middleware 数量等元数据，并通过 `Dispatcher` 执行 lookup 和 middleware pipeline。

业务注册仍保持简单模型：

```go
registry.Register(taskdef.UserSync, usersync.Handle)
```

注册时可以把运行时元数据和 handler 一起声明，metadata 用于 admin、dashboard、monitor 和测试 introspection，不改变 driver kernel：

```go
registry.Register(
    taskdef.UserSync,
    usersync.Handle,
    queue.WithRetry(3),
    queue.WithQueue("critical"),
    queue.WithTimeout(30*time.Second),
)
```

Worker 服务内推荐通过 `runtime.NewRegistry()` 得到绑定当前 worker 的 registry；测试或独立 runner 可以继续使用 `queue.NewRegistry(worker)`。

## Operations Platform

`OperationsRuntime` 包装 `queue.Manager`，统一提供：

- `Status` / `ListQueues` / `GetQueue`
- `RuntimeStatus` / `QueueStatus` / `WorkerStatus`
- `Metrics`
- `ListTasks` / `GetTask`
- `ListFailedTasks` / `CleanTasks`
- `RetryTask` / `ArchiveTask` / `CancelTask` / `DeleteTask`
- `PauseQueue` / `ResumeQueue` / `DrainQueue` / `Drain`

Admin API 和 `sdkitgo queue` command 复用同一套 operations 入口。Admin 和 command 只调用 operations API，不直接操作 worker runtime internals。Operations 属于 Queue Runtime Platform，不属于 worker business handler。

## Task Store

`core/queue` 内置 driver-neutral 的任务存储边界，用于承接后台队列看板需要的三类数据：

- 投递任务：`TaskRecord`
- 执行记录：`TaskRunRecord`
- 执行日志：`TaskRunLogRecord`

核心接口是 `TaskStore`：

```go
type TaskStore interface {
    RecordEnqueued(ctx context.Context, record TaskRecord) error
    EnsureRunning(ctx context.Context, record TaskRecord) (TaskRecord, error)
    StartRun(ctx context.Context, run TaskRunRecord) (TaskRunRecord, error)
    FinishRun(ctx context.Context, run TaskRunRecord) error
    AppendRunLog(ctx context.Context, log TaskRunLogRecord) error
    UpdateTaskStatus(ctx context.Context, update TaskStatusUpdate) error
}
```

`RuntimeInstance.Enqueue` 和 `BatchEnqueue` 在配置 `WithRuntimeTaskStore(store)` 后会自动调用 `RecordTaskEnqueued`。这层只记录核心字段：driver、queue、task id、type、payload、retry、timeout、unique、delay、scheduled time 和 trace/request/track/span。

如果 `store` 实现 `TaskScheduleStore`，future `ProcessIn/ProcessAt` 不会立即交给 driver，而是写成 `scheduled`。worker 启动 `StartSchedulePoller` 后，通过 `ClaimScheduled` 抢占到期任务，投递成功后更新为 `pending`，投递失败后更新为 `submit_failed`。抢占实现应使用数据库行锁或等价机制，避免多个 worker 实例重复投递。因为延迟任务会先停留在存储层，`RecordScheduled` 还应按 queue/type/payload 和 unique 窗口处理 `queue.Unique` 冲突。

如果 `store` 实现 `TaskAutoRetryStore`，应用可以通过 `RuntimeInstance.DispatchAutoRetryTasks(ctx, limit)` 扫描已到期的失败任务并重新投递。`queue.AutoRetry(max, delay)` 只记录自动恢复策略，不替代 driver 的 `queue.MaxRetry(n)`：`MaxRetry` 处理同一次投递内的执行重试；`AutoRetry` 只在最终失败或投递失败后生效，并复用原 `TaskRecord.RecordID` 更新状态。

自动恢复 store 实现应使用可抢占锁，例如 PostgreSQL `FOR UPDATE SKIP LOCKED`，并至少按以下谓词建立小范围索引：`auto_retry_enabled = true`、`status IN ('failed','submit_failed')`、`auto_retry_count < auto_retry_max`、`next_retry_at IS NOT NULL`。查询条件要和 partial index 谓词保持一致，避免参数化条件导致 PostgreSQL 无法使用 partial index。

worker 执行链通过 `TaskStoreMiddleware(store, opts)` 记录每次执行尝试：

```txt
EnsureRunning -> StartRun -> handler -> FinishRun -> UpdateTaskStatus
```

执行 run 状态区分尝试中错误和终态错误：仍会继续重试的尝试写为 `retry`，最终耗尽重试、deadletter 或 fatal error 写为 `failed`。`TaskStore` 实现和后台查询如果需要完整错误尝试链路，应同时读取 `retry` 和 `failed`；自动恢复只应基于 task 终态 `failed` 或 `submit_failed`。

handler 内部通过 `TaskLoggerFromContext(ctx)` 追加执行日志。未配置任务存储 middleware 时返回 `NoopTaskLogger`，不会引入 nil 判断。

`core/queue` 不定义数据库 schema。应用层根据实际存储实现 `TaskStore`，例如：

- `system_queue_task`：投递任务和当前状态
- `system_queue_task_run`：每次执行尝试
- `system_queue_task_run_log`：执行过程日志明细

这三个表可以在应用层按时间做 RANGE 分区，也可以换成 NATS、ClickHouse 或其他存储实现。只要实现 `TaskStore`，Admin 看板不需要感知底层 driver。

Runtime status 使用队列和 worker metadata 组合得出：

```txt
running
paused
draining
stopped
failed
```

Runtime metrics 从 `QueueInfo` 聚合，统一暴露 pending、active、scheduled、retry、archived、succeeded、failed、canceled、processed、failed_all。当前 asynq driver 把 terminal failed 任务映射为 archived，同时把 `QueueInfo.Failed` 作为 archived 数量暴露，便于 dashboard 统一展示 failure metrics。

## 能力矩阵

| Capability | asynq | nats JetStream |
|---|---|---|
| `enqueue` | yes | yes |
| `consume` | yes | yes |
| `retry` | yes | redelivery only |
| `timeout` | yes | yes，作为 handler context deadline |
| `deadline` | yes | no |
| `delay` | DB schedule poller | DB schedule poller |
| `unique` | yes | no，只有 TaskID 可借助 JetStream duplicate window 去重 |
| `priority` | no | no |
| `rate_limit` | queue control | no |
| `inspector` | yes | no，后台查询走 `TaskStore` |
| `pause_resume` | yes | no |
| `schedule` | no，使用 `crontab` 触发后投递队列 | no，使用 `crontab` 触发后投递队列 |
| `batch` | yes | yes |
| `lock` | Redis/Memory | Redis/Memory |
| `idempotency` | Redis/Memory | Redis/Memory |
| `outbox` | DB + Client | DB + Client |
| `log` | yes | yes |
| `trace` | yes | yes |

driver 不支持的能力必须返回 `queue.ErrCapabilityUnsupported`，禁止静默忽略。例如 asynq 不支持 per-task `WithPriority` 和 `WithRateLimitKey`。

JetStream driver 使用 pull durable consumer。多 worker 实例要作为同一消费组扩容时，必须使用相同的 `nats.durable_prefix`；如果 durable 不同，JetStream 会把同一条消息投递给每个 durable，形成重复执行。

入队阶段必须返回 provider-agnostic 错误，调用方不能解析 driver 错误字符串：

| 场景 | 对外错误 |
|---|---|
| 队列未初始化 | `queue.ErrNotInitialized` |
| 唯一任务重复 / TaskID 冲突 | `queue.ErrTaskDuplicated` |
| 不支持的能力 | `queue.ErrCapabilityUnsupported` |
| payload 编码失败 | `queue.ErrInvalidPayload` |
| 任务不存在 | `queue.ErrTaskNotFound` |
| 队列不存在 | `queue.ErrQueueNotFound` |

## 状态流转

统一任务状态：

```txt
scheduled -> pending -> active -> succeeded
scheduled -> pending -> active -> retry -> pending -> active -> succeeded
scheduled -> pending -> active -> retry -> archived
pending   -> canceled
scheduled -> canceled
retry     -> canceled
```

Asynq 映射：

| Asynq | Queue |
|---|---|
| pending | `StatePending` |
| active | `StateActive` |
| scheduled | `StateScheduled` |
| retry | `StateRetry` |
| archived | `StateArchived` |
| completed | `StateSucceeded` |

## 配置方案

Worker 消费和 Crontab 投递复用同一套队列连接配置，放在 `configs/worker.yaml` 的 `worker.queue`：

```yaml
worker:
  queue:
    driver: asynq
    addr: 127.0.0.1:6379
    password: ""
    db: 0
    concurrency: 10
    queues:
      critical: 10
      default: 5
      low: 1
    strict_priority: false
    workers:
      heavy:
        concurrency: 5
        queues:
          heavy: 1
    rate_limit:
      enabled: true
      default_limit: 100
      default_window: 1m
    lock:
      enabled: true
      prefix: "queue:lock:"
    idempotency:
      enabled: true
      prefix: "queue:done:"
    outbox:
      enabled: true
      batch_size: 100
      flush_interval: 5s
```

说明：

| 字段 | 说明 |
|------|------|
| `addr` | Redis 地址 |
| `password` | Redis 密码 |
| `db` | Redis DB |
| `concurrency` | worker 并发数，默认 10 |
| `queues` | 队列权重，默认 `default: 1` |
| `strict_priority` | 是否严格按队列优先级消费，默认 false |
| `workers` | worker profile，支持不同 worker 使用不同并发和队列权重 |
| `rate_limit` | Worker 队列限流治理配置，供 middleware 使用 |
| `outbox` | Worker Outbox poller 配置 |

推荐队列：

```go
map[string]int{
    "critical": 10,
    "default":  5,
    "low":      1,
}
```

## 初始化方案

HTTP 服务只初始化投递能力：

```go
bootstrap.Init(bootstrap.BootConfig{
    ConfigFile:  ConfigFile,
    ServiceName: "admin",
})
```

Worker 服务负责初始化队列、注册业务 handler、启动消费：

```go
cfg, err := appbootstrap.Init(appbootstrap.BootConfig{
    ConfigFile:  ConfigFile,
    Redis:       true,
    ServiceName: "worker",
})
if err != nil {
    logger.L.Fatal("初始化失败", zap.Error(err))
}

workerCfg, err := workerconfig.Load(configFile, "worker", cfg)
if err != nil {
    logger.L.Fatal("Worker配置加载失败", zap.Error(err))
}

srv := worker.NewServer(workerCfg)
srv.Start()
```

`worker/server.go` 的启动顺序：

1. `workerbootstrap.EnsureQueueRuntime(cfg)` 创建 `queue.RuntimeInstance`
2. `worker.RegisterEvents(runtime.NewRegistry())`
3. `runtime.Run(ctx)`

API 和 worker 不应强绑定在同一生命周期里，避免 API 重启导致 worker 中断。独立 worker 服务是默认部署形态。

## Runtime Kernel

`queue.RuntimeKernel` 是队列运行时内核的宿主对象，负责收敛原本分散在 worker bootstrap 中的 runtime 生命周期：

- `NewRunner`：创建 `Client + Worker + Manager` 完整运行实例，不写入任何包级默认实例。
- IsFailure：保存 provider 失败统计判断，例如限流错误默认不计入失败统计。
- Retry：由 `RetryStage` 和 `queue.RetryStrategy` 负责 provider-agnostic 重试治理；driver 只读取 `RateLimitError.RetryIn` 或 `RuntimeError.RetryIn` 作为 transport retry delay。
- RateLimit：保存运行时限流器和规范化后的 `RateLimitConfig`，业务 registry 只读取 `kernel.RateLimiter()`。
- Outbox：通过 `OutboxFactory` 注入具体实现，由 kernel 启动和停止 `OutboxPoller`。
- Orchestrator：持有 runtime event publisher 和 observer，worker registry 通过 kernel 注入的 orchestrator 执行 handler lifecycle。

worker 只负责把当前服务依赖注入给 kernel，例如 Redis rate limiter 和 Gorm outbox；不再自行管理 outbox poller cancel 或 queue runtime option 组合。

Runtime Kernel、Admin host、Worker host 和 `worker/command/queue` 不通过默认实例做 runtime discovery；队列实例必须来自显式 runtime、client、manager 或 operations 注入。

## Registry / Dispatcher

`queue.Registry` 面向 worker 注册事件，`queue.Dispatcher` 面向 runtime 执行事件：

- `Registry.Register(pattern, handlers...)` 记录事件 metadata，并把 dispatcher handler 注册到具体 `Worker`。
- `Registry.RegisterAll(queue.Register(pattern, middleware..., handler)...)` 用于批量声明式注册，保留启动期错误返回。
- `Dispatcher.Register` 负责记录 final handler 和任务级 middleware，支持 `queue.Middleware`、`queue.RuntimeMiddleware`、`queue.ContextHandler` 和 typed payload handler。
- `Dispatcher.Dispatch` 负责按 task type lookup、补齐 `queue.Message`，再交给 orchestrator 执行 stage middleware、runtime lifecycle hooks、task state 和 runtime event。
- `Dispatcher.AddHook` / `Registry.AddHook` 用于注册 `BeforeProcess`、`AfterProcess`、`OnSuccess`、`OnFailure` 生命周期扩展，worker 和 driver 不直接调用 hook。
- `Registry.Use` 不返回 error，不再直接扩散到 driver 级 `Worker.Use`，而是进入 dispatcher pipeline，避免 worker 持有业务 middleware runtime。
- `Registry.UseRuntime` / `Dispatcher.UseRuntime` 用于注册带 stage 的 runtime middleware；同一 stage 内保持注册顺序。

Runtime lifecycle 顺序：

```text
BeforeProcess
  -> middleware chain
      -> handler
AfterProcess
OnSuccess / OnFailure
```

Hook 是任务生命周期监听扩展点，用于在不改变业务 handler 和 middleware chain 的情况下做轻量前后置动作：

- `BeforeProcess`：任务执行前调用，可用于运行环境检查或准备动作；返回 error 会短路后续 middleware 和 handler。
- `AfterProcess`：无论成功失败都会调用，适合收尾、审计或自定义状态记录。
- `OnSuccess`：任务成功后调用。
- `OnFailure`：任务失败后调用。

Hook 适合统一审计、执行前环境校验、执行后状态记录、成功/失败通知和测试生命周期顺序。不适合承载 retry、deadletter、timeout、lock、rate limit、tracing、metrics、logging 等需要包裹执行链或参与治理顺序的逻辑，这些应使用对应 middleware stage。

Runtime orchestrator 是 dispatcher 和 handler 之间的执行编排层：

- `queue.Message.State` 记录执行链内部状态：`TaskPending`、`TaskRunning`、`TaskRetrying`、`TaskFailed`、`TaskDeadLetter`、`TaskSuccess`。
- `queue.Message.Runtime` 保存 runtime scoped context：`TraceID`、`WorkerID`、`QueueName`、`TaskState`。
- `queue.RuntimeEvent` 统一发布 `task.started`、`task.success`、`task.failed`、`task.retry`、`task.deadletter`、`task.timeout`。
- `queue.Observer` 统一接收 `OnTaskStart`、`OnTaskFinish`、`OnTaskRetry`、`OnTaskFailure`，用于聚合 trace、metrics、logs 和 dashboard 状态。
- driver 只负责 transport；handler execution、hook、event、observer 和 state 都从 orchestrator 进入。

新增能力的使用边界：

- `EventPublisher`：用于把 runtime event 推给 dashboard、SSE、WebSocket、task stream 等外部观测系统。publisher 必须轻量，不应阻塞 worker handler；耗时逻辑应自行异步化。
- `Observer`：用于聚合自定义 metrics、任务日志、runtime health 或内存态统计。observer 只观察执行结果，不修改重试、deadletter 或业务结果。
- 已有 realtime 模块不代表必须接 `EventPublisher` / `Observer`。realtime 是传输能力，Runtime Event 是 queue runtime 的事件源；只有需要把任务开始、成功、失败、重试、deadletter、timeout 实时推到管理端或审计流时才接入。
- 如果当前通过 Admin API 查询任务状态，且 tracing/logging/metrics middleware 已满足观测，`RuntimeKernelConfig.EventPublishers` 和 `Observers` 保持为空。
- `RuntimeContext`：供 handler、middleware、hook 读取当前 queue、worker、trace 和 task state。业务代码只读，不主动推进状态。
- `RuntimeError.Kind`：用于 runtime 层统一分类 retryable、fatal、timeout、deadletter、ignored。业务 handler 常用普通 error、`RetryableAfter`、`NewFatalError`、`NewIgnoredError`；timeout 和 deadletter 分类优先由 middleware 或 orchestrator 产生。
- `TaskState` 和 `TaskInfo.State` 含义不同：`TaskState` 的 `TaskRunning/TaskRetrying/...` 表示 handler 执行链内部状态；`TaskInfo.State` 的 `StatePending/StateActive/StateRetry/StateArchived/...` 表示 driver/manager 查询到的队列状态。

typed payload handler 仍保持：

```go
func Handle(ctx context.Context, payload *Payload) error
```

`queue.Message` 会通过 `queue.ContextWithMessage` 放入 `context.Context`，业务事件需要任务 ID、队列名、retry 信息时使用 `queue.MessageFromContext(ctx)`。

## Metadata 边界

`core/queue` 中的 metadata 分为注册期 metadata 和执行期 metadata：

- `HandlerMetadata` 是注册期元数据，来自 `queue.Register(..., queue.WithQueue(...), queue.WithTimeout(...), queue.WithRetry(...))` 等声明。它记录 pattern、handler 名、payload 类型、middleware 数量、默认队列、retry、timeout、delay、priority 和 trace 信息，用于 admin/dashboard/introspection、注册测试和 dispatcher 执行前注入。
- `Message.Metadata` 是执行期元数据，由 dispatcher 在任务执行前写入，供 runtime middleware 读取，例如 `queue.pattern`、`queue.queue`、`queue.max_retry`、`queue.timeout`、`queue.lock.key`、`queue.concurrency.key`、`queue.worker`。

边界：

- metadata 不是业务 payload，业务数据必须放在 `Task.Payload`。
- metadata 不用于跨进程链路传播；trace/request/tenant/user 等传播字段走 `Task.Headers`。
- 业务 handler 通常不直接依赖 `Message.Metadata`，需要业务数据时使用 `queue.DecodePayload[T](msg)`。
- middleware、admin、dashboard、测试和 runtime introspection 可以读取 metadata。
- `queue.WithTimeout(...)` 会进入 `HandlerMetadata`，dispatcher 注册期会基于它生成任务级 timeout middleware。

## Tracing 接入

`core/queue` 在 tracing 启用后会自动创建队列链路 span：

- `Enqueue` 创建 `producer::<task_type>` span。
- `Handle` 包装后的队列消费入口创建 `consumer::<task_type>` span。
- worker middleware 创建 `handler::<task_type>` span，用于承载业务 handler 及其下游 DB/Redis 调用。
- Asynq task headers 透传 `traceparent`、`tracestate`、`baggage`、`X-Track-ID`、`X-Request-ID`，不修改业务 payload。
- queue span 会写入 `trace_id`、`span_id`、`track_id`、`request_id`、`traceparent`，任务信息使用 OpenTelemetry `messaging.*` 语义字段记录。
- driver 和 worker 侧通过 `core/queue` correlation helper 读取 `track_id/request_id/trace_id/span_id`，不直接维护 header 名或 `traceparent` 解析规则。
- queue span 不额外写入 `sd.track_id`、`sd.request_id`、`task_id`、`task_type`、`queue` 这些自定义字段，避免 Jaeger attributes 重复。

业务 handler 必须继续透传收到的 `ctx`，数据库、Redis、SSE、WebSocket 发布等下游调用才能挂到 worker span 下。

## Runtime Host Boundary

Queue Runtime Kernel 不持有 transport ownership：

- Admin 是 HTTP Runtime Host，负责 route、auth、response、validator 和 middleware。
- Worker 是 Queue Runtime Host，负责 runtime lifecycle、consumer、event registry 和 graceful shutdown。
- `worker/command/queue` 是 Queue CLI Host，归 worker 服务持有，负责 Cobra command、参数绑定和输出格式。
- Admin 和 command 只调用 `runtime.Operations()`，不直接操作 runtime internals。
- `pkg/queue` 只保留 driver、control、outbox 等基础设施实现，不再包含 `transport/gin` 或 `transport/cobra`。

Admin 自主注册的队列 API 路径：

```txt
GET    /admin/v1/queue/dashboard
GET    /admin/v1/queue/tasks
GET    /admin/v1/queue/task/runs
GET    /admin/v1/queue/task/run/logs
POST   /admin/v1/queue/task/retry
POST   /admin/v1/queue/task/archive
POST   /admin/v1/queue/task/cancel
POST   /admin/v1/queue/task/delete
GET    /admin/v1/queue/failures
POST   /admin/v1/queue/failure/requeue
POST   /admin/v1/queue/queue/pause
POST   /admin/v1/queue/queue/resume
POST   /admin/v1/queue/queue/drain
```

`worker/command/queue` 自主注册 `sdkitgo queue` 子命令，并复用同一套 `OperationsRuntime`。所有任务查询、retry、archive、cancel、delete、clean、pause、resume、drain、status、metrics 都走 operations API；Outbox 提供 `queue outbox flush` 和 `queue outbox poll`。根 `command` 包只聚合注册，不承载 queue 命令逻辑。

HTTP API 约束：

- GET 只使用 query 参数，并通过 `ShouldBindQuery` 绑定。
- POST 只使用 JSON body，并通过 `ShouldBindBodyWith(&req, binding.JSON)` 绑定。
- 不使用 URL path 参数，例如 `/:id`、`/:queue`。
- 删除任务使用 `POST /admin/v1/queue/task/delete`，不使用 `DELETE` 方法。
- HTTP API 只把 `core/queue` 语义错误转换成统一 response，不暴露 Asynq/Redis 原始错误类型。
- `drain` 在通用 Manager 上退化为 pause + operations draining metadata；如果 driver 实现 `QueueDrainer`，则优先调用 driver 原生 drain。

## Outbox

`core/queue` 提供 `Outbox` 接口、`OutboxTask` 和 `OutboxPoller`；Gorm 实现位于 `pkg/queue/outbox/gorm`，用于“业务数据库事务成功后再投递队列”的场景，避免业务数据已提交但队列投递失败导致状态不一致。

表结构由 `gormoutbox.MigrateOutbox(ctx, db)` 迁移。项目如果需要通过统一模型迁移管理该表，应在自己的 migrate 命令中显式调用对应迁移；bootstrap 不再隐式执行业务 `models.AutoMigrate()`。表名为 `system_queue_outbox`，保存：

- `task_id`、`queue`、`type`
- `payload`、`headers`、`options`
- `status`：`pending` / `sent` / `failed`
- `attempts`、`last_error`
- `available_at`、`sent_at`

用法：

```go
import gormoutbox "github.com/huwenlong92/sdkit/pkg/queue/outbox/gorm"

runtime := queue.Runtime(ctx)
outbox := gormoutbox.NewGormOutbox(tx, runtime.Client())
err := outbox.Save(ctx,
    queue.NewTask(taskdef.TypeUserSync, payload),
    queue.Queue("critical"),
    queue.TaskID("biz-unique-id"),
    queue.MaxRetry(3),
)
```

批量保存：

```go
err := outbox.SaveBatch(ctx,
    queue.NewOutboxTask(taskWechat, queue.TaskID("order-1001-notify-wechat")),
    queue.NewOutboxTask(taskSMS, queue.TaskID("order-1001-notify-sms")),
    queue.NewOutboxTask(taskEmail, queue.TaskID("order-1001-notify-email")),
)
```

后台或调度器调用：

```go
err := outbox.Flush(ctx, 100)
```

约束：

- `Save` 会把当前 context 中的 `traceparent`、`X-Track-ID`、`X-Request-ID` 一起保存，`Flush` 投递后 worker 仍能恢复链路。
- `SaveBatch` 在同一事务内写入多条 outbox 记录，每条记录对应一个真实队列任务。
- `Flush` 使用 `FOR UPDATE SKIP LOCKED` 批量锁定待投递记录，支持多实例并发 flush。
- `Flush` 遇到 `queue.ErrTaskDuplicated` 时将记录标记为 `sent`，用于处理“已投递成功但更新 outbox 状态前进程退出”的重复投递场景。
- `NewOutboxPoller(outbox, cfg)` 提供常驻 flush loop，worker 可通过 `worker.queue.outbox.enabled=true` 自动启动。
- command 提供 `queue outbox flush --limit=100` 和 `queue outbox poll --limit=100 --interval=5s`。

性能与适用场景：

- Outbox 会在业务事务内多写 `system_queue_outbox`，带来额外行写入、索引写入和 WAL 压力。
- 一次 `SaveBatch` 会写多条记录，适合强一致副作用，不适合无边界拆成大量渠道任务。
- poller 查询依赖 `status + available_at` 索引，并使用 `FOR UPDATE SKIP LOCKED`；`batch_size` 和 `flush_interval` 必须按业务量保守配置。
- `sent` 记录会持续增长，生产环境需要清理/归档策略。
- 强一致场景使用 Outbox，例如订单创建后必须触发下游任务；普通可丢通知可以直接 `queue.Enqueue`。
- 多渠道通知量大时，优先投递一个 `notification:fanout` 聚合任务，由 worker 内部拆渠道，避免单个业务事务写入过多 outbox 行。
- `notification:fanout` 当前只保留为文档示例，不作为内置模板；更直接的方式仍然是 `SaveBatch` 写多条独立 outbox 记录。

## Runtime Middleware

`core/queue/runtime/middleware` 提供通用 handler middleware，支持全局挂载和单任务挂载。第三阶段后推荐使用 stage middleware，避免 tracing、metrics、retry、deadletter 等顺序被注册顺序打乱：

```go
import queuemiddleware "github.com/huwenlong92/sdkit/core/queue/runtime/middleware"

registry.UseRuntime(
    queue.StageMiddleware(queue.RecoverStage, queuemiddleware.Recover()),
    queue.StageMiddleware(queue.TraceStage, queuemiddleware.Tracing()),
    queue.StageMiddleware(queue.MetricsStage, queuemiddleware.Metrics(metrics)),
    queue.StageMiddleware(queue.LoggingStage, queuemiddleware.Logging()),
    queue.StageMiddleware(queue.ConcurrencyStage, queuemiddleware.Concurrency(concurrencyLimiter)),
    queue.StageMiddleware(queue.LockStage, queuemiddleware.Lock(locker)),
    queue.StageMiddleware(queue.RetryStage, queuemiddleware.Retry(retryStrategy)),
    queue.StageMiddleware(queue.DeadLetterStage, queuemiddleware.DeadLetter(deadletter)),
)

registry.Register("user:sync", queue.ContextChain(func(c *queue.HandlerContext) error {
    err := c.Next()
    if err != nil {
        return err
    }
    return nil
}), queue.StageMiddleware(queue.RateLimitStage, queuemiddleware.RateLimit(limiter, func(ctx context.Context, msg *queue.Message) (string, int, time.Duration, bool, error) {
    return "user_sync:user:1001", 1, 5 * time.Second, true, nil
})), handleUserSync, queue.WithTimeout(30*time.Second))
```

约束：

- worker 只负责 `driver -> Message -> Dispatcher`，业务 handler 必须从 dispatcher 进入统一 middleware chain。
- 固定执行顺序为 `Recover -> Tracing -> Metrics -> Logging -> Timeout -> RateLimit -> Concurrency -> Lock -> Retry -> DeadLetter -> Business`。默认 worker pipeline 不挂全局 timeout 和全局 rate limit；`queue.WithTimeout(...)` 会在注册期自动生成该任务自己的 `TimeoutStage`，消费侧限流需要在任务注册处显式挂 `RateLimitStage`。全局 middleware 用 `registry.UseRuntime` 注册，任务级治理 middleware 放在 `queue.Register(...)` 的 handler 前；普通 `registry.Use` 仍兼容，默认归入 `BusinessStage`。
- 选择 stage 时按 middleware 职责判断，不按文件位置判断：兜底恢复用 `RecoverStage`；trace/span/correlation 用 `TraceStage`；指标统计用 `MetricsStage`；任务日志用 `LoggingStage`；消费侧限流用 `RateLimitStage`；同类任务并发控制用 `ConcurrencyStage`；业务互斥锁用 `LockStage`；错误重试分类和 retry delay 用 `RetryStage`；终态失败和 deadletter 写入用 `DeadLetterStage`；普通业务校验、业务前后置逻辑默认用 `BusinessStage`。
- 任务级 timeout 不手动挂全局 stage，优先在注册时使用 `queue.WithTimeout(...)`。
- `ContextChain` 是 `c.Next()` 风格；`c.Next()` 会返回后续 middleware 或 handler 的 error，前置/后置逻辑都能感知失败。
- `Metrics` 只依赖 `queue.MetricsRecorder`，记录 `queue_task_total`、`queue_task_success_total`、`queue_task_fail_total`、`queue_task_duration`、`queue_task_retry_total`，不绑定 Prometheus 或 OTel metrics SDK。
- middleware 返回 `queue.RateLimited(retryIn, queue.ErrRateLimited)`，由 driver 使用 `RetryIn` 作为下次重试延迟。
- `apply=false` 时直接放行，适合只对部分任务类型、业务 key 或租户启用。
- `queue.WithTimeout(...)` 是任务级超时声明，会在 dispatcher 注册期生成任务级 timeout middleware；不要在默认 worker pipeline 中使用 `Timeout(d)` 做全局超时。
- `Concurrency(limiter)` 只做单机并发治理，默认按 `Message.Metadata[queue.MessageMetadataConcurrencyKey]` 或 task type 分组。
- `Lock(locker)` 默认读取 `queue.Message.Metadata[queue.MessageMetadataLockKey]` 和 `queue.Message.Metadata[queue.MessageMetadataLockTTL]`；也可以传入 `LockKeyFunc` 按任务 payload 计算锁。
- `Lock` 的 unlock error 只记录日志，不会污染业务 handler 已经返回的成功结果。
- `Retry` 只依赖 `queue.RetryStrategy`，返回 `queue.RuntimeError{Retryable:true}` 并携带 `RetryIn`；asynq driver 只读取该延迟，不自行决定任务类型级重试策略。
- `DeadLetter` 只依赖 `queue.DeadLetter`，在 retry exhausted 或 fatal error 时触发；成功写入 deadletter 后返回 `queue.NewDeadLetterError(err)`，避免外层 retry 再次包装为 retryable。
- `RateLimit` 只依赖 `queue.RateLimiter`，`Lock` 只依赖 `queue.Locker`，middleware 不依赖 asynq、Redis 或具体治理实现。
- runtime error 分类使用 `queue.RuntimeError.Kind`，业务可返回 `queue.NewRetryableError(err)`、`queue.RetryableAfter(d, err)`、`queue.NewFatalError(err)`、`queue.NewIgnoredError(err)`、`queue.NewTimeoutError(err)`、`queue.NewDeadLetterError(err)`。
- `core/queue` 不内置业务策略；具体按用户、租户、任务类型、通知渠道等限流的 key 规则由 worker 自己定义。
- `workermiddleware.RateLimit(limit, window)` 是 worker 侧便捷 helper：`limit` 表示窗口内允许执行的最大任务数，`window` 表示统计窗口；默认按 `handler:<task_type>` 做粗粒度限流。参数无效或 runtime 未配置 `RateLimiter` 时返回 nil。

Middleware 使用边界：

- 普通 `queue.Middleware` 默认归入 `BusinessStage`，适合业务校验、业务前后置逻辑。
- runtime governance 类 middleware 必须显式使用 `queue.StageMiddleware(...)`，例如限流、并发、锁、重试、deadletter。
- 全局 middleware 使用 `registry.UseRuntime(...)`，任务级 middleware 放在 `queue.Register(pattern, middleware..., handler)` 的 handler 前。
- `ContextChain` 提供 `c.Next()` 风格，适合需要清晰前后置逻辑的业务 middleware。
- 需要包裹执行链的能力用 middleware；只监听生命周期的能力用 hook；推送执行状态或聚合观测数据用 event publisher / observer。
- runtime task state 必须通过 `queue.TransitionTaskState(...)` 合法流转；`queue.SetTaskState(...)` 只保留为兼容入口，内部同样走状态机校验。
- `queue.RuntimeEvent.Type` 是 `queue.RuntimeEventType`，事件类型冻结为 `task.started`、`task.success`、`task.failed`、`task.retry`、`task.deadletter`、`task.timeout`。
- `MiddlewareStage` 已冻结为 `Recover -> Trace -> Metrics -> Logging -> Timeout -> RateLimit -> Concurrency -> Lock -> Retry -> DeadLetter -> Business`，maintenance mode 下不再新增 stage。
- `RuntimeContext` 只允许 runtime metadata、runtime resources 标识和 runtime state，不承载业务对象。
- Timeout middleware 只负责 context timeout signal；资源型 runtime 的真实中断由 sandbox/docker/cmd 等资源清理逻辑负责。
- `core/queue/runtime/*` package 结构冻结，后续不继续拆分 runtime 子包。

## Driver 适配规则

新增 driver 时必须：

- 只在 driver 包内依赖第三方 SDK。
- 实现 `RunnerDriver` 并通过启动接线层显式调用 `Register()` / `queue.RegisterDriver`，`core/queue` 根包不自动 import 具体 driver。
- 实现 `Client`、`Worker`、`Manager` 或显式返回 `ErrCapabilityUnsupported`。
- 映射到统一 `TaskInfo`、`QueueInfo`、`TaskState`、`QueueState`。
- 入队时透传 `core/tracking` 和 OpenTelemetry headers。
- 消费时恢复 tracking 到 `ctx`。
- driver 只负责 transport，不承载任务类型级 retry governance；自定义 retry delay 必须由 runtime `RetryStage` 产生 `RuntimeError.RetryIn`。
- 不支持的 option 不允许静默忽略。

## 更新记录

- 2026-05-22：移除旧 provider failure callback、`queue.Failure`、`FailureHandler`、`FailureWriter` 和 `FailureLogger`，任务错误统一由 `TaskStoreMiddleware` 写入执行记录。
- 2026-05-22：`TaskStoreMiddleware` 明确 run 状态语义，重试中的错误尝试为 `retry`，deadletter/fatal/最终失败为 `failed`。
- 2026-05-22：NATS JetStream driver 支持 `queue.Timeout`，并将 timeout 作为消费侧 handler context deadline。
- 2026-05-16：runtime kernel freeze：unlock failure 不影响业务成功；runtime state 统一经 `TransitionTaskState` 合法流转；`RuntimeEvent.Type` 类型化为 `RuntimeEventType`；retry authority 收口到 runtime `RetryStage`，driver 只读取 retry hint。
- 2026-05-16：清理未使用的私有兼容入口，handler chain 和 runtime option 测试统一使用公开 API `BuildHandlerChain`、`ApplyRuntimeOptions`。
- 2026-05-16：第三阶段 runtime orchestrator：新增 middleware stage system、runtime task state、runtime event、observer、runtime scoped context 和 `runtime/orchestrator|state|event|lifecycle|observability` 目录；`Dispatcher` 通过 orchestrator 执行 handler lifecycle。
- 2026-05-16：第二阶段 runtime lifecycle & governance：新增 `Hook` 生命周期、`MetricsRecorder`、`RetryStrategy`、`DeadLetter`、`ConcurrencyLimiter`、`RuntimeError` 分类和对应 runtime middleware；第三阶段后 worker 默认 pipeline 使用 `workermiddleware.RuntimePipelineStages()`。
- 2026-05-16：第一阶段 runtime pipeline 收口：dispatcher 在执行前注入 `queue.Message.Metadata`，新增 `Recover`、`Logging`、`Timeout`、`Lock` runtime middleware；第三阶段后 timeout 改为 `queue.WithTimeout(...)` 驱动的任务级 middleware。
- 2026-05-16：`core/queue.Registry` 新增 `RegisterAll` 和 `queue.Register(pattern, middleware..., handler)` 注册声明，`Registry.Use` 改为无 error 返回；worker 可用 core API 批量注册任务并保留启动期错误返回。
- 2026-05-16：队列 tracing/rate-limit middleware 工厂收敛到 `core/queue/runtime/middleware`，`core/queue` 根包只保留 handler chain 基础类型和标准队列抽象。
- 2026-05-16：新增 `core/queue/facade/producer` producer-only Runtime Capability；新增 `core/queue/facade/operations` 管理端 Runtime Capability；`queue` capability 可按服务选择投递端或投递 + 管理端，不启动 worker。
- 2026-05-16：API/Admin handler 的 queue 投递示例改为使用服务注入的 producer，不再依赖 bootstrap queue 公共能力。
- 2026-05-16：删除 `queue.Default`、`DefaultClient`、`DefaultManager` 以及包级 init/register/manage/close 兼容 API；队列实例必须通过 runtime context、显式 client/manager/operations 或 facade 注入。
- 2026-05-15：建立 Runtime API Boundary，删除 `pkg/queue/transport/gin` 和 `pkg/queue/transport/cobra`，Admin/Command 分别持有 HTTP/Cobra transport，统一调用 `OperationsRuntime`。
- 2026-05-15：新增 `core/queue/runtime_metadata.go` 和 `core/queue/runtime_status.go`，Registry metadata 支持 queue/retry/timeout/delay/priority/trace，Operations runtime 支持 runtime status、worker status、metrics、failed task fallback、clean 和 drain。
- 2026-05-15：新增 `queue.Dispatcher` 和 `queue.RuntimeKernel`，把 registry middleware pipeline、retry option、rate limit state 和 outbox poller 生命周期从 worker bootstrap 收敛到 Queue Runtime Kernel。
- 2026-05-15：新增 `queue.Registry` 和 typed payload handler 适配，worker 注册入口收敛为 `worker.RegisterEvents`，业务事件目录改为 `worker/event`。
- 2026-05-15：新增 `queue.Push` 和 `queue.Delay`，作为默认实例迁移期 helper；`queue.Unique(ttl)` 保持为唯一任务投递选项。
- 2026-05-13：新增 `pkg/queue/outbox/gorm` 作为 DB Outbox Gorm 实现，`core/queue` 仅保留 Outbox 标准接口、`OutboxTask` 和 poller；worker/bootstrap、queue command、models migration 和 worker 集成测试切到新路径。
- 2026-05-13：真实 worker demo 覆盖任务错误记录和 tracing 链路。
- 2026-05-13：队列标准 API 统一收敛到 `core/queue` 根包，具体 driver 位于 `pkg/queue/*`。
- 2026-05-13：完成标准层收口，`Task`、`Message`、`Client`、`Manager`、`Driver`、`Config`、`Option`、`RuntimeOption`、状态和错误模型改由 `core/queue` 根包直接定义。
- 2026-05-13：新增 `pkg/queue/control/redis` 和 `pkg/queue/control/memory` 作为队列锁、幂等、限流实现，`core/queue` 仅保留 `Locker`、`Idempotency`、`RateLimiter` 接口和标准错误 helper；worker Redis 限流接线切到新路径。
- 2026-05-13：队列调度职责收敛到 crontab，Asynq driver 不声明 schedule capability；crontab 统一使用 robfigcron 触发后投递队列。
- 2026-05-13：新增 `pkg/queue/asynq` 作为 Asynq queue driver，worker/admin/queue command 接线切到新路径。
- 2026-05-13：Asynq driver 改为启动接线层显式注册，`core/queue` 根包不再自动 import driver。
- 2026-05-13：公开 driver 合同所需 helper：`ApplyOptions`、`RuntimeOptions`、`ApplyRuntimeOptions`、`DefaultRuntimeOptions`、`CloneCapabilities`，为后续 driver 迁出 `core/queue` 做准备。
- 2026-05-13：新增 `queue.NewClient` / `queue.NewManager`，`InitClient` 改为初始化投递端，`InitManager` 初始化管理端，投递端不再依赖完整 `QueueRunner`。
- 2026-05-13：`queue.New` 通过 driver registry 创建 `QueueRunner`，根包不暴露 Asynq 具体类型。
- 2026-05-13：worker 处理 context 补充 `task_id/queue/type`，任务 header 解析统一从 `traceparent` 提取 `trace_id/span_id`。
- 2026-05-12：worker 执行记录补充 `track_id/request_id/trace_id/span_id`，新增真实 demo 覆盖重试、限流、唯一任务和删除后重投。
- 2026-05-12：Queue tracing operation name 改为 `producer::<task_type>`、`consumer::<task_type>`、`handler::<task_type>`。
- 2026-05-12：队列 driver 引入统一 Capability/State/Manager/API/Command/Governance 抽象。
- 2026-05-12：Queue span 去除重复自定义 attributes，任务信息统一使用 `messaging.*` 语义字段。
- 2026-05-12：新增 Queue OpenTelemetry producer/consumer span，使用 Asynq task headers 透传 trace 和业务追踪字段。
- 2026-05-11：`queue.Config` 归属回 `core/queue`，`InitClient` / `InitServer` 直接接收 `queue.Config`，不再依赖 `core/config.Queue`。

## 任务定义

任务定义放在 `worker/taskdef`，只包含任务类型、payload 和构造函数。

```go
const TypeUserSync = "user:sync"

type UserSyncPayload struct {
    UserID int64  `json:"user_id"`
    Source string `json:"source"`
    Force  bool   `json:"force"`
}

func NewUserSyncTask(userID int64, source string, force bool) queue.Task {
    return queue.NewTask(TypeUserSync, UserSyncPayload{
        UserID: userID,
        Source: source,
        Force:  force,
    })
}
```

`queue.Task.Payload` 支持：

- struct：通过 `core/jsonx` 序列化
- `[]byte`：原样投递
- `string`：转成 `[]byte`
- `nil`：空 payload

业务 payload 推荐使用 struct，并带 JSON tag。

## 投递方案

业务入口使用 `queue.Enqueue` 或注入的 `queue.Client`：

```go
info, err := queue.Enqueue(
    ctx,
    taskdef.NewUserSyncTask(1001, "admin", false),
    queue.Queue(queue.DefaultQueueName),
    queue.MaxRetry(3),
    queue.Timeout(30*time.Second),
    queue.Unique(5*time.Minute),
)
```

常用选项：

| 选项 | 说明 |
|------|------|
| `queue.Queue(name)` | 投递到指定队列 |
| `queue.MaxRetry(n)` | 最大重试次数，负数按 0 处理 |
| `queue.Timeout(d)` | handler 单次执行超时 |
| `queue.Deadline(t)` | handler 截止时间 |
| `queue.ProcessIn(d)` | 延迟执行 |
| `queue.ProcessAt(t)` | 指定时间执行 |
| `queue.TaskID(id)` | 指定任务 ID |
| `queue.Unique(ttl)` | 指定时间内按 type + payload + queue 去重 |
| `queue.AutoRetry(max, delay)` | 终态失败后由应用调度层自动恢复重投 |
| `queue.Retention(d)` | 成功任务保留时间 |
| `queue.Group(name)` | 分组聚合 |

需要复用投递参数时，在 `taskdef` 中提供 helper：

```go
func EnqueueUserSync(ctx context.Context, q queue.Client, payload UserSyncPayload) (*queue.TaskInfo, error) {
    if q == nil {
        return nil, queue.ErrNotInitialized
    }
    return q.Enqueue(ctx,
        queue.NewTask(TypeUserSync, payload),
        queue.Queue(queue.DefaultQueueName),
        queue.MaxRetry(3),
        queue.Timeout(30*time.Second),
        queue.Unique(5*time.Minute),
    )
}
```

## Worker 注册方案

业务事件函数放在 `worker/event`，接收 typed payload：

```go
func HandleUserSync(ctx context.Context, payload *taskdef.UserSyncPayload) error {
    return service.SyncUser(ctx, payload.UserID)
}
```

统一在 `worker/registry.go` 注册：

```go
import workermiddleware "sdkitgo/worker/middleware"

func RegisterEvents(r *queue.Registry) error {
    return r.RegisterAll(
        queue.Register(taskdef.TypeUserSync,
            queue.StageMiddleware(queue.RateLimitStage, workermiddleware.RateLimit(10, time.Minute)),
            event.HandleUserSync,
        ),
    )
}
```

`queue.Registry` 负责记录事件并注册到 `queue.Dispatcher`。`Dispatcher` 负责 handler lookup、middleware pipeline、typed payload 解码，并把 `queue.Message` 放入 context。需要任务 ID、队列名或 retry 信息时，业务事件可以通过 `queue.MessageFromContext(ctx)` 读取；业务事件不接收 `*asynq.Task`，也不直接处理 driver middleware pipeline。

## 重试和限流

普通失败直接返回 error，由队列按 `queue.MaxRetry` 重试：

```go
return err
```

worker 侧可以通过 `RetryStage` 按任务类型定制重试间隔：

```go
registry.UseRuntime(
    queue.StageMiddleware(queue.RetryStage, queuemiddleware.Retry(queue.RetryDelayStrategy(retryDelay))),
)
```

`RuntimeKernelConfig` 不再接收任务类型级 retry delay。driver 只读取 `queue.RateLimited(...)` 或 `queue.RuntimeError.RetryIn` 给出的延迟。

需要按外部服务限流时间重试时，返回 `queue.RateLimited(...)`：

```go
return queue.RateLimited(2*time.Minute, errors.New("remote service rate limited"))
```

`RateLimitError` 的语义：

- 使用 `RetryIn` 作为下次重试延迟
- 默认不计入失败统计

默认失败统计判断：

```go
queue.WithIsFailure(func(err error) bool {
    return !queue.IsRateLimitError(err)
})
```

不希望重试的错误不要在业务层直接依赖 Asynq 的 `SkipRetry`。如确实需要，应先在 `core/queue` 增加 provider-agnostic 的错误类型。

## 失败记录方案

队列任务执行失败时，错误记录由 `TaskStoreMiddleware` 写入任务执行表。`core/queue` 只定义任务存储接口、执行 middleware 和任务日志 context，不绑定具体数据库表。

应用层通常会落三类记录：

- `system_queue_task`：任务索引、payload、当前状态、最近错误和链路字段。
- `system_queue_task_run`：每次执行尝试、attempt、耗时、错误和链路字段。
- `system_queue_task_run_log`：执行过程日志。

`core/queue` 不再维护 provider failure callback、`queue.Failure`、`FailureHandler`、`FailureWriter` 或 `FailureLogger`。标准错误日志由 runtime logging middleware 输出；后台失败列表和失败重投应基于 `system_queue_task_run` 联 `system_queue_task` 查询。

## 固定 TaskID 和重投

固定 `TaskID` 或唯一任务失败后，队列内已有任务可能仍在 Asynq 的 pending、scheduled、retry 或 archived 状态中。再次使用相同 `TaskID` 入队会冲突。

后台重投流程：

1. 从任务索引和执行记录读取 `queue + task_id + type + payload`
2. 调用 `operations.DeleteTask(ctx, row.Queue, row.TaskID)` 删除队列内已有任务
3. 用原始 payload 重新 `queue.Enqueue`

当任务仍停留在 `TaskScheduleStore` 的 `scheduled` 状态、尚未进入 driver 时，driver 删除会返回 not found。`OperationsRuntime.DeleteTask` 会回退到可选 `TaskDeletionStore`，由应用存储实现把对应任务记录标记为 `canceled`，同时解除后续唯一任务冲突。

```go
operations := queue.Runtime(ctx).Operations()
if err := operations.DeleteTask(ctx, row.Queue, row.TaskID); err != nil {
    return err
}

_, err := queue.Enqueue(ctx,
    queue.NewTask(row.Type, row.Payload),
    queue.Queue(row.Queue),
    queue.TaskID(row.TaskID),
    queue.MaxRetry(3),
)
```

`DeleteTask` 不能删除 active 状态任务，后台操作应等待任务结束后再执行。

## 队列统计

通过 CLI 查看队列状态：

```bash
go run ./cmd/sdkitgo queue stats
```

输出包括 pending、active、scheduled、completed、archived、retry、processed、failed 和 latency。

## 日志和 Trace

队列日志统一使用 `core/logger`。当前 Asynq 日志通过 `logger.Asynq("asynq")` 接入。

业务错误日志应统一记录任务上下文：

- trace id 或请求上下文中的链路字段
- task id
- queue
- task type
- retry count
- max retry
- latency
- error

如果后续引入统一 trace，应在 `queue.Task` 或 option 层扩展 provider-agnostic metadata，不要让业务直接操作 Asynq header。

## 适用场景

适合放入队列的任务：

- access log 异步写入
- 邮件、通知、webhook
- 字幕、视频等耗时处理
- 统计同步
- 缓存重建
- 外部系统同步

不适合放入队列的任务：

- 必须在 HTTP 请求内同步返回结果的短路径逻辑
- 强依赖当前事务未提交状态的操作
- panic 作为控制流的 worker 逻辑
- 多实例重复执行的 cron 逻辑

## 约束

- 业务代码使用 `queue.Task`，不要返回 `*asynq.Task`
- 业务 handler 使用 `queue.Message`，不要接收 `*asynq.Task`
- 业务投递使用 `queue.Enqueue(ctx, task, opts...)` 或显式注入的 `queue.Client`
- provider/server/command 启动接线层使用 service context 或显式注入获取 runtime instance；handler 执行链优先使用 `queue.Enqueue(ctx, task, opts...)`
- 业务注册放在 `worker`，`core/queue` 不注册具体业务
- 任务 payload 使用结构体并带 JSON tag
- JSON 编解码使用 `core/jsonx`
- HTTP 响应走应用层 response，core queue 不直接依赖响应协议
- 日志走 `core/logger`
- scheduler/cron 只负责触发，不属于 `core/queue`
- 不要直接在业务中创建 Asynq client/server/mux

## 验收标准

- `core/queue` 对外只有项目内抽象，业务侧不依赖 Asynq 类型
- `core/queue/facade/producer` 的 runtime capability 明确为 producer-only，不启动 worker 或 manager
- `core/queue/facade/operations` 的 runtime capability 明确为 producer + manager，不启动 worker
- HTTP 服务可以只投递任务，不启动 worker
- worker 服务可以注册 handler 并消费多队列任务
- `critical/default/low` 多队列权重生效
- `MaxRetry`、`Timeout`、`ProcessIn`、`ProcessAt`、`TaskID`、`Unique`、`Retention` 和 `Group` 可正常映射到 provider
- 普通错误按配置重试
- `queue.RateLimited(...)` 使用指定延迟，并默认不计入失败统计
- 执行记录能记录 `track_id/request_id/trace_id/span_id`
- 固定 `TaskID` 失败后可以先删除队列内已有任务再重投
- 真实 Redis/DB 集成 demo 使用短唯一队列名，避免被本机其他 worker 抢消费
- 队列统计命令可查看各队列状态
- Runtime Instance、Registry Runtime 和 Operations Runtime 均可在不新增包级默认实例依赖的情况下被测试和复用
