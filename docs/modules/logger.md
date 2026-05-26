# Logger 模块方案

## 作用

`core/logger` 统一项目日志入口，负责初始化 Zap、日志分割、组件日志适配和链路字段复用。

## Runtime Capability

`core/logger` 是日志实现包，Runtime Capability 接入层统一放在 `core/logger/facade`：

```text
core/logger/
  logger.go
  context.go
  facade/
    config.go
    client.go
    use.go
    default.go
```

启动时由主 Runtime 注册：

```go
import loggercap "github.com/huwenlong92/sdkit/core/logger/facade"

runtimeApp.RegisterCapabilities(
    loggercap.Use(loggercap.WithConfig(appCfg.Log.LoggerConfig("admin", appCfg.App.Mode))),
)
```

bootstrap 使用 `loggercap.WithConfigLoader(...)`，确保配置能力先初始化，再由 `loggercap.Use` 读取最终配置：

```go
loggercap.Use(loggercap.WithConfigLoader(func(app *runtime.App) (loggercap.Config, error) {
    return cfg.Log.LoggerConfig("admin", cfg.App.Mode), nil
}))
```

也可以在独立命令中绕过 Runtime 使用：

```go
logger.Init("queue", "info", "dev")
```

`loggercap.Use()` 默认按框架底座能力处理，metadata `Internal=true`。需要在启动信息或 CLI 中对外展示 logger capability 时，调用方必须显式传入 `loggercap.WithExternal()`。未传 `WithConfig` / `WithConfigLoader` / `WithLogger` 时，facade 会复用已存在的 `logger.L`；如果还没有默认 logger，则使用零值配置初始化，底层 `logger.Configure` 会补齐默认日志配置。

## 配置项

| 字段 | 说明 |
|------|------|
| `Name` | 服务或组件名，决定默认日志目录和文件名 |
| `Level` | `debug` / `info` / `warn` / `error` |
| `Mode` | `dev` 时控制台优先输出 stdout，否则输出 stderr |
| `Format` | `console` / `json` |
| `RootDir` | 日志根目录，默认 `logs` |
| `Rotation` | 日志分割配置，默认按大小分割，可配置为按日期分割 |

配置文件示例：

```yaml
log:
  level: debug
  format: console
  root_dir: logs
  rotation:
    mode: size
    max_size: 10
    max_backups: 5
    max_age: 30
    compress: false
```

`rotation.mode` 支持：

- `size`：默认模式，使用 lumberjack 按文件大小分割。
- `daily`：按日期分割，文件名为 `<filename-base>-YYYY-MM-DD.log`，例如 `logs/api/api-2026-05-17.log`。该模式按 `max_age` 清理旧日期文件，`max_size`、`max_backups` 和 `compress` 当前仅对 `size` 模式生效。

## 对外接口

```go
logger.L
logger.Configure(cfg)
logger.Init(name, level, mode)
logger.Named(name)
logger.Writer(name, filename)
logger.WriteSyncer(name, filename)
logger.ContextFields(ctx)
logger.Field(ctx, key)
logger.WithField(ctx, key, value)
logger.WithContext(ctx, log)
logger.Ctx(ctx)
logger.Sync()
```

Runtime facade 对外接口：

```go
loggercap.Use(opts...)
loggercap.WithConfig(cfg)
loggercap.WithConfigLoader(loader)
loggercap.WithLogger(log)
loggercap.WithExternal()
```

## 中间件

Logger 模块本身不提供 Gin 中间件。HTTP 链路字段由以下模块负责：

- `core/tracking`：生成/透传 `X-Track-ID`
- `core/tracing`：生成/透传 OpenTelemetry `trace_id/span_id`
- `core/requestid`：生成/透传 `X-Request-ID`
- `core/accesslog`：采集访问日志并记录 tracking/request 字段

推荐注册顺序：

```txt
Recovery -> Tracking -> Tracing -> RequestID -> CORS -> AccessLog -> Auth -> Handler
```

## Hook

