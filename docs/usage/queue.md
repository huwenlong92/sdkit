# 队列

`core/queue` 是异步任务基础设施入口。业务代码不要直接创建 `asynq.Client`、`asynq.Server` 或 `asynq.ServeMux`，也不要 import 第三方队列 SDK。

当前默认 driver 是 Asynq + Redis，但业务侧只依赖 `core/queue` 的抽象。`queue.NewClient` 会按 `queue.Config.Driver` 从 driver registry 创建投递端；`queue.NewRunner` 创建完整运行实例，不暴露 Asynq 具体类型。

`core/queue` 不自动注册具体 driver。服务启动接线层需要在首次 `NewClient`、`NewManager` 或 `NewRunner` 前注册 driver：

```go
import asynqdriver "github.com/huwenlong92/sdkit/pkg/queue/asynq"

asynqdriver.Register()
```

标准类型和接口都从 `core/queue` 根包导入，业务代码不使用内部路径或具体 driver 路径。

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
```

定时任务不放在 `core/queue`。需要按 cron 或 DB 配置触发任务时，在 `crontab` 中触发 `queue.Enqueue(...)`。

## 配置

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
    workers:
      heavy:
        concurrency: 5
        queues:
          heavy: 1
    lock:
      enabled: true
      prefix: "queue:lock:"
    idempotency:
      enabled: true
      prefix: "queue:done:"
```

字段说明：

| 字段 | 说明 |
|------|------|
| `addr` | Redis 地址 |
| `password` | Redis 密码 |
| `db` | Redis DB |
| `concurrency` | worker 并发数，默认 10 |
| `queues` | 队列权重，默认只消费 `default` |
| `strict_priority` | 是否严格按优先级消费，默认 false |
| `workers` | worker profile 配置，适合 default/heavy/low 等独立 worker |

## 初始化边界

HTTP 服务只需要能投递任务：

```go
bootstrap.Init(bootstrap.BootConfig{
    ConfigFile:  ConfigFile,
    ServiceName: "admin",
})
```

如果服务只需要投递队列任务，可以显式创建投递端：

```go
client, err := queue.NewClient(cfg.Queue)
if err != nil {
    return err
}

_, err = client.Enqueue(ctx, task)
```

HTTP handler 中应使用当前服务注入的 producer：

```go
info, err := queue.Enqueue(
    c.Request.Context(),
    queue.NewTask("user:sync", gin.H{"user_id": userID}),
    queue.Unique(5*time.Minute),
)
```

`queue.Unique(ttl)` 仍是投递选项，用于唯一任务控制。

需要显式依赖注入时，直接创建 `queue.Client`：

```go
client, err := queue.NewClient(cfg.Queue)
if err != nil {
    return err
}
```

## Runtime Producer Capability

`core/queue/facade/producer` 提供 producer runtime capability 接入。它只代表队列投递端，不启动 worker，也不注册业务 handler：

```go
import queueproducer "github.com/huwenlong92/sdkit/core/queue/facade/producer"

app.Use(queueproducer.Use(
    queueproducer.WithConfig(cfg.Queue),
))
```

默认 capability 名称是 `queue`。服务本地 producer 必须使用服务命名空间：

```go
queueproducer.Use(
    queueproducer.WithName(ctx.LocalName(queueproducer.Name)),
    queueproducer.WithConfig(cfg.Queue),
)
```

`bootstrap` 不注册 queue marker。需要投递任务的 HTTP 服务自行声明 producer capability；Worker 是队列消费者运行时，不加载 producer capability。

后台管理入口需要投递和管理任务时，使用 `core/queue/facade/operations`：

```go
import queueops "github.com/huwenlong92/sdkit/core/queue/facade/operations"

queueops.Use(
    queueops.WithName(ctx.LocalName(queueops.Name)),
    queueops.WithConfig(queueops.NewConfig(cfg.Name, cfg.Type, cfg.Queue)),
)
```

Worker 服务负责初始化队列、注册 handler、启动消费者：

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

`worker/server.go` 中的启动顺序是：

1. `workerbootstrap.EnsureQueueRuntime(cfg)` 创建 `queue.RuntimeInstance`
2. `worker.RegisterEvents(runtime.NewRegistry())`
3. `runtime.Run(ctx)`

启动层优先通过 facade 或 runtime instance 读取队列能力：

```go
runtime := queueops.RuntimeFromServiceContext(serviceCtx)
ops := runtime.Operations()
metadata := runtime.Metadata()
```

handler 执行链内直接使用当前 context 中注入的队列能力投递任务：

```go
info, err := queue.Enqueue(ctx, task, opts...)
```

`ctx` 必须来自已注入 queue runtime 的入口，例如 HTTP request context、worker runtime context，或显式 `queue.ContextWithRuntime(...)`。新增 handler/business 代码投递任务使用 `queue.Enqueue(ctx, task, opts...)`，队列管理使用 `queue.Runtime(ctx).Operations()` 或显式 `OperationsRuntime`。`queue.From(app)` 只保留在 runtime wiring、provider startup 和 bootstrap lifecycle 中。`core/queue` 不提供 route register 或 command register；HTTP/Cobra transport 由 Admin/Command host 自己持有。

Admin 的队列投递端和管理端使用通用 operations facade，以 service local capability 方式注册：

```go
import queueops "github.com/huwenlong92/sdkit/core/queue/facade/operations"

queueops.Use(
    queueops.WithName(ctx.LocalName(queueops.Name)),
    queueops.WithConfig(queueops.NewConfig(cfg.Name, cfg.Type, cfg.Queue)),
)
```

该能力会创建 admin 侧 producer、manager 和 `queue.RuntimeInstance`。Runtime wiring 会把实例以 `admin.queue.operations` 写入 `ServiceContext.Capabilities`。Admin router 通过 `queueops.RuntimeFromServiceContext(ctx)` 读取 runtime，交给 `app/middleware.QueueRuntime` 挂载中间件，并注册 `/admin/v1/queue/*` 路由。

API 的 queue demo 通过 `core/queue/facade/producer` 注册 `api.queue.producer` producer，handler 从 request context 中读取当前服务的 queue runtime：

```go
info, err := queue.Enqueue(
    c.Request.Context(),
    taskdef.NewExampleTask("hello"),
)
```

worker 可以在初始化 Queue Runtime Kernel 时接入失败回调、失败统计判断、限流器和 outbox poller：

```go
runtime, err := queue.InitRuntimeInstance(ctx, cfg.Queue, queue.RuntimeKernelConfig{
    FailureHandler: queue.LogFailureHandler("Worker队列任务失败"),
    FailureWriter:  workerbootstrap.NewQueueFailureWriter(database.DB, 100),
    IsFailure: func(err error) bool {
        return !queue.IsRateLimitError(err)
    },
})
defer runtime.Close()
```

