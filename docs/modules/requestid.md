# RequestID 模块方案

## 作用

`core/requestid` 负责为每个 HTTP 请求提供请求级唯一 ID，用于幂等排查、单次请求定位和日志串联。

RequestID 与 TrackID 的职责不同：

- `request_id`：标识一次 HTTP 请求。
- `track_id`：标识一次业务请求追踪链路。

当前模块只处理 Gin HTTP 请求，不负责分布式追踪协议转换。

## 初始化

在 Gin router 中注册：

```go
r.Use(requestid.Middleware())
```

推荐顺序：

```txt
Recovery -> Tracking -> Tracing -> RequestID -> CORS -> AccessLog -> BBR -> RateLimit -> Auth/Casbin -> Handler
```

RequestID 必须在 AccessLog 前注册。当前服务把 RequestID 放在 Tracing 后面，tracing middleware 会在下游 middleware 执行完成后读取 `request_id`，写入 HTTP root span 的 `sd.request_id` attribute。

## 配置项

当前没有配置项。

## 对外接口

```go
const (
    Header = "X-Request-ID"
    Key    = "request_id"
)

func Middleware() gin.HandlerFunc
func WithRequestID(ctx context.Context, requestID string) context.Context
func FromContext(ctx context.Context) string
func Get(c *gin.Context) string
```

## 中间件

`Middleware` 的行为：

1. 优先读取请求头 `X-Request-ID`。
2. 请求头为空时生成 UUID。
3. 写入 Gin Context：`request_id`。
4. 写入响应头：`X-Request-ID`。
5. 写入 `c.Request.Context()`：typed key。
6. 调用后续 handler。

## Hook

模块本身不提供 hook。以下模块会读取 `request_id`：

- `core/accesslog`：写入访问日志 Entry。
- `core/logger`：`ContextFields(ctx)` / `WithContext(ctx, log)`。
- `core/database`：pgx logger 从 context 追加字段。
- `core/redis`：Redis hook 从 context 追加字段。
- `core/queue` / `core/eventbus` / `core/realtime`：通过 headers 或消息上下文传播 request 信息。

## 使用示例

```go
r.Use(requestid.Middleware())

func Handler(c *gin.Context) {
    requestID := requestid.Get(c)
    ctx := c.Request.Context()
    _ = requestID
    _ = ctx
}
```

调用数据库、Redis、队列等基础设施时应透传 `c.Request.Context()`，让底层日志自动带上 `request_id`。非 Gin 场景写入 request id 时使用 `WithRequestID`，读取使用 `FromContext`。

## 注意事项

- 不要在业务 handler 中重复生成 request id。
- 客户端传入 `X-Request-ID` 时会透传，不做格式校验。
- 日志字段名固定为 `request_id`，不要混用 `requestId`、`request-id`。
- response header 固定为 `X-Request-ID`。
- `request_id` 只表示一次 HTTP 请求或消息请求 ID，不要写入 `trace_id` 或 `track_id`。
- 新代码不要直接 `context.WithValue(ctx, requestid.Key, value)`；`requestid.Key` 仅作为 Gin context 和字段名使用，不作为 request context key。

## 已知限制

- 不做 header 值长度限制。
- 不做全局唯一性校验。
- 不直接支持非 Gin 框架。

## 更新记录

- 2026-05-10：补充 request context 透传规则和 logger/pgx/redis/accesslog 关联说明。
- 2026-05-13：同步 HTTP 推荐 middleware 顺序，明确 `request_id` 与 `trace_id/track_id` 的字段边界。
- 2026-05-13：新增 typed context key 层，`WithRequestID` / `FromContext` 只使用 typed key。
