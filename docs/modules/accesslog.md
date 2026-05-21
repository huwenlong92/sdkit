# AccessLog 模块方案

## 作用

`core/accesslog` 提供通用 HTTP 访问日志采集能力，不绑定具体数据库表。服务侧通过实现 `Writer` 决定落库方式。

## 初始化

```go
accessLogger := accesslog.NewLogger(writer, accesslog.Config{
    QueueSize:     1024,
    BatchSize:     100,
    FlushInterval: 200 * time.Millisecond,
})
accessLogger.Start(ctx)
```

## 配置项

| 字段 | 说明 |
|------|------|
| `QueueSize` | 异步队列长度 |
| `BatchSize` | 批量写入大小 |
| `FlushInterval` | 定时 flush 间隔 |

## 对外接口

```go
type Writer interface {
    WriteBatch(ctx context.Context, entries []*Entry) error
}

type Entry struct {
    Source    string
    TrackID   string
    TraceID   string
    RequestID string
    UID       string
    Method    string
    Path      string
    Query     string
    IP        string
    UserAgent string
    Headers   []byte
    ReqBody   []byte
    RespBody  []byte
}
```

操作者身份通过业务服务注入 resolver，core 不直接读取 `auth_user_id` 或解释用户语义：

```go
type Actor struct {
    ID   string
    Type string
    Name string
}

type ActorResolver func(*gin.Context) Actor
type Skipper func(*gin.Context) bool
```

敏感字段默认覆盖 `authorization/cookie/password/token/secret`，业务可按服务追加字段和 header：

```go
r.Use(accesslog.Middleware(
    "admin",
    accesslog.WithLogger(accessLogger),
    accesslog.WithAdditionalSensitiveFields("otp", "pin_code"),
    accesslog.WithAdditionalSensitiveHeaders("X-Internal-Secret"),
))
```

可通过 `WithSkipper` 按请求跳过记录，也可以在请求处理中调用 `Skip(c)` 标记当前请求不写访问日志：

```go
r.Use(accesslog.Middleware(
    "admin",
    accesslog.WithLogger(accessLogger),
    accesslog.WithSkipper(func(c *gin.Context) bool {
        return c.Request.URL.Path == "/ping"
    }),
))

func Handler(c *gin.Context) {
    accesslog.Skip(c)
}
```

固定的 method 或 IP 白名单可以直接用内置选项，不需要每次手写 `WithSkipper`：

```go
r.Use(accesslog.Middleware(
    "admin",
    accesslog.WithLogger(accessLogger),
    accesslog.WithSkipMethods("OPTIONS", "HEAD"),
    accesslog.WithSkipIPs("127.0.0.1", "10.0.0.0/8"),
))
```

`WithSkipIPs` 同时支持精确 IP 和 CIDR 网段。配置值解析失败时会忽略该项；如果需要强校验配置，应在应用层加载配置时先校验。

测试或调试场景可显式传空列表，关闭字段或 header 脱敏：

```go
r.Use(accesslog.Middleware(
    "admin",
    accesslog.WithLogger(accessLogger),
    accesslog.WithSensitiveFields(),
    accesslog.WithSensitiveHeaders(),
))
```

## 中间件

```go
r.Use(tracking.Middleware())
r.Use(tracing.Middleware("admin"))
r.Use(requestid.Middleware())
r.Use(accesslog.Middleware("admin", accesslog.WithLogger(accessLogger)))
```

推荐顺序是 `Recovery -> Tracking -> Tracing -> RequestID -> CORS -> AccessLog -> BBR -> RateLimit -> Auth/Casbin -> Handler`。AccessLog 必须在 Tracking、Tracing、RequestID 之后，才能完整采集 `track_id/trace_id/request_id`。

中间件采集 `track_id`、`trace_id`、`request_id`、uid、请求头、请求体、响应体、状态码和耗时：

- `Entry.TrackID`：业务追踪 ID，来自 `core/tracking` / `X-Track-ID`。
- `Entry.TraceID`：OpenTelemetry trace ID，来自当前 request context 的 span；未注册 tracing middleware 时为空。
- `Entry.RequestID`：一次 HTTP 请求 ID，来自 `core/requestid` / `X-Request-ID`。
- `Entry.UID`：由 `ActorResolver` 注入；未注入时为空。

## 注意事项

- `Push` 非阻塞，队列满时丢弃日志。
- 未传入 `Logger` 时，`Middleware` 只透传请求，不采集请求体或响应体。
- `Start(ctx)` 在 context 结束时 flush 剩余日志。
- 敏感 header 会过滤。
- JSON body 的 password/token/secret/cookie/authorization 字段会脱敏。
- form-urlencoded 和 multipart/form-data 的 password/token/secret/cookie/authorization 字段会脱敏。
- 可通过 `WithSensitiveFields` / `WithSensitiveHeaders` 覆盖脱敏规则，传空表示关闭；通过 `WithAdditionalSensitiveFields` / `WithAdditionalSensitiveHeaders` 在默认规则上追加。
- 可通过 `WithSkipper`、`WithSkipMethods`、`WithSkipIPs` 或 `Skip(c)` 跳过当前请求访问日志。
- 请求体采集只保存有限摘要，不会截断后续 handler 可读取的原始 body。

## 更新记录

- 2026-05-10：补充 `trace_id/request_id` 采集和 JSON body 敏感字段脱敏。
- 2026-05-13：`Entry` 新增 `track_id`，`trace_id` 恢复为 OpenTelemetry trace ID，访问日志不再把业务 tracking ID 写入 `TraceID`。
- 2026-05-13：新增 `ActorResolver`，访问日志不再默认读取 `auth_user_id`。
- 2026-05-13：访问日志对外身份注入接口统一为 `ActorResolver`。
- 2026-05-13：同步 HTTP 推荐 middleware 顺序，明确 AccessLog 位于 Tracking/Tracing/RequestID 之后。
- 2026-05-21：`Logger` 为空时中间件直接透传；请求体采样不再截断 handler 输入；补充 form 和 multipart 字段脱敏；新增敏感字段和 header 覆盖/追加配置。
- 2026-05-21：新增 `WithSkipper` 和 `Skip(c)`，支持按请求跳过访问日志记录。
- 2026-05-21：新增 `WithSkipMethods` 和 `WithSkipIPs`，支持按 HTTP method、精确 IP 和 CIDR 网段跳过访问日志记录。