Queue Runtime Kernel 会创建 runtime orchestrator。需要把任务事件推给 dashboard、SSE、WebSocket 或自定义指标聚合时，通过 `RuntimeKernelConfig` 注入 publisher / observer：

```go
runtime, err := queue.InitRuntimeInstance(ctx, cfg.Queue, queue.RuntimeKernelConfig{
    EventPublishers: []queue.EventPublisher{publisher},
    Observers:       []queue.Observer{observer},
})
```

orchestrator 统一维护 handler 执行期间的 `queue.Message.State` 和 `queue.Message.Runtime`。业务 middleware 或 handler 读取当前任务上下文时，优先使用：

```go
msg, _ := queue.MessageFromContext(ctx)
runtimeCtx, _ := queue.RuntimeContextFromContext(ctx)
```

runtime task state 用于执行链内部观测：

```txt
pending -> running -> success
running -> retrying
running -> failed -> deadletter
```

driver manager 查询仍返回 provider 状态，例如 `StatePending`、`StateActive`、`StateRetry`、`StateArchived`。

如果需要失败日志批量入库，把 writer 交给 `queue.RuntimeKernel`：

```go
runtime, err := queue.InitRuntimeInstance(ctx, cfg.Queue, queue.RuntimeKernelConfig{
    FailureHandler: queue.LogFailureHandler("Worker队列任务失败"),
    FailureWriter:  workerbootstrap.NewQueueFailureWriter(database.DB, 100),
    FailureLog: queue.FailureLogConfig{
        QueueSize:     1024,
        BatchSize:     100,
        FlushInterval: 200 * time.Millisecond,
    },
})
defer runtime.Close() // 退出时 flush 剩余失败日志并关闭底层队列资源
```

## 定义任务

任务定义放在 `worker/taskdef`，只包含类型、payload 和构造函数。

```go
package taskdef

import "github.com/huwenlong92/sdkit/core/queue"

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

`queue.Task.Payload` 可以是 struct、`[]byte`、`string` 或 `nil`。struct 会通过 `core/jsonx` 序列化。

## 投递任务

在 handler 或业务入口中投递：

```go
task := taskdef.NewUserSyncTask(1001, "admin", false)

info, err := queue.Enqueue(
    c.Request.Context(),
    task,
    queue.Queue(queue.DefaultQueueName),
    queue.MaxRetry(3),
    queue.Timeout(30*time.Second),
    queue.Unique(5*time.Minute),
)
if err != nil {
    response.Error(c, apperrors.NewCodeWithData(apperrors.CodeOperationFailed, err.Error(), nil))
    return
}

response.Success(c, gin.H{"task_id": info.ID})
```

常用选项：

| 选项 | 说明 |
|------|------|
| `queue.Queue(name)` | 投递到指定队列 |
| `queue.MaxRetry(n)` | 最大重试次数 |
| `queue.Timeout(d)` | handler 单次执行超时 |
| `queue.ProcessIn(d)` | 延迟多久执行 |
| `queue.ProcessAt(t)` | 指定时间执行 |
| `queue.TaskID(id)` | 指定任务 ID |
| `queue.Unique(ttl)` | 指定时间内按 type + payload + queue 去重 |
| `queue.Retention(d)` | 成功任务保留时间 |
| `queue.Group(name)` | 分组聚合 |
| `queue.WithPriority(n)` | 当前 asynq driver 不支持，返回 `ErrCapabilityUnsupported` |
| `queue.WithRateLimitKey(key)` | 当前 asynq driver 不支持，由治理层 `RateLimiter` 处理 |

需要更方便复用时，可以在 `taskdef` 中提供投递 helper：

```go
func EnqueueUserSync(ctx context.Context, q queue.Client, payload UserSyncPayload) (*queue.TaskInfo, error) {
    return q.Enqueue(ctx,
        queue.NewTask(TypeUserSync, payload),
        queue.Queue(queue.DefaultQueueName),
        queue.MaxRetry(3),
        queue.Timeout(30*time.Second),
        queue.Unique(5*time.Minute),
    )
}
```

新增代码建议从 runtime 或依赖注入拿投递端：

```go
runtime := queue.Runtime(ctx)
info, err := taskdef.EnqueueUserSync(ctx, runtime.Client(), payload)
```

投递阶段的错误必须在调用方处理。队列是异步执行，但入队失败、唯一任务重复、固定 TaskID 冲突、Redis 不可用这些都是同步可感知结果：

```go
info, err := taskdef.EnqueueSendNotification(ctx, producer, taskdef.SendNotificationPayload{
    UserID:  userID,
    Title:   title,
    Content: content,
})
if errors.Is(err, queue.ErrTaskDuplicated) {
    response.Error(c, apperrors.NewCodeWithData(apperrors.CodeQueueTaskConflict, "任务已存在，请勿重复提交", nil))
    return
}
if err != nil {
    response.Error(c, apperrors.NewCodeWithData(apperrors.CodeOperationFailed, err.Error(), nil))
    return
}

