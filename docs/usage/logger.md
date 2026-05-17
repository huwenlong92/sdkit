# Logger 日志使用

`core/logger` 提供全局日志配置和各组件日志适配，统一使用 Zap + lumberjack 分割。当前不切 slog。

## Runtime Capability 引入

`core/logger` 保留日志实现和业务 API；Runtime 接入门面放在 `core/logger/facade`。如果只做能力注册，可以把 facade import 成 `logger`：

```go
import logger "github.com/huwenlong92/sdkit/core/logger/facade"

app := runtime.New()
app.RegisterCapabilities(
    logger.Use(logger.WithConfig(logger.Config{
        Name:   "admin",
        Level:  "info",
        Mode:   "dev",
        Format: "console",
    })),
)
```

bootstrap 会在主 Runtime 中先加载配置，再通过 `core/logger/facade` 的 `logger.Use(logger.WithConfigLoader(...))` 注册公共 logger 能力。业务日志仍然直接使用根包 `github.com/huwenlong92/sdkit/core/logger`。独立命令仍可使用简写：

```go
logger.Init("admin", "info", "dev")
```

## 文件结构

`core/logger` 按实现和 Runtime facade 分层：

```text
core/logger/
  logger.go   # Config、Zap client、默认 logger、Named、Writer、Asynq、Cron
  context.go  # context 链路字段
  facade/
    config.go
    client.go
    use.go
    default.go
```

## 目录结构

默认写入 `logs/`，按组件分目录：

```text
logs/
  admin/admin.log
  api/api.log
  worker/worker.log
  sse/sse.log
  worker-eventbus/worker-eventbus.log
  worker-realtime-demo/worker-realtime-demo.log
  gorm/runtime.log
  asynq/asynq.log
  crontab/crontab.log
```

dev 模式同时输出 stdout，非 dev 模式同时输出 stderr。

## 业务日志

```go
logger.L.Info("user created", zap.Int64("user_id", id))
```

创建独立组件 logger：

```go
jobLogger := logger.Named("job")
jobLogger.Info("job started")
```

HTTP 请求内优先透传 `context.Context`，底层 pgx、Redis 等 adapter 会从 context 中读取 `track_id`、`trace_id`、`span_id`、`request_id`：

```go
log := logger.WithContext(ctx, logger.Named("worker"))
log.Info("task handled", zap.String("type", taskType))
```

业务 service 可以把基础 logger 放在结构体里复用，但不要保存带 `ctx` 的 logger。每次请求、队列任务或 crontab run 的 `ctx` 都不同，进入具体方法后按需 attach 当前 context：

```go
type OrderService struct {
    log *zap.Logger
}

func NewOrderService() *OrderService {
    return &OrderService{
        log: logger.Named("order"),
    }
}

func (s *OrderService) Create(ctx context.Context, req CreateOrderRequest) error {
    log := logger.WithContext(ctx, s.log)

    log.Info("create order start")
    if err := s.repo.Create(ctx, req); err != nil {
        log.Error("create order failed", zap.Error(err))
        return err
    }
    log.Info("create order success")
    return nil
}
```

如果只打一条日志，可以直接 inline：

```go
logger.WithContext(ctx, s.log).Info("create order success")
```

需要写入标准日志字段时使用 helper，不直接 `context.WithValue` 写 string key：

```go
ctx = logger.WithField(ctx, logger.TaskIDKey, taskID)
```

仓库测试会扫描新增代码，禁止对标准链路字段直接使用 `context.WithValue`。业务代码应通过 `logger.WithField`、`requestid.WithRequestID`、`tracking.WithTrackID` 等 helper 写入。

业务 error 日志必须带 `zap.Error(err)`：

```go
if err != nil {
    logger.L.Error("create user failed", zap.Error(err))
    return err
}
```

不要在业务中直接使用 `zap.L()`、`zap.S()`、`log.Println` 或 `fmt.Println` 记录运行日志。CLI 命令的结果输出可以保留 `fmt.Println`。

