# 通用组件

HTTP 通用能力按职责拆分在 `core/*` 和应用侧 middleware 中，服务按需显式注册。

## Recovery

```go
import "github.com/huwenlong92/sdkit/core/recovery"

r.Use(recovery.Middleware())
```

捕获 panic，记录 stack，并返回统一错误响应。

## Tracking

详见 [tracking.md](tracking.md)。

## Tracing

详见 [tracing.md](tracing.md)。

## RequestID

详见 [requestid.md](requestid.md)。

## CORS

```go
import "github.com/huwenlong92/sdkit/core/cors"

r.Use(cors.Middleware())
r.Use(cors.Middleware(
    cors.WithOrigins("https://example.com"),
    cors.WithMethods("GET", "POST"),
    cors.WithHeaders("Content-Type", "Authorization"),
    cors.WithExposeHeaders("X-Session-Expires-At", "X-Request-ID", "X-Track-ID"),
    cors.WithMaxAge("3600"),
))
```

默认允许：

- origins: `*`
- methods: `GET, POST, OPTIONS`
- headers: `Content-Type, Authorization, X-Request-ID, X-Track-ID`
- expose headers: `X-Session-Expires-At, X-Request-ID, X-Track-ID`

## 推荐顺序

```go
r.Use(recovery.Middleware())
r.Use(tracking.Middleware())
r.Use(tracing.Middleware("admin"))
r.Use(requestid.Middleware())
r.Use(cors.Middleware())
r.Use(adminmiddleware.AccessLog(accessLogger))
r.Use(appmiddleware.BBR(bbrCfg))
r.Use(adminmiddleware.RateLimit(limiterCfg))
```

`Tracking`、`Tracing` 和 `RequestID` 应在 `AccessLog` 前注册，否则访问日志无法完整记录 `track_id/trace_id/request_id`。当前 admin/api/sse 的基础顺序保持为 `Tracking -> Tracing -> RequestID`。