response.Success(c, gin.H{"task_id": info.ID})
```

常见投递错误：

| 错误 | 处理建议 |
|------|----------|
| `queue.ErrTaskDuplicated` | 返回“任务已存在”，不要当作系统异常 |
| `queue.ErrNotInitialized` | 返回服务不可用或初始化失败 |
| `queue.ErrCapabilityUnsupported` | 返回参数/能力不支持 |
| `queue.ErrInvalidPayload` | 返回请求 payload 不合法 |

## 批量投递

```go
infos, err := queue.BatchEnqueue(ctx, []queue.Task{
    queue.NewTask(taskdef.TypeUserSync, payload1),
    queue.NewTask(taskdef.TypeUserSync, payload2),
}, queue.Queue(queue.DefaultQueueName))
```

当前 asynq driver 按顺序投递，遇到错误会返回已投递的结果和错误。

## Outbox 投递

业务写库和投递队列必须保持一致时，使用 DB Outbox。典型流程是：在业务事务内调用 `Save`，事务提交后由后台任务或调度器调用 `Flush`。

Gorm 实现位于 `pkg/queue/outbox/gorm`，示例中使用：

```go
import gormoutbox "github.com/huwenlong92/sdkit/pkg/queue/outbox/gorm"
```

```go
err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    if err := tx.Create(&order).Error; err != nil {
        return err
    }

    runtime := queue.Runtime(ctx)
    outbox := gormoutbox.NewGormOutbox(tx, runtime.Client())
    return outbox.Save(ctx,
        queue.NewTask(taskdef.TypeUserSync, payload),
        queue.Queue("critical"),
        queue.TaskID("order-sync-1001"),
        queue.MaxRetry(3),
    )
})
```

同一业务动作需要投递多个任务时，使用 `SaveBatch`。例如订单创建后同时发微信、短信、邮件：

```go
err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    if err := tx.Create(&order).Error; err != nil {
        return err
    }

    runtime := queue.Runtime(ctx)
    outbox := gormoutbox.NewGormOutbox(tx, runtime.Client())
    return outbox.SaveBatch(ctx,
        queue.NewOutboxTask(
            queue.NewTask(taskdef.TypeSendNotification, taskdef.SendNotificationPayload{
                UserID:  order.UserID,
                Title:   "微信通知",
                Content: "订单已创建",
            }),
            queue.Queue("critical"),
            queue.TaskID(fmt.Sprintf("order-%d-notify-wechat", order.ID)),
            queue.MaxRetry(3),
        ),
        queue.NewOutboxTask(
            queue.NewTask(taskdef.TypeSendNotification, taskdef.SendNotificationPayload{
                UserID:  order.UserID,
                Title:   "短信通知",
                Content: "订单已创建",
            }),
            queue.Queue("critical"),
            queue.TaskID(fmt.Sprintf("order-%d-notify-sms", order.ID)),
            queue.MaxRetry(3),
        ),
        queue.NewOutboxTask(
            queue.NewTask(taskdef.TypeSendNotification, taskdef.SendNotificationPayload{
                UserID:  order.UserID,
                Title:   "邮件通知",
                Content: "订单已创建",
            }),
            queue.Queue("critical"),
            queue.TaskID(fmt.Sprintf("order-%d-notify-email", order.ID)),
            queue.MaxRetry(3),
        ),
    )
})
```

每个 `OutboxTask` 会生成一条独立 outbox 记录，必须使用不同 `TaskID`，这样微信、短信、邮件可以独立投递、独立重试、独立失败排查。

可选聚合任务示例：

如果业务不想在事务里写多条 outbox，可以只写一条 `notification:fanout` 聚合任务。这个方式不是当前默认模板，只作为多渠道通知量较大时的备选设计。

```go
type NotificationFanoutPayload struct {
    BizType  string                `json:"biz_type"`
    BizID    string                `json:"biz_id"`
    UserID   int64                 `json:"user_id"`
    Title    string                `json:"title"`
    Content  string                `json:"content"`
    Channels []NotificationChannel `json:"channels"`
}

type NotificationChannel struct {
    Type string         `json:"type"` // wechat / sms / email
    Data map[string]any `json:"data,omitempty"`
}

func NewNotificationFanoutTask(payload NotificationFanoutPayload) queue.Task {
    return queue.NewTask("notification:fanout", payload)
}
```

Outbox 写法：

```go
payload := NotificationFanoutPayload{
    BizType: "order_created",
    BizID:   fmt.Sprintf("%d", order.ID),
    UserID:  order.UserID,
    Title:   "订单已创建",
    Content: "订单已创建成功",
    Channels: []NotificationChannel{
        {Type: "wechat"},
        {Type: "sms"},
        {Type: "email"},
    },
}

err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    if err := tx.Create(&order).Error; err != nil {
        return err
    }

    runtime := queue.Runtime(ctx)
    outbox := gormoutbox.NewGormOutbox(tx, runtime.Client())
    return outbox.Save(ctx,
        NewNotificationFanoutTask(payload),
        queue.Queue("critical"),
        queue.TaskID(fmt.Sprintf("order-%d-notification-fanout", order.ID)),
        queue.MaxRetry(3),
        queue.Timeout(30*time.Second),
    )
})
```

worker 处理时再拆渠道：

```go
func HandleNotificationFanout(ctx context.Context, msg *queue.Message) error {
    payload, err := queue.DecodePayload[NotificationFanoutPayload](msg)
    if err != nil {
        return err
    }

    for _, ch := range payload.Channels {
        switch ch.Type {
        case "wechat":
            // send wechat
        case "sms":
            // send sms
        case "email":
            // send email
        default:
            return fmt.Errorf("unsupported notification channel: %s", ch.Type)
        }
    }
    return nil
}
```

fanout handler 需要自己定义渠道级失败策略：整体失败后重试全部渠道，或者记录渠道级状态后只补偿失败渠道。

性能取舍：

- `Save` / `SaveBatch` 会在业务事务内写 `system_queue_outbox`，会增加事务耗时和数据库写入压力。
- Outbox 只建议用于“业务提交后必须投递”的强一致任务。
- 普通可丢通知、埋点、非关键广播可以直接 `queue.Enqueue`。
- 多渠道通知量大时，不建议一单写几十条 outbox；优先写一个 `notification:fanout` 聚合任务，让 worker 再拆微信、短信、邮件。
- 生产环境需要清理或归档 `sent` 记录，否则 outbox 表会持续增长。

手动 Flush：

```go
runtime := queue.Runtime(ctx)
outbox := gormoutbox.NewGormOutbox(database.DB, runtime.Client())
if err := outbox.Flush(ctx, 100); err != nil {
    logger.WithContext(ctx, logger.L).Warn("queue outbox flush failed", zap.Error(err))
}
```

常驻 poller：

```go
poller := queue.NewOutboxPoller(outbox, queue.OutboxPollerConfig{
    BatchSize:     100,
    FlushInterval: 5 * time.Second,
})
go poller.Run(ctx)
```

命令：

```bash
go run ./cmd/sdkitgo queue outbox flush --limit=100
go run ./cmd/sdkitgo queue outbox poll --limit=100 --interval=5s
```

Outbox 会保存 payload、headers 和投递 options。headers 包含当前 context 的 tracking/request/tracing 信息，所以 worker 失败日志仍能写入 `track_id/request_id/trace_id/span_id`。

当前已实现：`Save`、`SaveBatch`、`Flush`、常驻 poller、独立 command、DB 迁移、重复任务语义处理、真实 DB/Redis 集成测试。

当前未实现：outbox 管理 API/页面、失败记录清理/归档、按业务类型分片 flush。

### 文件上传任务

`worker/taskdef.FileGenerateUploadPayload` 是内置文件上传任务 payload，适合把 worker 生成或获取到的文件异步保存到 filesystem 支持的后端。

按文件来源选择字段：

| 字段 | 场景 | 说明 |
|------|------|------|
| `temp_file_path` | worker 业务过程生成临时文件，例如每日 Excel 报表 | 上传成功后删除临时文件，失败时保留给队列重试 |
| `source_url` | 拉取第三方视频、图片、附件后保存 | worker 直接从 URL 流式读取，不把文件内容写入队列 |
| `source_path` | worker 可访问的普通本机路径或共享卷文件 | 上传成功后不删除源文件 |
| `content` | 小文本、小 JSON、测试数据 | 只用于小内容输入，不要用于大文件 |

字段优先级为 `source_url` > `temp_file_path` > `source_path` > `content`。详细用法见 [worker-file-upload.md](worker-file-upload.md)。

## 注册 Worker

业务事件放在 `worker/event`：

```go
func HandleUserSync(ctx context.Context, payload *taskdef.UserSyncPayload) error {
    return service.SyncUser(ctx, payload.UserID)
}
```

在 `worker/registry.go` 注册，worker 级 middleware 从 `worker/middleware` 导入：

```go
import workermiddleware "sdkitgo/worker/middleware"

