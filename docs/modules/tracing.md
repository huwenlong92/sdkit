# Tracing 模块方案

## 作用

`core/tracing` 提供 OpenTelemetry 链路追踪能力，负责 trace/span 的生成、传播、日志字段提取和 OTLP 导出；Gin HTTP middleware 位于 `core/gin/tracing`。

职责边界：

- `core/tracking`：业务追踪 ID，字段为 `track_id`，HTTP header 为 `X-Track-ID`。
- `core/tracing`：OpenTelemetry 链路追踪，字段为 `trace_id` / `span_id`。

tracking 和 tracing 职责独立，二者都必须按各自边界使用。

## 统一改造审阅

`tracing / tracking` 的统一改造归档见 [../../plans/framework/tracing_unification.md](../../plans2/framework/tracing_unification.md)，现状审阅见 [../../plans/framework/tracing_review.md](../../plans2/framework/tracing_review.md)。

当前冻结的命名规则：

- `trace_id` 只表示 OpenTelemetry trace ID。
- `span_id` 只表示 OpenTelemetry span ID。
- `track_id` 只表示业务追踪 ID，对应 `X-Track-ID`。
- `request_id` 只表示请求或消息请求 ID，对应 HTTP `X-Request-ID` 或消息 payload 内 request id。

已知边界：

- 业务追踪统一使用 `core/tracking`。
- `core/eventbus` 使用 `Headers` 传播 `traceparent`、`X-Track-ID`、`X-Request-ID`；`core/realtime.Event.TraceID` 只保留 OpenTelemetry trace ID。
- `app/realtime` WebSocket transport 在 upgrade 时提取或生成 `X-Track-ID` / `X-Request-ID`，每条 message 创建独立 span；message payload 内的 `request_id` 使用 `ws_request_id` 语义。

## 配置

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

`service_name` 为空时使用启动服务名。`strict=false` 时 exporter 初始化失败不会让服务 panic；`strict=true` 时初始化错误会返回给 bootstrap。

代码默认仍为关闭：

```go
tracing.DefaultConfig().Enabled == false
```

## Runtime Capability

`core/tracing` 是 tracing 实现包，Runtime Capability 接入层统一放在 `core/tracing/facade`：

```text
core/tracing/
  tracing.go
  config.go
  span.go
  propagator.go
  facade/
    config.go
    client.go
    use.go
    default.go
```

启动时由主 Runtime 注册：

```go
import tracingcap "github.com/huwenlong92/sdkit/core/tracing/facade"

runtimeApp.RegisterCapabilities(
    tracingcap.Use(tracingcap.WithConfig(cfg.Tracing)),
)
```

bootstrap 使用 `tracingcap.WithConfigLoader(...)`，确保配置能力先初始化，再由 `tracingcap.Use` 读取最终配置，并补齐 `service_name` / `environment`。

`tracingcap.Use()` 默认按框架底座能力处理，metadata `Internal=true`。需要在启动信息或 CLI 中对外展示 tracing capability 时，调用方必须显式传入 `tracingcap.WithExternal()`。未传 `WithConfig` / `WithConfigLoader` 时使用 `core/tracing` 的默认配置，默认 `enabled=false`，只设置 propagator，不创建 exporter。

## 初始化

`bootstrap.Init` 会读取 `tracing` 配置，并通过 `core/tracing/facade` 调用根包初始化：

```go
shutdown, err := tracing.Init(ctx, cfg)
```

`Init` 行为：

- `enabled=false` 时只设置 propagator，返回 noop shutdown。
- `enabled=true` 时创建 OTLP gRPC exporter。
- 设置全局 `TracerProvider`。
- 设置全局 propagator：`traceparent`、`tracestate`、`baggage`、`X-Track-ID`。
- 返回 graceful shutdown 函数。

服务退出时调用 `tracing.Shutdown(ctx)` flush exporter。

Runtime facade 对外接口：

```go
tracingcap.Use(opts...)
tracingcap.WithConfig(cfg)
tracingcap.WithConfigLoader(loader)
tracingcap.WithServiceName(name)
tracingcap.WithEnvironment(env)
tracingcap.WithLogger(log)
tracingcap.WithExternal()
```

## Gin Middleware

HTTP 服务在 `tracking` 后、`requestid` 前注册 tracing middleware：

```go
import gintracking "github.com/huwenlong92/sdkit/core/gin/tracking"
import gintracing "github.com/huwenlong92/sdkit/core/gin/tracing"
import ginrequestid "github.com/huwenlong92/sdkit/core/gin/requestid"

r.Use(gintracking.Middleware())
r.Use(gintracing.Middleware("admin"))
r.Use(ginrequestid.Middleware())
```

完整推荐顺序是：

```txt
Recovery -> Tracking -> Tracing -> RequestID -> CORS -> AccessLog -> BBR -> RateLimit -> Auth/Casbin -> Handler
```

中间件行为：

