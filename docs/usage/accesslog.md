# accesslog 访问日志

## 概述

`core/accesslog` 只提供通用访问日志能力，不绑定任何数据库表或业务模型：

- `Entry`：通用日志结构
- `Writer`：批量写入接口
- `Logger`：异步队列 + 批量 flush
- `Middleware`：Gin 请求采集中间件

具体落库由业务服务自己实现 `Writer`。当前 admin 服务使用 GORM 写入自己的 `SystemAccessLog` 表。

## core 接口

```go
type Entry struct {
    Source     string
    TrackID    string
    TraceID    string
    RequestID  string
    UID        string
    Method     string
    Path       string
    Query      string
    IP         string
    UserAgent  string
    Headers    []byte
    ReqBody    []byte
    StatusCode int
    RespBody   []byte
    Latency    int64
    CreatedAt  int64
}

type Writer interface {
    WriteBatch(ctx context.Context, entries []*Entry) error
}
```

`core/accesslog` 不允许依赖 admin、GORM model 或具体数据库表。

## 创建 Logger

```go
writer := adminaccesslog.NewWriter(database.DB, 100)

logger := accesslog.NewLogger(writer, accesslog.Config{
    QueueSize:     1024,
    BatchSize:     100,
    FlushInterval: 200 * time.Millisecond,
})

ctx, cancel := context.WithCancel(context.Background())
logger.Start(ctx)
defer cancel()
```

行为：

- `Push(entry)` 非阻塞
- 队列满时丢弃日志，不阻塞接口响应
- 满 `BatchSize` 刷新
- 到 `FlushInterval` 刷新
- `ctx.Done()` 时刷新剩余日志
- 写入错误只打印日志，不影响请求

## Gin 中间件

core 中间件通过 `WithLogger` 接入异步 Logger：

```go
r.Use(accesslog.Middleware(
    "admin",
    accesslog.WithLogger(logger),
    accesslog.WithActorResolver(func(c *gin.Context) accesslog.Actor {
        identity := authgin.GetIdentity(c)
        if identity == nil {
            return accesslog.Actor{}
        }
        return accesslog.Actor{
            ID:   strconv.FormatInt(identity.SubjectID, 10),
            Type: identity.SubjectType,
            Name: identity.Username,
        }
    }),
))
```

admin 已封装为：

```go
import adminmiddleware "sdkitgo/app/admin/middleware"

r.Use(adminmiddleware.AccessLog(logger))
```

推荐注册顺序：

```text
Recovery -> Tracking -> Tracing -> RequestID -> CORS -> AccessLog -> BBR -> RateLimit -> Auth/Casbin -> Handler
```

AccessLog 必须位于 Tracking、Tracing、RequestID 之后，否则 `track_id`、`trace_id`、`request_id` 可能为空。BBR、RateLimit、Auth/Casbin 位于 AccessLog 之后时，被限流、未认证或未授权的请求也会被采集。

## admin 接入

admin 启动时创建 writer/logger，并传入 router：

```go
logCtx, cancelLog := context.WithCancel(context.Background())

accessLogger := accesslog.NewLogger(
    adminaccesslog.NewWriter(database.DB, 100),
    accesslog.Config{
        QueueSize:     1024,
        BatchSize:     100,
        FlushInterval: 200 * time.Millisecond,
    },
)
accessLogger.Start(logCtx)

router := SetupRouter(rateLimitCfg, bbrCfg, accessLogger)
```

关闭服务时取消 context，触发剩余日志 flush：

```go
err := httpServer.Shutdown(ctx)
cancelLog()
return err
```

## admin Writer

admin 的 writer 位于 `app/admin/infra/component/accesslog/writer.go`，使用 GORM `CreateInBatches`：

```go
type Writer struct {
    db        *gorm.DB
    batchSize int
}

func (w *AccessLogWriter) WriteBatch(ctx context.Context, entries []*accesslog.Entry) error {
    // Entry -> admin model.SystemAccessLog
    return w.db.WithContext(ctx).CreateInBatches(rows, w.batchSize).Error
}
```

默认 batch size 为 `100`。

## admin 表模型

admin 落库模型位于 `app/admin/model/system_access_log.go`：

| 字段 | 说明 |
|------|------|
| `source` | 服务来源，admin 固定为 `admin` |
| `track_id` | 业务追踪 ID，来自 `core/tracking` / `X-Track-ID` |
| `trace_id` | OpenTelemetry trace ID，来自当前 request context 的 span；未注册 tracing middleware 时为空 |
| `request_id` | 请求 ID，来自 `core/requestid` |
| `uid` | 用户 ID，未登录为空字符串 |
| `method` | 请求方法 |
| `path` | 请求路径 |
| `query` | URL query |
| `ip` | 客户端 IP |
| `user_agent` | User-Agent |
| `headers` | 请求头 JSON，敏感头已过滤 |
| `req_body` | 请求体 |
| `resp_body` | 响应体 |
| `status_code` | HTTP 状态码 |
| `latency` | 耗时，单位 ms |
| `created_at` | 创建时间，Unix 毫秒 |
| `updated_at` | 更新时间，Unix 毫秒 |

## 采集内容

中间件记录：

- method
- path
- query
- ip
- user-agent
- headers
- request body
- response status code
- response body
- latency
- uid
- source
- track_id（业务追踪 ID）
- trace_id（OpenTelemetry trace ID）
- request_id

请求中只做采集和非阻塞入队，不同步写数据库。

## req_body 捕获策略

| Content-Type | 捕获内容 |
|------|------|
| `application/json` | JSON 结构，敏感字段替换为 `(redacted)`，超过 200 字符的字符串值替换为 `(string: N chars)` |
| `multipart/form-data` | 表单字段名和值，不含文件二进制 |
| `application/octet-stream`、`image/*`、`video/*`、`audio/*` | `(binary body omitted)` |
| `application/x-www-form-urlencoded` | 原始 `key=value` |
| 空 body | 空 |

请求体读取后会还原给后续 handler，不影响 `ShouldBindJSON`、`PostForm` 等读取。

## resp_body 捕获策略

| Content-Type | 捕获内容 |
|------|------|
| `text/*`、`application/json`、`application/xml`、`application/javascript`、`application/x-www-form-urlencoded` | 内容，最多 32KB |
| 其他，如图片、文件下载 | `(binary body omitted)` |

## 敏感头过滤

以下请求头不会写入日志：

- `Authorization`
- `Cookie`
- `Set-Cookie`
- `X-Api-Key`
- `X-Auth-Token`

## JSON 敏感字段过滤

JSON 请求体中字段名包含以下关键词时，字段值会替换为 `(redacted)`：

- `authorization`
- `cookie`
- `password`
- `token`
- `secret`

匹配不区分大小写，`access_token`、`refreshToken`、`client_secret` 都会脱敏。

## 工具函数

```go
import "github.com/huwenlong92/sdkit/core/accesslog"

postMap, err := accesslog.GetRequestBody(c)
queryMap := accesslog.GetRequestQuery(c)
headers := accesslog.GetRequestHeaders(c)

inputs, err := accesslog.RequestInputs(c)
// 返回 {"GET": {...}, "POST": {...}}

jsonStr := accesslog.FilterHeaders(r.Header)
```