- Asynq：`logger.Asynq("asynq")`
- Cron：`logger.Cron("crontab")`
- SSE：`logger.Named("sse")`
- Worker EventBus：`logger.Named("worker-eventbus")`
- Worker Realtime Demo：`logger.Named("worker-realtime-demo")`
- GORM：`core/database` 使用 `logger.Writer("gorm", "runtime.log")`
- pgx：`core/database` 使用 `logger.Named("pgx")`，并追加 context 中的 `track_id/trace_id/span_id/request_id`
- Redis：`core/redis` hook 记录命令名、耗时、错误和 context 链路字段
- Queue：worker context 会补充 `task_id/queue/type`
- Crontab：run context 会补充 `run_id/job_id`

## 使用示例

```go
logger.L.Info("user created",
    zap.Int64("user_id", id),
)

logger.WithContext(ctx, logger.Named("pgx")).Info("query done")
```

业务 service 推荐复用基础 logger，并在每次请求、队列任务或 crontab run 中通过当前 `ctx` 派生本次日志对象：

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
    return nil
}
```

不要把 `logger.WithContext(ctx, log)` 返回的 logger 存到全局或长期存活的 struct 中；它只适合当前调用链。

业务日志必须处理 error：

```go
if err != nil {
    logger.L.Error("create user failed", zap.Error(err))
    return err
}
```

## 注意事项

- 使用 Zap，不切 slog。
- 不在业务中直接调用 `zap.L()` 或 `zap.S()`。
- 日志必须优先带 `track_id`；启用 tracing 后同时带 `trace_id/span_id`，HTTP 请求内通过 context 透传。
- `ContextFields` 支持 `track_id`、`trace_id`、`span_id`、`request_id`、`task_id`、`queue`、`type`、`run_id`、`job_id`；新链路字段应优先走 context，而不是在每层重复拼接。
- 标准日志字段写入 context 时使用 `logger.WithField`，读取使用 `logger.Field` 或 `ContextFields`；context 内部只使用 typed key。
- `core/logger/tests` 包含 guard 测试，会扫描仓库并阻止新增标准链路字段的 direct `context.WithValue` 写法。
- error 日志必须带 `zap.Error(err)`。
- 禁止输出密码、token、secret、cookie、authorization 等敏感信息。

## 已知限制

- 没有全局 Zap Core 脱敏拦截器，业务侧仍需避免传入敏感字段。
- GORM 日志复用 writer，但不是 Zap encoder。
- accesslog 会对 JSON、form-urlencoded 和 multipart/form-data 中的敏感字段做脱敏，并支持按 middleware 实例覆盖或追加敏感字段和 header。

## 更新记录

- 2026-05-26：Logger runtime facade 默认作为 internal 底座能力，新增 `WithExternal()` 显式对外展示；默认依赖收敛到 `defaultUseOptions()`。
- 2026-05-10：确认 Zap 为统一实现；补充 context 字段工具、pgx context 字段透传、accesslog trace/request 字段和 JSON body 脱敏说明。
- 2026-05-12：`ContextFields` 增加 `track_id`，HTTP tracking 模块切换为 `core/tracking` / `X-Track-ID`。
- 2026-05-12：`ContextFields` 支持从 OpenTelemetry span context 自动追加 `trace_id/span_id`。
- 2026-05-13：`ContextFields` 补充队列任务和 crontab run 字段：`task_id/queue/type/run_id/job_id`。
- 2026-05-13：新增 typed context key 层，`WithField` / `Field` 只使用 typed key。
- 2026-05-21：同步 accesslog form-urlencoded、multipart/form-data 字段脱敏和自定义脱敏规则说明，支持覆盖为空用于测试调试。
- 2026-05-13：补充业务 service logger 使用约定：基础 logger 可复用，带 context 的 logger 只在当前调用链内临时生成。
- 2026-05-13：新增 direct `context.WithValue` guard 测试，禁止标准链路字段绕过 helper 写入 context。
- 2026-05-15：Runtime Capability 接入层迁移到 `core/logger/facade`，按 `config.go/client.go/use.go/default.go` 组织，根包保留日志实现和业务 API。
