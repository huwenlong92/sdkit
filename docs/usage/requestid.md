# RequestID 使用文档

`core/requestid` 提供请求 ID 的 header 和 context API；Gin middleware 使用 `core/gin/requestid`。

## 注册中间件

```go
import ginrequestid "github.com/huwenlong92/sdkit/core/gin/requestid"

r.Use(ginrequestid.Middleware())
```

推荐注册顺序：

```go
r.Use(recovery.Middleware())
r.Use(gintracking.Middleware())
r.Use(gintracing.Middleware("admin"))
r.Use(ginrequestid.Middleware())
r.Use(cors.Middleware())
r.Use(adminmiddleware.AccessLog(accessLogger))
```

`RequestID` 应在 `AccessLog` 前注册，否则访问日志无法记录 `request_id`。当前服务把 `RequestID` 放在 `Tracing` 后面；tracing middleware 会在 `c.Next()` 返回后从 request context 读取 `request_id`，并补充到 HTTP root span 的 `sd.request_id` attribute。

## Header

使用请求头：

```txt
X-Request-ID
```

客户端传入则透传，未传则生成 UUID。响应会返回同名 header。

## 读取 request_id

```go
requestID := ginrequestid.Get(c)
```

非 Gin 场景使用：

```go
import "github.com/huwenlong92/sdkit/core/requestid"

ctx = requestid.WithRequestID(ctx, requestID)
requestID = requestid.FromContext(ctx)
```

值会同时写入：

- Gin Context：`request_id`
- response header：`X-Request-ID`
- request context：typed key

## 日志透传

调用数据库、Redis、队列等基础设施时透传 `c.Request.Context()`，底层日志才能自动带上 `request_id`：

```go
func Handler(c *gin.Context) {
    ctx := c.Request.Context()
    // db.WithContext(ctx) / redis.Get(ctx, key) / queue.Enqueue(ctx, task)
}
```

## 注意事项

- 不要在业务 handler 中重复生成 request id。
- 日志字段名固定为 `request_id`。
- Header 固定为 `X-Request-ID`。
- `request_id` 不等同于 OpenTelemetry `trace_id`，也不等同于业务 `track_id`。
- 新代码不要直接用 `context.WithValue` 写 `requestid.Key`。
