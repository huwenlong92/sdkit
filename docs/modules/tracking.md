# Tracking 模块方案

## 作用

`core/tracking` 负责业务追踪 ID 的配置、生成和 context 透传；Gin HTTP middleware 位于 `core/gin/tracking`。

当前模块只提供轻量 tracking 能力，不实现 OpenTelemetry、Jaeger、W3C Trace Context、span tree 或采样策略。

## Tracking 与 RequestID

- `track_id`：标识一次业务请求追踪链路。
- `request_id`：标识一次 HTTP 请求。

## 初始化

```go
import gintracking "github.com/huwenlong92/sdkit/core/gin/tracking"

r.Use(gintracking.Middleware())
```

推荐顺序：

```txt
Recovery -> Tracking -> Tracing -> RequestID -> CORS -> AccessLog -> BBR -> RateLimit -> Auth/Casbin -> Handler
```

Tracking 应在 Tracing、AccessLog、Auth 和业务 handler 之前注册，保证 HTTP root span、访问日志和后续日志都能读取 `track_id`。

## 配置项

```go
type Config struct {
    Enabled        bool
    Header         string
    ResponseHeader string
    Generator      string
    ForceNew       bool
}
```

默认配置：

```yaml
tracking:
  enabled: true
  header: X-Track-ID
  response_header: X-Track-ID
  generator: uuid
  force_new: false
```

## 对外接口

```go
const (
    Header = "X-Track-ID"
    Key    = "track_id"
)

func DefaultConfig() Config
func WithTrackID(ctx context.Context, trackID string) context.Context
func TrackID(ctx context.Context) string
func MustTrackID(ctx context.Context) string
func NewTrackID() string
```

Gin 适配入口：

```go
import gintracking "github.com/huwenlong92/sdkit/core/gin/tracking"
import coretracking "github.com/huwenlong92/sdkit/core/tracking"

func Middleware(configs ...coretracking.Config) gin.HandlerFunc
func Get(c *gin.Context) string
```

## 中间件

`Middleware` 的行为：

1. 默认优先读取请求头 `X-Track-ID`。
2. 请求头为空时生成 UUID。
3. `ForceNew=true` 时忽略请求头并重新生成。
4. 写入 Gin Context：`track_id`。
5. 写入响应头：`X-Track-ID`。
6. 写入 `c.Request.Context()`：typed key。
7. `Enabled=false` 时直接跳过，不写 header 和 context。

## Hook

以下模块会读取 `track_id`：

- `core/accesslog`：写入访问日志 `Entry.TrackID` 和 `system_access_log.track_id`。
- `core/logger`：`ContextFields(ctx)` / `WithContext(ctx, log)` / `Ctx(ctx)`。
- `core/database`：pgx logger 从 context 追加字段。
- `core/redis`：Redis hook 从 context 追加字段。
- `core/queue`：投递时写入 task headers，worker 失败日志入库时落到 `track_id`。
- `core/crontab`：Runner 复用或生成 `track_id`，run 记录和任务日志事件都会带该字段。
- `core/eventbus` / `core/realtime`：通过 headers 传播 `track_id`，不写入 `trace_id`。

## 导入约束

业务追踪的纯能力统一使用 `github.com/huwenlong92/sdkit/core/tracking`；Gin 接入统一使用 `github.com/huwenlong92/sdkit/core/gin/tracking`。仓库内由 import guard 阻止重新引入非正式 tracking 入口。

## 注意事项

- 不要在业务 handler 中重复生成 track id。
- 客户端传入 `X-Track-ID` 时会透传，不做格式校验。
- 日志字段名固定为 `track_id`，不要混用 `trackId`、`track-id`。
- 不要把业务追踪 ID 写入 `trace_id`；`trace_id` 只表示 OpenTelemetry trace。
- response header 默认固定为 `X-Track-ID`。
- `MustTrackID` 不 panic；context 中不存在时返回一个新 UUID。
- 新代码不要直接 `context.WithValue(ctx, tracking.Key, value)`；`tracking.Key` 仅作为 Gin context 和字段名使用，不作为 request context key。

## 更新记录

- 2026-05-12：新增 `core/tracking`，明确业务追踪 ID 使用 `track_id/X-Track-ID` 语义，补充配置、context API、middleware 和 logger 关联说明。
- 2026-05-13：AccessLog、Queue、Crontab 文档统一 `track_id` / `trace_id` 语义，禁止继续用 `TraceID` 承载业务 tracking ID。
- 2026-05-13：同步 HTTP 推荐 middleware 顺序为 `Tracking -> Tracing -> RequestID`，明确 Tracking 必须先于 Tracing。
- 2026-05-13：新增 typed context key 层，`WithTrackID` / `TrackID` 只使用 typed key。
- 2026-05-13：业务追踪入口统一为 `core/tracking`，import guard 放在 `core/tracking/tests`。
- 2026-05-27：Gin middleware 拆到 `core/gin/tracking`，`core/tracking` 保留配置、生成和 context API。