- 从请求头提取 `traceparent` / `tracestate`。
- 创建 HTTP server root span。
- 将 span context 写回 `c.Request.Context()`。
- 保留并关联 `X-Track-ID`。
- 通过统一 correlation helper 写入 `trace_id` / `span_id` / `track_id` / `request_id` / `traceparent`。
- HTTP root span 额外保留 `sd.track_id` / `sd.request_id`，用于兼容已有 Jaeger 检索习惯。
- 记录 method、path、host、status。
- handler panic 时 record error 并继续向外抛给 recovery。
- 5xx 响应标记 span error。

## Context API

```go
func TraceID(ctx context.Context) string
func SpanID(ctx context.Context) string
func RequestID(ctx context.Context) string
func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span)
```

业务手动 span：

```go
ctx, span := tracing.StartSpan(ctx, "risk.check")
defer span.End()
```

## HTTP Propagation

```go
func InjectHTTPHeader(ctx context.Context, header http.Header)
func ExtractHTTPHeader(ctx context.Context, header http.Header) context.Context
func HeadersFromContext(ctx context.Context) map[string]string
func ContextFromHeaders(ctx context.Context, headers map[string]string) context.Context
```

透传字段：

- `traceparent`
- `tracestate`
- `baggage`
- `X-Track-ID`
- `X-Request-ID`

`HeadersFromContext` / `ContextFromHeaders` 用于 Asynq、Outbox、EventBus 等非 HTTP carrier。队列 driver 不直接依赖该 API，而是通过 `core/queue` 的队列链路契约接入。

span 关联 attributes 统一入口：

```go
func SetSpanCorrelationAttributes(ctx context.Context, span trace.Span)
func HeaderValue(headers map[string]string, key string) string
func TraceIDFromHeaders(headers map[string]string) string
func SpanIDFromHeaders(headers map[string]string) string
```

该组 API 使用普通字段名 `trace_id`、`span_id`、`track_id`、`request_id`、`traceparent`，供 HTTP root span、Redis、Queue、Worker 失败日志等基础设施 span 和日志使用。HTTP root span 额外保留 `sd.track_id` / `sd.request_id` 兼容字段。

Context 写入遵循 typed key 规则：`core/tracing` 从 carrier 恢复 request id 时调用 `requestid.WithRequestID`，恢复 `trace_id/span_id` 日志字段时调用 `logger.WithField`；不再读写 string context key。

## Logger 集成

`logger.ContextFields(ctx)` 会自动输出：

- `track_id`
- `trace_id`
- `span_id`
- `request_id`
- `task_id` / `queue` / `type`（队列任务 context 中存在时）
- `run_id` / `job_id`（crontab run context 中存在时）

tracing 关闭或 context 没有有效 span 时，不输出空 `trace_id` / `span_id`。

`core/accesslog` 的 `trace_id` 只记录 OpenTelemetry trace ID；业务追踪 ID 使用独立的 `track_id` 字段。

## 数据库接入

`core/database` 初始化时会在 tracing 启用后自动接入数据库操作 tracing：

- GORM：注册轻量 callback plugin，覆盖 create/query/update/delete/row/raw。
- Pgx：在 `pgxpool.Config` 上挂载带 correlation wrapper 的 `otelpgx` tracer。
- Pgx 保留现有 `tracelog.TraceLog`，通过 `pgx/multitracer` 同时写 SQL 日志和 OpenTelemetry span。
- GORM 和 Pgx 数据库 span 都会写入 `trace_id/span_id/track_id/request_id/traceparent`。
- admin 系统管理 handler、登录 hooks、admin/api Casbin middleware 已透传请求 context，真实 HTTP 请求中的数据库操作会挂在对应 HTTP span 下。
- Queue：投递时创建 `producer::<task_type>` span，消费时创建 `consumer::<task_type>` span，业务 handler 创建 `handler::<task_type>` span，并通过 Asynq task headers 透传 trace 和业务追踪字段。
- Queue Outbox：保存任务时持久化 correlation headers；后台 Flush 时使用保存的 headers 恢复 enqueue context，让延迟投递 producer span 继续归属原始请求链路。
- Redis：`core/redis` hook 为普通命令创建 `redis.<cmd>` span，为 pipeline 创建 `redis.pipeline` span。
- Crontab：`core/crontab.Registry.Dispatch` 为每次任务执行创建 `crontab.execute` span；调度触发时通常是 root span，手动触发透传请求 context 时会挂在上游 trace 下，本地 handler 会继续透传该 context。
- EventBus/SSE：订阅 handler 创建 `eventbus.handle <event_name|topic>` consumer span，用于表示单条事件处理。
- WebSocket：upgrade 创建 `websocket.upgrade` span，message dispatch 创建 `websocket.message <action>` span，发布侧 broadcast headers 会跨 Redis gateway 传播。

接口：

```go
func InstrumentGorm(db *gorm.DB) error
func InstrumentPgxPoolConfig(cfg *pgxpool.Config) error
```

Redis tracing 在 `core/redis` hook 中实现，不在 `core/tracing` 暴露 Redis client instrumentation 入口。

## 内部约束