func RegisterEvents(r *queue.Registry) error {
    return r.RegisterAll(
        queue.Register(taskdef.TypeUserSync,
            queue.StageMiddleware(queue.RateLimitStage, workermiddleware.RateLimit(10, time.Minute)),
            event.HandleUserSync,
            queue.WithRetry(3),
            queue.WithQueue("critical"),
            queue.WithTimeout(30*time.Second),
        ),
    )
}
```

可以通过 middleware 包装 handler：

```go
import workermiddleware "sdkitgo/worker/middleware"

func RegisterEvents(r *queue.Registry) error {
    r.UseRuntime(workermiddleware.RuntimePipelineStages()...)
    return r.RegisterAll(
        queue.Register(taskdef.TypeUserSync,
            queue.StageMiddleware(queue.RateLimitStage, workermiddleware.RateLimit(10, time.Minute)),
            event.HandleUserSync,
            queue.WithTimeout(30*time.Second),
        ),
    )
}
```

`queue.Registry` 会把事件注册到 `queue.Dispatcher`，dispatcher 负责 handler lookup、middleware pipeline、typed payload 解码，并把 `queue.Message` 和 `queue.ContextMetadata` 放入 context。需要任务元信息时使用 `queue.MessageFromContext(ctx)` 或 `queue.MetadataFromContext(ctx)`，`queue.Message` 中包含：

| 字段 | 说明 |
|------|------|
| `ID` | Asynq 任务 ID |
| `Type` | 任务类型 |
| `Payload` | 原始 payload |
| `Queue` | 当前消费队列 |
| `RetryCount` | 当前重试次数 |
| `MaxRetry` | 最大重试次数 |
| `Headers` | 透传的 trace/request/track 等任务 headers |
| `Metadata` | dispatcher 注入的 runtime metadata，例如注册期 timeout、queue、retry、trace |

Registry metadata 可通过 `registry.Metadata()` 或 `runtime.Registry().Metadata()` 读取，包含 pattern、handler、payload、queue、retry、timeout、delay、priority、trace 和 middleware 信息，用于 admin、dashboard、monitor 做 introspection。`queue.WithTimeout(...)` 是任务级声明，dispatcher 注册时会自动为该任务生成 timeout middleware；默认 worker pipeline 不启用全局 timeout。

metadata 使用边界：

- `HandlerMetadata` 是注册期信息，描述 task handler 的注册声明，例如 pattern、handler、payload 类型、默认队列、retry、timeout。
- `Message.Metadata` 是执行期信息，dispatcher 在执行前写入，主要给 runtime middleware 使用。
- metadata 不是业务 payload；业务数据放在 `Task.Payload`，handler 中用 `queue.DecodePayload[T](msg)` 读取。
- trace/request/tenant/user 等跨进程传播字段走 `Task.Headers`，不走 metadata。
- 业务 handler 通常不直接依赖 `Message.Metadata`，除非是在写 runtime middleware 或管理端 introspection。

选择 `MiddlewareStage` 时按职责判断：

| 职责 | Stage |
|------|-------|
| 捕获 panic | `queue.RecoverStage` |
| trace、span、correlation | `queue.TraceStage` |
| 指标统计 | `queue.MetricsStage` |
| 任务日志 | `queue.LoggingStage` |
| 消费侧限流 | `queue.RateLimitStage` |
| 同类任务并发控制 | `queue.ConcurrencyStage` |
| 业务互斥锁 | `queue.LockStage` |
| 错误重试分类、retry delay | `queue.RetryStage` |
| 终态失败、deadletter 写入 | `queue.DeadLetterStage` |
| 普通业务校验和业务前后置逻辑 | `queue.BusinessStage` |

任务级超时优先使用 `queue.WithTimeout(...)`，不要在默认 worker pipeline 中挂全局 timeout。

Middleware 本质是包裹 `HandlerFunc`：

```go
type Middleware func(queue.HandlerFunc) queue.HandlerFunc
```

普通业务校验、业务前后置逻辑可以直接作为任务级 middleware 注册，默认归入 `BusinessStage`：

```go
func ValidateUserSync() queue.Middleware {
    return func(next queue.HandlerFunc) queue.HandlerFunc {
        return func(ctx context.Context, msg *queue.Message) error {
            if msg == nil {
                return queue.ErrInvalidPayload
            }
            return next(ctx, msg)
        }
    }
}

queue.Register(
    taskdef.TypeUserSync,
    ValidateUserSync(),
    event.HandleUserSync,
)
```

治理类 middleware 应显式指定 stage，避免落入默认 `BusinessStage` 后顺序不符合治理语义：

```go
queue.Register(
    taskdef.TypeUserSync,
    queue.StageMiddleware(queue.RateLimitStage, workermiddleware.RateLimit(10, time.Minute)),
    event.HandleUserSync,
)
```

全局 middleware 用 `registry.UseRuntime(...)`，对所有任务生效：

```go
registry.UseRuntime(
    queue.StageMiddleware(queue.RecoverStage, queuemiddleware.Recover()),
    queue.StageMiddleware(queue.TraceStage, queuemiddleware.Tracing()),
    queue.StageMiddleware(queue.LoggingStage, queuemiddleware.Logging()),
    queue.StageMiddleware(queue.RetryStage, queuemiddleware.Retry(strategy)),
)
```

需要在测试或适配层手动组装 handler chain 时，使用公开 API `queue.BuildHandlerChain(...)`。`core/queue` 不再保留小写私有兼容包装入口。

需要 `c.Next()` 风格的前后置逻辑时使用 `ContextChain`：

```go
queue.ContextChain(func(c *queue.HandlerContext) error {
    err := c.Next()
    if err != nil {
        return err
    }
    return nil
})
```

Hook 用于监听任务生命周期，不用于包裹业务执行链：

```go
registry.AddHook(queue.HookFunc{
    Before: func(ctx context.Context, msg *queue.Message) error {
        return nil
    },
    After: func(ctx context.Context, msg *queue.Message, err error) {
    },
    Success: func(ctx context.Context, msg *queue.Message) {
    },
    Failure: func(ctx context.Context, msg *queue.Message, err error) {
    },
})
```

执行顺序是 `BeforeProcess -> middleware chain -> handler -> AfterProcess -> OnSuccess/OnFailure`。`BeforeProcess` 返回 error 会短路后续执行。Hook 适合统一审计、执行前环境校验、执行后状态记录、成功/失败通知和测试生命周期顺序；retry、deadletter、timeout、lock、rate limit、tracing、metrics、logging 这类需要包裹执行链或参与治理顺序的逻辑应使用 middleware stage。

Runtime Event 和 Observer 用于把任务执行状态推给外部观测系统。它们在 `RuntimeKernelConfig` 注入，由 orchestrator 统一调用：

```go
type QueueEventPublisher struct{}

