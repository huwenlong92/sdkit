# Tracing 使用文档

`core/tracing` 用于 OpenTelemetry 链路追踪。业务追踪 ID 仍使用 `core/tracking` 的 `X-Track-ID`。

## Runtime Capability 引入

`core/tracing` 保留 tracing 实现和业务 API；Runtime 接入门面放在 `core/tracing/facade`。如果只做能力注册，可以把 facade import 成 `tracing`：

```go
import tracing "github.com/huwenlong92/sdkit/core/tracing/facade"

app := runtime.New()
app.RegisterCapabilities(
    tracing.Use(tracing.WithConfig(tracing.Config{
        Enabled:     true,
        ServiceName: "api",
        Environment: "dev",
        Endpoint:    "127.0.0.1:4317",
        Insecure:    true,
    })),
)
```

bootstrap 会在主 Runtime 中先加载配置，再通过 `core/tracing/facade` 的 `tracing.Use(tracing.WithConfigLoader(...))` 注册公共 tracing 能力。业务 middleware、span、propagation 仍然直接使用根包 `github.com/huwenlong92/sdkit/core/tracing`。

## 启用配置

Tracing 配置放在 `configs/tracing.yaml`，主配置 `configs/config.yaml` 通过 `imports` 引入：

```yaml
imports:
  - tracing.yaml
```

开发环境 Jaeger 已可通过 OTLP gRPC 接入：

```yaml
tracing:
  enabled: true
  service_name: ""
  environment: dev
  endpoint: 192.168.1.126:4317
  insecure: true
  sample_ratio: 1.0
  strict: false
  timeout: 5s
```

访问 Jaeger UI：

```text
http://192.168.1.126:16686
```

`service_name` 留空时使用当前启动服务名，例如 `admin`、`api`、`sse`。

## HTTP 服务

现有 admin/api/sse router 已注册：

```go
r.Use(tracking.Middleware())
r.Use(tracing.Middleware("admin"))
r.Use(requestid.Middleware())
```

完整 HTTP 推荐顺序是 `Recovery -> Tracking -> Tracing -> RequestID -> CORS -> AccessLog -> BBR -> RateLimit -> Auth/Casbin -> Handler`。`Tracking` 必须先于 `Tracing`，让 HTTP root span 带上 `track_id`；`RequestID` 放在 `Tracing` 后面，tracing middleware 会在下游返回后补充 `sd.request_id`。

请求进入后会自动创建 HTTP root span。未初始化 tracing 或 `enabled=false` 时 middleware 走 OpenTelemetry noop provider，不影响请求。

## 手动 span

业务内需要细分耗时时：

```go
ctx, span := tracing.StartSpan(ctx, "risk.check")
defer span.End()
```

可附加属性：

```go
ctx, span := tracing.StartSpan(ctx, "risk.check",
    attribute.String("risk.type", "login"),
)
defer span.End()
```

## 日志字段

透传 request context 后，日志会自动追加：

```go
logger.WithContext(ctx, logger.Named("risk")).Info("risk checked")
```

输出字段包括：

- `track_id`
- `trace_id`
- `span_id`
- `request_id`
- 队列任务 context 中的 `task_id`、`queue`、`type`
- crontab run context 中的 `run_id`、`job_id`

HTTP root span 会写入统一 correlation attributes：

- `trace_id`
- `span_id`
- `track_id`
- `request_id`
- `traceparent`

并额外保留兼容字段：

- `sd.track_id`
- `sd.request_id`

AccessLog 中 `track_id` 和 `trace_id` 是两个独立字段：`track_id` 来自 `X-Track-ID`，`trace_id` 来自 OpenTelemetry span。

## HTTP 调用透传

发起下游 HTTP 请求前注入 header：

```go
req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
if err != nil {
    return err
}
tracing.InjectHTTPHeader(ctx, req.Header)
```

接收下游请求时提取：

```go
ctx = tracing.ExtractHTTPHeader(ctx, req.Header)
```

会透传：

- `traceparent`
- `tracestate`
- `baggage`
- `X-Track-ID`
- `X-Request-ID`

非 HTTP driver 需要使用 map headers 时，统一复用 tracing correlation helper：

```go
headers := tracing.HeadersFromContext(ctx)
ctx = tracing.ContextFromHeaders(ctx, headers)
```

这组 helper 只负责标准链路字段，不承载业务 payload。

队列 driver 应使用 `core/queue` 暴露的队列链路契约，而不是直接调用 tracing helper：

```go
headers := queue.CorrelationHeadersFromContext(ctx)
ctx = queue.ContextFromCorrelationHeaders(ctx, headers)
```

这些 helper 恢复 context 时只写 typed key；读取链路字段必须使用 `tracking.TrackID`、`requestid.FromContext`、`logger.Field` 或对应模块 helper。

## 数据库操作

