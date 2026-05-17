# Tracking 使用文档

`core/tracking` 用于生成或透传 HTTP 业务追踪 ID。

## 注册中间件

```go
import "github.com/huwenlong92/sdkit/core/tracking"

r.Use(tracking.Middleware())
```

推荐注册顺序：

```go
r.Use(recovery.Middleware())
r.Use(tracking.Middleware())
r.Use(tracing.Middleware("admin"))
r.Use(requestid.Middleware())
r.Use(cors.Middleware())
r.Use(adminmiddleware.AccessLog(accessLogger))
```

`Tracking` 应在 `Tracing` 和 `AccessLog` 前注册，否则 HTTP root span、访问日志和底层日志无法记录 `track_id`。`RequestID` 放在 `Tracing` 后面，tracing middleware 会在后续 middleware 写入 request context 后补充 `sd.request_id`。

## Header

默认使用请求头：

```txt
X-Track-ID
```

客户端传入则透传，未传则生成 UUID。响应会返回同名 header。

## 配置

```go
r.Use(tracking.Middleware(tracking.Config{
    Enabled:        true,
    Header:         "X-Track-ID",
    ResponseHeader: "X-Track-ID",
    Generator:      "uuid",
    ForceNew:       false,
}))
```

`ForceNew=true` 会忽略客户端传入值并重新生成。

## 读取 track_id

```go
trackID := tracking.Get(c)
sameTrackID := tracking.TrackID(c.Request.Context())
```

值会同时写入：

- Gin Context：`track_id`
- response header：`X-Track-ID`
- request context：typed key

非 HTTP 场景可手动透传：

```go
ctx = tracking.WithTrackID(ctx, trackID)
```

## 日志透传

调用数据库、Redis、队列等基础设施时透传 `c.Request.Context()`，底层日志才能自动带上 `track_id`：

```go
func Handler(c *gin.Context) {
    ctx := c.Request.Context()
    // db.WithContext(ctx) / redis.Get(ctx, key) / queue.Enqueue(ctx, task)
}
```

业务日志可使用：

```go
logger.WithContext(ctx, logger.Named("order")).Info("order created")
```

AccessLog 会把该值写入 `track_id`。`trace_id` 只来自 OpenTelemetry span，不再用于存储业务 tracking ID。

## 导入约束

业务追踪统一使用 `github.com/huwenlong92/sdkit/core/tracking`。`core/tracking/tests` 中的 import guard 会阻止重新引入非正式 tracking 入口。

## 注意事项

- 不要在业务 handler 中重复生成 track id。
- 日志字段名固定为 `track_id`。
- 不要把 `track_id` 写入 `trace_id` 字段。
- Header 默认固定为 `X-Track-ID`。
- Tracking 只负责业务追踪 ID，不实现 OpenTelemetry、Jaeger、span、metrics。
- 新代码不要直接用 `context.WithValue` 写 `tracking.Key`。