func (QueueEventPublisher) Publish(ctx context.Context, event queue.RuntimeEvent) {
    // event.Type 是 queue.RuntimeEventType:
    // task.started / task.success / task.failed / task.retry / task.deadletter / task.timeout
    // event.Message: 当前任务消息
    // event.Error: 失败原因
}

runtime, err := queue.InitRuntimeInstance(ctx, cfg.Queue, queue.RuntimeKernelConfig{
    EventPublishers: []queue.EventPublisher{
        QueueEventPublisher{},
    },
    Observers: []queue.Observer{
        queue.ObserverFunc{
            Start: func(ctx context.Context, msg *queue.Message) {
            },
            Finish: func(ctx context.Context, msg *queue.Message, err error) {
            },
        },
    },
})
```

使用建议：

- dashboard、SSE、WebSocket 推送任务状态时用 `EventPublisher`。
- 聚合自定义 metrics、audit 或 runtime health 时用 `Observer`。
- publisher / observer 必须轻量，避免阻塞 worker handler；耗时逻辑应转成异步队列或内部缓冲。
- 不要在 publisher / observer 中修改业务结果、重试策略或 deadletter 行为，这些属于 middleware stage 或业务 handler。
- 已有 realtime 能力时，默认不需要接 `EventPublisher` / `Observer`。只有当 Admin/dashboard/SSE/WebSocket 需要实时展示 queue runtime 状态，或需要 queue audit stream 时，才把 Runtime Event 接到现有 realtime 发布器。
- 如果任务状态通过 Admin API 查询、tracing/log/metrics 已满足观测，保持 `RuntimeKernelConfig.EventPublishers` 和 `Observers` 为空即可。

RuntimeContext 和 task state 用于读取当前执行态：

```go
msg, _ := queue.MessageFromContext(ctx)
runtimeCtx, _ := queue.RuntimeContextFromContext(ctx)

_ = msg.State              // pending / running / retrying / failed / deadletter / success
_ = runtimeCtx.QueueName   // 当前队列
_ = runtimeCtx.TraceID     // 当前 trace id
_ = runtimeCtx.TaskState   // 当前任务执行态
```

使用建议：

- handler、middleware、hook 内需要任务状态、队列名、trace id 时读取 `RuntimeContextFromContext`。
- admin 查询队列内任务状态时仍使用 `TaskInfo.State`，它表示 provider 状态，例如 `StatePending`、`StateActive`、`StateRetry`、`StateArchived`。
- 不要直接写 `msg.State`。runtime middleware 内需要推进状态时使用 `queue.TransitionTaskState(...)`，非法流转会返回 `false`。
- `RuntimeContext` 只放 runtime metadata、runtime resources 标识和 runtime state，不放业务 payload、业务对象或外部 SDK client。

RuntimeError 支持错误分类：

```go
return queue.RetryableAfter(5*time.Second, err) // 可重试，并指定延迟
return queue.NewFatalError(err)                 // 终态失败
return queue.NewIgnoredError(err)               // 忽略失败统计
return queue.NewTimeoutError(err)               // 超时分类
return queue.NewDeadLetterError(err)            // 直接进入 deadletter 分类
```

业务 handler 通常只需要返回普通 error 或 `RetryableAfter` / `NewFatalError`。`NewTimeoutError`、`NewDeadLetterError` 更适合 runtime middleware 或治理层使用。

默认 worker runtime pipeline 顺序：

```text
Recover -> Tracing -> Metrics -> Logging -> Concurrency -> Lock -> Retry -> DeadLetter -> Business
```

其中 Metrics、Concurrency、Lock、DeadLetter 没有注入具体实现时会跳过；RateLimit 需要在任务注册处显式挂载；Retry 默认复用 worker 的 retry delay 策略。driver 只负责 transport：它只读取 `queue.RateLimited(...)` 或 `queue.RuntimeError.RetryIn` 给出的重试延迟，不再承载任务类型级 retry governance。Timeout middleware 只负责发出 context timeout signal；sandbox、docker、cmd 等资源型 runtime 必须通过自己的 resource cleanup 完成真正中断。

## Operations API

Admin API 和 `sdkitgo queue` command 统一走 `queue.OperationsRuntime`：

```go
operations := queue.Runtime(ctx).Operations()

status, err := operations.RuntimeStatus(ctx)
metrics, err := operations.Metrics(ctx)
tasks, err := operations.ListFailedTasks(ctx, queue.TaskQuery{Queue: queue.DefaultQueueName})
cleaned, err := operations.CleanTasks(ctx, queue.TaskQuery{Queue: queue.DefaultQueueName, State: queue.StateArchived})
err = operations.DrainQueue(ctx, queue.DefaultQueueName)
```

对外状态统一为：

```txt
running
paused
draining
stopped
failed
```

`DrainQueue` 默认会 pause 队列并在 operations metadata 中标记 draining；driver 如果实现 `queue.QueueDrainer`，会优先使用原生 drain。`Metrics` 从 `QueueInfo` 聚合 pending、active、scheduled、retry、archived、succeeded、failed、canceled、processed、failed_all。

## 链路追踪

投递和消费队列任务时会自动创建 OpenTelemetry span：

- `producer::<task_type>`：投递任务。
- `consumer::<task_type>`：队列消费者领取并执行任务。
- `handler::<task_type>`：worker 业务 handler 执行入口。

`core/queue` 使用 Asynq task headers 透传 `traceparent`、`tracestate`、`baggage`、`X-Track-ID`、`X-Request-ID`，不会改写业务 payload。

driver 和 worker 侧统一通过 `core/queue` correlation helper 读取 `track_id/request_id/trace_id/span_id`，业务代码不需要解析 headers。

在 Jaeger span attributes 中可以看到：

```json
{
  "trace_id": "...",
  "span_id": "...",
  "track_id": "...",
  "request_id": "...",
  "traceparent": "..."
}
```

任务 ID、任务类型和队列名使用 OpenTelemetry `messaging.message.id`、`messaging.message.type`、`messaging.destination.name` 语义字段记录。queue span 不额外写入 `sd.track_id`、`sd.request_id`、`task_id`、`task_type`、`queue` 这些自定义字段，避免 attributes 重复。

worker handler 的 `ctx` 会带 `logger.ContextFields` 可识别的 `task_id/queue/type`，业务日志可以直接使用 `logger.WithContext(ctx, log)`。

App 服务入口可以用 API demo 直接投递 worker 示例任务：

```http
POST /api/queue/mechanism/demo
Content-Type: application/json
X-Track-ID: demo-track
X-Request-ID: demo-request