- 默认关闭，由配置显式启用。
- 不删除、不替换 `X-Track-ID`。
- 不在业务代码中直接创建新的全局 TracerProvider。
- 数据库调用必须透传 context，例如 `database.Gorm(ctx)`、`database.PGX(ctx).Query(ctx, ...)`，否则数据库 span 无法挂到 HTTP trace 下。
- Redis、队列、crontab handler 调用也必须透传 context，否则无法串联上游 trace，也无法写入 `track_id/request_id`。
- EventBus 发布时必须透传 context；driver 必须完整保留 `Event.Headers`，不能把 `track_id/request_id` 写入 `trace_id`。
- WebSocket message payload 内的 `request_id` 不能写入通用 `request_id`，必须使用 `ws_request_id`，避免和 HTTP upgrade 的 `X-Request-ID` 混淆。
- `core/tracing` 定义 trace/track/request 字段语义和底层传播 helper。
- `core/queue` 定义队列链路契约，具体 driver 只依赖 `core/queue` 的 correlation API。
- `pkg` 只实现 Asynq、Outbox、EventBus driver 等具体适配，不维护独立 header 规则。
- 新增 HTTP 框架接入时必须放到对应 adapter 目录；不要把 Gin、Echo 等框架依赖放回 `core/tracing` 根包。

## 更新记录

- 2026-05-26：Tracing runtime facade 默认作为 internal 底座能力，新增 `WithExternal()` 显式对外展示；默认依赖收敛到 `defaultUseOptions()`。
- 2026-05-27：Gin HTTP middleware 拆到 `core/gin/tracing`，`core/tracing` 根包只保留 tracing 抽象、provider 初始化和传播 helper。
- 2026-05-16：HTTP root span 接入统一 correlation helper，补齐 `trace_id/span_id/track_id/request_id/traceparent`；Queue/Redis 去掉分散的手写 `trace_id`。
- 2026-05-16：新增 `core/tracing/facade` Runtime Capability 接入层，按 `config.go/client.go/use.go/default.go` 组织，根包保留 tracing 实现和业务 API。
- 2026-05-13：EventBus/SSE 单事件处理创建 `eventbus.handle <name|topic>` span；redisstream driver 补齐默认 middleware 链。
- 2026-05-13：WebSocket upgrade/message/broadcast 接入 tracing correlation，区分 HTTP `request_id` 和 message `ws_request_id`。
- 2026-05-13：EventBus/Realtime 改为通过 headers 传播 trace/track/request，`TraceID` 不再承载 `track_id/request_id`。
- 2026-05-13：`core/queue` 增加队列 correlation API，Asynq、Outbox、Worker 失败日志通过队列契约接入，避免具体 driver 直接定义链路语义。
- 2026-05-13：Queue Outbox Flush 使用保存的 correlation headers 恢复 enqueue context，补偿投递 producer span 归入原始请求 trace。
- 2026-05-13：新增 tracing correlation helper，统一 map/header 传播、`traceparent` 解析和基础设施 span correlation attributes；Redis 改为复用该 helper。
- 2026-05-13：tracing correlation extraction 接入 typed context key，不再读写 string context key。
- 2026-05-13：GORM 和 Pgx 数据库 span 统一写入 `trace_id/track_id/request_id/traceparent` correlation attributes；Pgx 通过 otelpgx tracer provider wrapper 接入。
- 2026-05-13：AccessLog 明确区分 `track_id` 和 OpenTelemetry `trace_id`；`logger.ContextFields` 补充队列任务和 crontab run 字段。
- 2026-05-13：业务追踪入口统一为 `core/tracking`。
- 2026-05-13：补充 tracing/tracking 统一改造审阅入口，冻结 `trace_id/span_id/track_id/request_id` 字段语义。
- 2026-05-12：Crontab Runner 增加 cron root span，本地任务和队列投递任务均透传 cron trace context。
- 2026-05-12：Queue/Redis/GORM/worker handler span 增加 `trace_id` attribute，便于 Jaeger tags 中直接查看和过滤。
- 2026-05-12：Queue span 去除重复自定义 attributes，仅保留 `trace_id/track_id/request_id/traceparent`，任务信息使用 `messaging.*` 语义字段。
- 2026-05-12：Queue 增加 producer/consumer span 和 Asynq headers propagation；Redis hook 增加命令和 pipeline span。
- 2026-05-12：HTTP root span 增加 `sd.track_id` 和 `sd.request_id` attributes，便于 Jaeger 中按业务追踪 ID 和请求 ID 检索。
- 2026-05-12：admin/api 请求路径数据库操作透传 `Request.Context()`，支持在 Jaeger 中查看 HTTP span 下的 GORM 数据库 span。
- 2026-05-12：GORM/Pgx 数据库操作接入 tracing，Redis/Queue 保留扩展入口。
- 2026-05-12：新增 `core/tracing` OpenTelemetry 骨架，支持 Gin root span、OTLP gRPC exporter、Jaeger OTLP endpoint、logger trace/span 字段、HTTP propagation 和 instrumentation 接口。