## Tracking 和 Request ID

HTTP 服务通过中间件生成或透传链路字段：

- `core/tracking`：`X-Track-ID` -> `track_id`
- `core/tracing`：`traceparent` / `tracestate` -> `trace_id` / `span_id`
- `core/requestid`：`X-Request-ID` -> `request_id`
- `core/queue`：worker 任务 context -> `task_id` / `queue` / `type`
- `core/crontab`：定时任务 run context -> `run_id` / `job_id`

推荐注册顺序：

```text
Recovery -> Tracking -> Tracing -> RequestID -> CORS -> AccessLog -> Auth -> Handler
```

`core/logger.ContextFields(ctx)` 和 `logger.WithContext(ctx, log)` 只读取 typed context key。Handler 中调用数据库、Redis、队列等基础设施时应透传 `c.Request.Context()`。队列 handler 和 crontab handler 收到的 context 已带任务或 run 字段，业务日志可直接使用 `logger.WithContext`。

## Database

GORM 和 pgx 日志由 `core/database` 初始化时统一接入，不在业务代码里直接创建数据库 logger。

```go
database.Init(appCfg.DB, appCfg.App.Mode)
```

GORM SQL 日志写入 `logs/gorm/runtime.log`，pgx 日志写入 `logs/pgx/pgx.log`。

pgx adapter 使用 Zap 输出，并会追加 context 中的 `track_id`、`trace_id`、`span_id`、`request_id`。

## Redis

Redis hook 由 `core/redis` 初始化时接入：

```go
redis.Init(ctx, cfg, logger.L)
```

Redis 日志记录命令名、pipeline 命令数、耗时和错误。错误日志会带 `zap.Error(err)`；如果 context 中存在 `track_id`、`trace_id`、`span_id`、`request_id`，会一起输出。

## Asynq

```go
asynq.NewServer(redisOpt, asynq.Config{
    Concurrency: 10,
    Logger:      logger.Asynq("asynq"),
})
```

写入 `logs/asynq/asynq.log`。

## Crontab

```go
cron.New(
    cron.WithSeconds(),
    cron.WithLogger(logger.Cron("crontab")),
)
```

写入 `logs/crontab/crontab.log`。

## 日志分割

默认策略：

| 参数 | 默认值 |
|------|------|
| `MaxSize` | 10 MB |
| `MaxBackups` | 5 |
| `MaxAge` | 30 天 |
| `Compress` | false |

自定义：

```yaml
log:
  level: info
  format: json
  root_dir: logs
  rotation:
    max_size: 50
    max_backups: 10
    max_age: 30
    compress: true
```

`bootstrap` 会把这些配置传给 `core/logger`。`logger.Named("xxx")` 创建的组件日志会复用同一套 level、format、root_dir 和 rotation 配置。

## SSE / Worker Demo

SSE 相关日志已经按组件拆分：

| 组件 | 文件 |
|------|------|
| SSE 服务、SSE 连接、EventBus 订阅 | `logs/sse/sse.log` |
| worker EventBus 初始化 | `logs/worker-eventbus/worker-eventbus.log` |
| worker realtime demo 进度推送 | `logs/worker-realtime-demo/worker-realtime-demo.log` |

这些日志都会带 `node_name`，多进程部署时可用于区分节点。

## 敏感信息

访问日志不会记录以下敏感 header：

- `Authorization`
- `Cookie`
- `Set-Cookie`
- `X-Api-Key`
- `X-Auth-Token`

JSON 请求体中的敏感字段会写成 `(redacted)`，字段名包含以下关键词都会脱敏：

- `authorization`
- `cookie`
- `password`
- `token`
- `secret`

业务日志不要主动输出密码、token、secret、cookie、authorization 等敏感值。当前没有全局 Zap Core 拦截器，传给 Zap 的普通业务字段不会被自动改写。