{
  "scenario": "retry_fast",
  "message": "retry demo"
}
```

`scenario` 支持：

| scenario | 行为 |
|----------|------|
| `trace_probe` | event 中执行 PostgreSQL 查询和 Redis `INCR`，用于验证跨组件 trace |
| `retry` | event 返回普通错误，按 worker 重试策略重试 |
| `retry_fast` | event 返回普通错误，worker 使用 200ms 快速重试 |
| `rate_limit` | event 返回 `queue.RateLimited`，失败日志写入 `rate_limited=true` |

响应会返回 `trace_id`、`track_id`、`request_id`、`task_id`、`queue` 和 `type`，可直接用 `trace_id` 在 Jaeger 中查询 HTTP -> queue producer -> worker consumer -> event handler 链路。

## 多进程和幂等

多个 worker 进程可以同时消费同一个 Redis 队列。`core/queue` 底层由 asynq 负责从 Redis 原子领取任务，正常情况下同一个任务不会被多个进程同时处理。

需要注意的是，`concurrency` 是单个 worker 进程的并发数，不是全局并发数。例如配置 `concurrency: 10` 时，启动 3 个 worker 进程，最多可能同时处理 30 个任务。

队列语义按“至少一次执行”设计。任务失败重试、worker 崩溃恢复、超时重新投递、人工重投时，同一个业务动作可能被再次执行。关键业务 handler 必须保证重复执行也不会产生重复副作用。

### 防重复入队

如果同一个业务任务短时间内只需要保留一个，可以使用 `queue.Unique(...)`：

```go
_, err := queue.Enqueue(ctx,
    queue.NewTask(taskdef.TypeUserSync, payload),
    queue.Queue(queue.DefaultQueueName),
    queue.MaxRetry(3),
    queue.Timeout(30*time.Second),
    queue.Unique(5*time.Minute),
)
```

`queue.Unique(ttl)` 按任务 type、payload 和 queue 去重，适合短时间合并重复请求。

如果业务动作天然有唯一 ID，优先使用固定 `TaskID`：

```go
taskID := fmt.Sprintf("order_paid:%d", orderID)

_, err := queue.Enqueue(ctx,
    queue.NewTask("order:paid", payload),
    queue.Queue(queue.DefaultQueueName),
    queue.TaskID(taskID),
    queue.MaxRetry(5),
)
```

`TaskID` 适合订单支付、账单结算、回调处理这类明确只能存在一条任务的场景。

`Unique` 和 `TaskID` 只能减少重复入队，不能作为 handler 幂等保证。已经入队的任务仍可能因为失败重试而再次执行。

### 数据库唯一键

发送通知、发放奖励、创建外部资源这类有副作用的任务，推荐先写入一条带业务唯一键的记录，再执行副作用：

```sql
CREATE UNIQUE INDEX uniq_notification_biz_key
ON notifications (biz_key);
```

```go
func HandleSendNotification(ctx context.Context, msg *queue.Message) error {
    payload, err := queue.DecodePayload[taskdef.SendNotificationPayload](msg)
    if err != nil {
        return err
    }

    bizKey := fmt.Sprintf("send_notification:%d:%s", payload.UserID, payload.Title)

    err = db.WithContext(ctx).Create(&Notification{
        BizKey:  bizKey,
        UserID:  payload.UserID,
        Title:   payload.Title,
        Content: payload.Content,
        Status:  "pending",
    }).Error
    if isDuplicateKey(err) {
        return nil
    }
    if err != nil {
        return err
    }

    if err := sender.Send(ctx, payload.UserID, payload.Title, payload.Content); err != nil {
        return err
    }

    return db.WithContext(ctx).
        Model(&Notification{}).
        Where("biz_key = ?", bizKey).
        Update("status", "sent").Error
}
```

重复执行时，第二次插入会命中唯一键，handler 返回 `nil`，避免队列继续重试，也避免重复发送。

### 状态机

订单、工单、审核流这类有明确状态流转的任务，推荐用条件更新保证只推进一次：

```go
result := db.WithContext(ctx).
    Model(&Order{}).
    Where("id = ? AND status = ?", payload.OrderID, "paid").
    Updates(map[string]any{
        "status": "shipping",
    })
if result.Error != nil {
    return result.Error
}
if result.RowsAffected == 0 {
    return nil
}

return shipOrder(ctx, payload.OrderID)
```

只有 `paid` 状态能推进到 `shipping`。如果任务重复执行，状态已经变化，`RowsAffected == 0` 时直接返回成功。

### 分布式锁

如果同一资源同一时刻只能有一个任务运行，可以在 handler 内加 Redis 锁。锁用于降低并发冲突，最终正确性仍应落到唯一键或状态机上：

```go
lockKey := fmt.Sprintf("lock:user_sync:%d", payload.UserID)

ok, err := redis.SetNX(ctx, lockKey, msg.ID, 2*time.Minute).Result()
if err != nil {
    return err
}
if !ok {
    return nil
}
defer redis.Del(ctx, lockKey)