`core/database` 已在 tracing 启用后自动接入 GORM 和 Pgx：

```go
// GORM 必须透传 ctx。
err := database.DB.WithContext(ctx).Create(&user).Error

// Pgx 必须透传 ctx。
rows, err := database.PGX(ctx).Query(ctx, "SELECT id, username FROM sd_system_user WHERE id = $1", id)
```

在 Jaeger 中可以看到 HTTP root span 下的 `gorm.query`、`gorm.create` 或 Pgx query/batch span。GORM 和 Pgx 数据库 span 会写入 `trace_id`、`span_id`、`track_id`、`request_id`、`traceparent`，方便直接按业务追踪 ID 或请求 ID 过滤数据库操作。

admin 的系统管理 handler、登录 hooks、admin/api Casbin middleware 已按这个规范接入，请求触发数据库查询时会自动挂到当前 HTTP span 下。后续新增 HTTP handler 时也必须继续使用 `WithContext(c.Request.Context())`。

## Queue 和 Redis

`core/queue` 已接入 producer/consumer tracing：

- 投递任务时创建 `producer::<task_type>` span。
- worker 消费时创建 `consumer::<task_type>` span，业务 handler 创建 `handler::<task_type>` span。
- Asynq task headers 会透传 `traceparent`、`tracestate`、`baggage`、`X-Track-ID`、`X-Request-ID`。
- queue span 会写入 `trace_id`、`span_id`、`track_id`、`request_id`、`traceparent`，任务信息使用 `messaging.message.id`、`messaging.message.type`、`messaging.destination.name` 语义字段记录。
- worker 失败日志会从 task headers 解析 `track_id/request_id/trace_id/span_id`，同时日志 context 可带 `task_id/queue/type`。
- Asynq driver、Outbox 和 worker 失败日志统一复用 `core/queue` 的 correlation API，避免各自维护 header 解析规则。
- Outbox 保存任务时会持久化当前请求的 correlation headers；后台 Flush 补偿投递时会先从保存的 headers 恢复 enqueue context，因此 producer span、consumer span 和 handler span 会尽量归到原始请求 trace 下。

`core/redis` 已在 hook 中创建 Redis span：

- 普通命令：`redis.<cmd>`。
- Pipeline：`redis.pipeline`。
- Redis span 会写入 `trace_id`、`span_id`、`track_id`、`request_id`、`traceparent`。

`core/crontab` 已接入 crontab execution span：

- 每次任务执行创建 `crontab.execute` span。
- 本地任务 handler 收到的 `ctx` 已包含 crontab execution span。
- run context 会带 `run_id/job_id/track_id`，run 记录和任务日志事件会落这些字段。
- 调度触发没有上游 context 时，`crontab.execute` 是 root span；手动触发透传请求 context 时，会挂在上游 trace 下。

## EventBus 和 SSE

`core/eventbus` 发布事件时会从 `ctx` 写入 correlation headers，memory、redis、redisstream driver 都必须保留 `Event.Headers`：

- `traceparent` / `tracestate` / `baggage`
- `X-Track-ID`
- `X-Request-ID`

订阅 handler context 会从 `Event.Headers` 恢复 trace、track 和 request 信息。`Event.TraceID` 和 `core/realtime.Event.TraceID` 只表示 OpenTelemetry trace ID，不能写入 `track_id/request_id`。

默认 driver 会为每次 event handler 调用创建 `eventbus.handle <event_name|topic>` consumer span。SSE 服务订阅 eventbus 后转发到本机连接，也落在这个单事件 span 下。

## WebSocket

`app/realtime` WebSocket transport 不经过 Gin middleware，upgrade 入口会自行提取或生成 correlation 字段：

- 从 headers 提取 `traceparent`、`tracestate`、`baggage`、`X-Track-ID`、`X-Request-ID`
- 缺少 `X-Track-ID` 或 `X-Request-ID` 时生成新值，并写回 101 response headers
- 每条业务 message 创建 `websocket.message <action>` span
- 发布到 EventBus 的 realtime event 会保存 correlation headers，跨 gateway 后再恢复 context

WebSocket message payload 中的 `request_id` 是消息级 ID，对应 context 字段 `ws_request_id`，不会覆盖 HTTP upgrade 的 `request_id`。

## 注意事项

- tracing 默认关闭，必须配置 `tracing.enabled=true`。
- Jaeger all-in-one 需要开启 OTLP collector，并开放 `4317`。
- 数据库、Redis、队列、crontab handler 调用必须透传 context。
- 当前阶段 GORM/Pgx、Queue、Redis、Crontab 已自动接入；业务仍必须透传 context 才能串联链路。
- 不要在 `pkg` driver 内重新定义 `trace_id/track_id/request_id` 语义；队列 driver 应复用 `core/queue` correlation API，其他基础设施 driver 再按模块边界复用对应 core helper。
