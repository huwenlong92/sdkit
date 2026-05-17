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
- `Start(ctx)` 在 context 结束时 flush 剩余日志。
- 敏感 header 会过滤。
- JSON body 的 password/token/secret/cookie/authorization 字段会脱敏。
- form-urlencoded 原始 body 暂未做字段级脱敏。

## 更新记录

- 2026-05-10：补充 `trace_id/request_id` 采集和 JSON body 敏感字段脱敏。
- 2026-05-13：`Entry` 新增 `track_id`，`trace_id` 恢复为 OpenTelemetry trace ID，访问日志不再把业务 tracking ID 写入 `TraceID`。
- 2026-05-13：新增 `ActorResolver`，访问日志不再默认读取 `auth_user_id`。
- 2026-05-13：访问日志对外身份注入接口统一为 `ActorResolver`。
- 2026-05-13：同步 HTTP 推荐 middleware 顺序，明确 AccessLog 位于 Tracking/Tracing/RequestID 之后。