return syncUser(ctx, payload.UserID)
```

推荐顺序：

1. 有明确业务唯一 ID 时，投递使用 `queue.TaskID(...)`。
2. 只需要短时间去重时，投递使用 `queue.Unique(...)`。
3. 有副作用的 handler 使用数据库唯一键或状态机保证幂等。
4. 同一资源并发冲突较高时，再补充分布式锁。

## 重试和限流

普通错误直接返回：

```go
return err
```

任务投递时指定最大重试次数和单次执行超时：

```go
_, err := queue.Enqueue(ctx,
    taskdef.NewUserSyncTask(1001, "admin", false),
    queue.Queue(queue.DefaultQueueName),
    queue.MaxRetry(3),
    queue.Timeout(30*time.Second),
)
```

worker 侧可通过 `RetryStage` 按任务类型定制重试间隔：

```go
func retryDelay(retryCount int, err error, msg *queue.Message) time.Duration {
    if msg != nil && msg.Type == taskdef.TypeUserSync {
        return time.Duration(retryCount) * time.Minute
    }
    return 2 * time.Minute
}
```

该策略应挂在 runtime middleware，例如默认 worker pipeline 中的 `workermiddleware.Retry()`。driver 不再通过 `RuntimeKernelConfig` 接收任务类型级 retry delay。

需要按指定时间重试时，返回 `queue.RateLimited(...)`：

```go
if remoteLimited {
    return queue.RateLimited(2*time.Minute, err)
}
```

`RateLimitError` 会使用 `RetryIn` 作为下次重试延迟，并且不计入失败统计。

runtime error 可用于显式表达治理语义：

```go
return queue.RetryableAfter(30*time.Second, err)
return queue.NewFatalError(err)
return queue.NewIgnoredError(err)
```

`RetryableAfter` 会把下一次重试延迟传给 driver；`NewFatalError` 可触发 DeadLetter；`NewIgnoredError` 默认不计入失败。

完整 handler 示例：

```go
func HandleUserSync(_ context.Context, msg *queue.Message) error {
    payload, err := queue.DecodePayload[taskdef.UserSyncPayload](msg)
    if err != nil {
        return err
    }

    switch payload.Source {
    case "retry":
        return errors.New("user sync example retry error")
    case "rate_limit":
        return queue.RateLimited(2*time.Minute, errors.New("remote service rate limited"))
    }

    return nil
}
```

不希望重试的错误不要在业务层直接依赖 Asynq 的 `SkipRetry`，使用 `queue.NewIgnoredError(err)` 表达 provider-agnostic 的忽略语义。

## 失败接收

队列任务执行失败时，`core/queue` 会调用 `FailureHandler`。标准失败日志只打印任务 ID、队列、类型、重试信息、trace/request 字段和错误原因，不打印 payload：

```go
failureHandler := queue.LogFailureHandler("Worker队列任务失败")
```

`queue.Failure` 字段：

| 字段 | 说明 |
|------|------|
| `TaskID` | 任务 ID |
| `Queue` | 失败所在队列 |
| `Type` | 任务类型 |
| `Payload` | 原始 payload |
| `Err` | handler 返回的错误 |
| `RetryCount` | 当前重试次数 |
| `MaxRetry` | 最大重试次数 |
| `RateLimited` | 是否为 `queue.RateLimited(...)` |
| `Headers` | Asynq task headers，包含 `X-Track-ID`、`X-Request-ID`、`traceparent` 等链路字段 |

### 批量入库

`core/queue` 只定义通用接口，不绑定具体表：

```go
type FailureWriter interface {
    WriteBatch(ctx context.Context, failures []*Failure) error
}
```

项目内示例 writer：

- `worker/bootstrap/failure_writer.go`
- `models.SystemQueueFailureLog`

项目自己的 migrate 命令需要显式创建失败日志分区父表 `system_queue_failure_log`，按 `created_at` 做 PostgreSQL `RANGE` 月分区，并预创建当前月和下月分区。`payload` 使用 `jsonb`，便于后续按原 payload 重新投递任务。生产环境如果通过独立迁移流程建表，需要同步 `models.SystemQueueFailureLog` 对应结构；bootstrap 不再隐式执行 `models.AutoMigrate()`。

表内保留 `task_id`、`queue`、`type`、`payload`、`retry_count`、`max_retry`、`rate_limited`、`track_id`、`request_id`、`trace_id`、`span_id` 和 `error`，用于后台排查、删除队列内已有失败任务，以及按原任务内容重新投递。

### 固定 task_id / 唯一任务重投

asynq 中固定 `TaskID` 或唯一任务失败后，已有任务仍在 retry / archived 等状态里。失败代表任务没有完整处理完，原 `task_id` 仍会占用，再用相同 `TaskID` 入队会冲突。

后台重试这类任务时，先用日志表里的 `queue + task_id` 删除 asynq 内已有任务，再重新入队：

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

`OperationsRuntime.DeleteTask` 底层会删除 pending、scheduled、retry 或 archived 状态的 asynq 任务；active 状态不能删除，应等待任务结束后再操作。

`queue.FailureLogger` 行为和 `core/accesslog.Logger` 类似：

- `Push` 非阻塞，缓冲满时丢弃并输出标准日志
- 达到 `BatchSize` 自动写入
- 到达 `FlushInterval` 自动写入
- context 取消后会 flush 剩余日志
- `PushContext` 会保留 context values 里的 trace/request/track 信息，但不会继承任务 context 的取消信号；异步落库不会因为 asynq 任务结束而拿到 canceled context

## 队列统计

```bash
go run ./cmd/sdkitgo queue stats
```

输出各队列的 pending、active、scheduled、completed、archived、retry、processed、failed 和 latency。

## 示例位置

当前项目内示例：

- `worker/taskdef/user_sync.go`：任务类型、payload、构造函数、投递 helper
- `worker/taskdef/file_upload.go`：文件上传任务 payload，支持 `source_url` / `temp_file_path` / `source_path` / `content`
- `worker/event/user_sync.go`：worker event handler
- `worker/event/file_upload.go`：文件上传事件处理，负责流式上传和临时文件清理
- `worker/registry.go`：event 和 middleware 挂载
- `worker/bootstrap/runtime.go`：队列运行时接线
- `worker/bootstrap/failure_writer.go`：失败日志批量入库 writer
- `worker/bootstrap/retry.go`：重试间隔接线
- `worker/tests/worker_queue_demo_test.go`：真实 DB/Redis demo，覆盖重试、限流、唯一任务、删除后重投和 tracing
- `models/system_queue_failure_log.go`：失败日志分区表模型
- `core/queue/facade/producer`：只创建 producer client
- `core/queue/facade/operations`：创建 producer client、manager 和 `RuntimeInstance`
- `app/admin/handler/notification.go`：HTTP handler 投递任务

## 约定

- 业务代码使用 `queue.Task`，不要返回 `*asynq.Task`。
- 业务 handler 使用 `queue.Message`，不要接收 `*asynq.Task`。
- 任务 payload 使用结构体并带 JSON tag。
- 投递入口使用 `queue.Enqueue(ctx, task, opts...)` 或显式注入的 `queue.Client`。
- 业务代码不调用 Asynq 专用构造函数；需要扩展 driver 时放到 `core/queue/driver/<name>` 并注册为 `queue.RunnerDriver`。
- worker 业务注册放在 `worker`，`core/queue` 不注册具体业务。
- scheduler/cron 只负责触发，不属于 `core/queue`。
## 任务管理

`OperationsRuntime` 统一提供队列和任务管理：

```go
operations := queue.Runtime(ctx).Operations()
queues, err := operations.ListQueues(ctx)
task, err := operations.GetTask(ctx, "critical", "task-id")
err = operations.RetryTask(ctx, "critical", "task-id")
err = operations.ArchiveTask(ctx, "critical", "task-id")
err = operations.DeleteTask(ctx, "critical", "task-id")
err = operations.PauseQueue(ctx, "low")
err = operations.ResumeQueue(ctx, "low")
```

查询任务：

```go
tasks, err := operations.ListTasks(ctx, queue.TaskQuery{
    Queue: "default",
    State: queue.StatePending,
    Limit: 50,
})
```

## API 管理

Admin 服务自己注册队列 HTTP 路由，实际路径为 `/admin/v1/queue/*`。路由、鉴权、response、validator 和 middleware 都属于 Admin Runtime Host；handler 只调用 `queue.OperationsRuntime` 和显式注入的 `queue.Client`。

接口列表：

| 方法 | 路径 | 参数 |
|------|------|------|
| GET | `/admin/v1/queue/queues` | 无 |
| GET | `/admin/v1/queue/queue` | query: `queue` |
| GET | `/admin/v1/queue/tasks` | query: `queue`、`state`、`type`、`task_id`、`limit`、`offset`、`cursor` |
| GET | `/admin/v1/queue/task` | query: `queue`、`id` |
| POST | `/admin/v1/queue/tasks` | JSON body: 入队请求 |
| POST | `/admin/v1/queue/task/retry` | JSON body: `queue`、`id` |
| POST | `/admin/v1/queue/task/archive` | JSON body: `queue`、`id` |
| POST | `/admin/v1/queue/task/cancel` | JSON body: `queue`、`id` |
| POST | `/admin/v1/queue/task/delete` | JSON body: `queue`、`id` |
| POST | `/admin/v1/queue/queue/pause` | JSON body: `queue` |
| POST | `/admin/v1/queue/queue/resume` | JSON body: `queue` |

约束：

- GET 使用 query，不使用 URL path 参数。
- POST 使用 JSON body，不使用 URL path 参数。
- 删除任务使用 POST action，不使用 `DELETE` 方法。
- API 会把 `queue.ErrTaskDuplicated` 映射为 `err_code=4091`，调用方可以明确感知重复投递。

手动入队请求：

```json
{
  "type": "video.process",
  "queue": "heavy",
  "payload": {"video_id": 123},
  "max_retry": 3,
  "timeout_seconds": 600,
  "unique_seconds": 300
}
```

任务动作请求使用 JSON body，不使用 URL 参数，也不使用 DELETE 方法：

```json
{
  "queue": "critical",
  "id": "task-id"
}
```

队列动作请求：

```json
{
  "queue": "low"
}
```

写操作应由上层后台路由接入鉴权和审计；`core/queue` 已预留 `AuditLogger` 接口。

## Command 管理

`worker/command/queue` 归 worker 服务持有，注册 `sdkitgo queue` Cobra command，并通过显式 `queue.Client` 或 `runtime.Operations()` 使用队列能力。根 `command` 包只聚合注册，不承载 queue 命令逻辑。Command host 负责参数绑定、输出格式和 outbox 子命令，`core/queue` 不暴露 Cobra 适配层。

```bash
sdkitgo queue status
sdkitgo queue runtime
sdkitgo queue metrics
sdkitgo queue queues
sdkitgo queue tasks --queue=default --state=pending
sdkitgo queue task --queue=default --id=xxx
sdkitgo queue enqueue video.process --queue=heavy --payload='{"video_id":123}' --max-retry=3 --timeout=10m --unique=5m
sdkitgo queue retry --queue=default --id=xxx
sdkitgo queue archive --queue=default --id=xxx
sdkitgo queue cancel --queue=default --id=xxx
sdkitgo queue delete --queue=default --id=xxx
sdkitgo queue clean --queue=default --state=archived
sdkitgo queue pause --queue=low
sdkitgo queue resume --queue=low
sdkitgo queue drain --queue=low
sdkitgo queue outbox flush --limit=100
sdkitgo queue outbox poll --limit=100 --interval=5s
```

不支持的能力会返回明确错误，例如：

```txt
queue driver asynq does not support capability: priority
```

## 业务锁、幂等、限流

Redis 版本：

```go
import rediscontrol "github.com/huwenlong92/sdkit/pkg/queue/control/redis"

locker := rediscontrol.NewLocker(redisClient, "queue:lock:")
unlock, ok, err := locker.Lock(ctx, "video:123", 5*time.Minute)
if err != nil {
    return err
}
if !ok {
    return queue.ErrLockNotAcquired
}
defer unlock(context.Background())

idem := rediscontrol.NewIdempotency(redisClient, "queue:done:")
done, err := idem.Done(ctx, "video:123")
if err != nil || done {
    return err
}

limiter := rediscontrol.NewRateLimiter(redisClient, "queue:rate:")
allowed, retryIn, err := limiter.Allow(ctx, "tenant:1", 100, time.Minute)
if !allowed {
    return queue.RateLimited(retryIn, err)
}
```

测试或单进程场景可以使用 `pkg/queue/control/memory`：

```go
import memorycontrol "github.com/huwenlong92/sdkit/pkg/queue/control/memory"

locker := memorycontrol.NewLocker()
idem := memorycontrol.NewIdempotency()
limiter := memorycontrol.NewRateLimiter()
```

Worker 需要消费侧限流时，在任务注册处显式挂 `workermiddleware.RateLimit(limit, window)`，例如 `queue.StageMiddleware(queue.RateLimitStage, workermiddleware.RateLimit(10, time.Minute))`。`limit` 表示窗口内允许执行的最大任务数，`window` 表示统计窗口；当前 worker helper 默认按 `handler:<task_type>` 作为限流 key。`worker.queue.rate_limit.enabled=true` 且 runtime 初始化了 `RateLimiter` 后才会生效。

限流策略放在 `worker/middleware` 内定义。`core/queue/runtime/middleware` 提供 `RateLimit` 和 `RateLimitKeyFunc` 扩展点；`core/queue` 保留 `ContextChain`。`ContextChain` 是 `c.Next()` 风格，middleware 可以通过 `err := c.Next()` 接收后续 middleware 或 handler 的错误。

## 常见问题

- `WithPriority` 报 `ErrCapabilityUnsupported`：asynq 不支持单任务 priority，请用队列权重配置。
- `WithRateLimitKey` 报 `ErrCapabilityUnsupported`：限流在业务治理层处理，使用 `RateLimiter` 后再入队或消费。
- `core/queue` 不提供 `Scheduler`：定时调度统一由 `crontab` 负责，触发后再调用 `queue.Enqueue`。
- worker 收到退出信号后 `Run(ctx)` 正常返回 `nil`，不会把 `context canceled` 当作服务错误。
