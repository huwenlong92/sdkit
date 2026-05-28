# Realtime 模块

## 模块定位

`core/realtime` 定义实时能力的稳定契约：event、identity、client、publisher、gateway、router、registry、dispatcher、room、presence、错误和配置。

具体运行时实现位于：

```txt
pkg/realtime/gateway/
pkg/realtime/memory/
pkg/realtime/publisher/
pkg/realtime/transport/
pkg/realtime/ws/
pkg/realtime/sse/
```

`app/realtime` 是唯一 realtime gateway 服务入口，服务层 WebSocket/SSE adapter 位于 `app/realtime/transports/ws` 和 `app/realtime/transports/sse`。

本阶段是 breaking refactor，不保留旧 manager、旧 action router dispatch、旧 direct response helper 或旧 int64 user runtime。

## Runtime 架构

```text
transport
  -> gateway runtime
  -> router match
  -> compiled middleware pipeline
  -> handler
```

`pkg/realtime/gateway.Runtime` 是 Realtime Runtime Core，负责：

- event orchestration
- router lookup
- compiled pipeline invoke
- local dispatcher / publisher 入口收口
- action activity lifecycle 回调

Router 只做 route register、group route 和 route lookup。middleware chain 在 route 注册阶段编译到 `Route.Compiled`，runtime 处理 action 时不再动态拼接 middleware。

## Gateway State Runtime

`app/realtime/internal/state` 是 gateway 状态运行时，状态按职责拆分：

```txt
app/realtime/internal/state/
    connection/
    heartbeat/
    presence/
    room/
    session/
```

状态边界：

- `connection`：连接生命周期，统一 `connect -> authenticate -> active -> idle -> disconnect -> cleanup`。
- `presence`：用户在线状态、设备、平台、last seen 和 reconnect/idle/offline 状态。
- `room`：只维护 membership、join、leave 和 member lookup，不承载业务房间语义。
- `session`：同用户同设备重连时替换旧 connection，并触发旧连接 cleanup。
- `heartbeat`：维护 last active、timeout detect 和 timeout cleanup。

`app/realtime/internal/registry` 只保留 connection index。room membership 和 presence 不再由 registry 维护；disconnect cleanup 统一由 state runtime 执行：

```txt
connection remove
room leave
presence update
session cleanup
heartbeat cleanup
```

Transport 只触发标准生命周期：

```txt
on connect
on disconnect
on activity
```

WS/SSE transport 不维护 presence、room 或业务状态。WebSocket action 进入 `Runtime.Handle()` 时也会通过 lifecycle 刷新 last active、heartbeat 和 presence。

## Core Contracts

### Publisher

```go
type Publisher interface {
    PushUser(context.Context, string, *Event) error
    PushRoom(context.Context, string, *Event) error
    Broadcast(context.Context, *Event) error
}
```

Publisher 只负责 eventbus publish，不允许持有 registry、做 local dispatch 或写 transport connection。

### Runtime Facade

`core/realtime/facade` 是 runtime capability 入口，负责在服务启动阶段依赖 `core/eventbus/facade` 创建 realtime publisher，并绑定到 `runtime.App` container 和 `core/realtime` 默认 publisher。

文件边界：

- `core/realtime/binding.go`：只放 `KeyRealtime`、`From(app)`、`Bind(app, publisher)`。
- `core/realtime/facade/use.go`：只放 `Use(...)`、`WithConfig(...)`、`WithConfigLoader(...)`、依赖声明和 shutdown 生命周期。

业务代码优先使用 `core/realtime.PushUser`、`PushRoom`、`Broadcast` 或服务自己的 `infra/realtime` adapter，不直接依赖 `facade.From(app)`。

### Gateway

```go
type Gateway interface {
    Handle(*ActionContext) error
    Publish(context.Context, *Event) error
    PushUser(context.Context, string, *Event) error
    PushClient(context.Context, string, *Event) error
    PushRoom(context.Context, string, *Event) error
    Broadcast(context.Context, *Event) error
}
```

### Dispatcher

```go
type Dispatcher interface {
    DispatchEvent(context.Context, *Event) error
    DispatchLocal(context.Context, *Event) error
    DispatchClient(context.Context, string, *Event) error
    PushUser(context.Context, string, *Event) error
    PushClient(context.Context, string, *Event) error
    PushRoom(context.Context, string, *Event) error
    Broadcast(context.Context, *Event) error
}
```

Dispatcher 只负责本进程 local dispatch。

`app/realtime/internal/delivery.EventDispatcher` 会对 room delivery 使用 state room index 做 member lookup，registry 只负责按 client id / user id 找连接。

### Registry

```go
type Registry interface {
    Add(*Client) error
    Remove(clientID string) error
    Get(clientID string) (*Client, bool)
    GetUserClients(userID string) []*Client
    GetRoomClients(roomID string) []*Client
}
```

Registry 不包含 Push、Dispatch、Handle、Publish。

## Router 与 Pipeline

`pkg/realtime/gateway.Router` 实现：

```go
type Router interface {
    Use(...MiddlewareFunc)
    Group(prefix string, ...MiddlewareFunc) Router
    On(action string, ...HandlerFunc)
    Match(action string) (*Route, bool)
}
```

`pipeline.go` 统一执行 global middleware、group middleware 和 route handler。Router 不承担 runtime dispatch。

Route 注册后的运行时模型：

```go
type Route struct {
    Action     string
    Middleware []MiddlewareFunc
    Handler    HandlerFunc
    Compiled   HandlerFunc
}
```

`Compiled` 在 `On()` 阶段生成，包含 global middleware、group middleware 和 route handler。runtime 的处理顺序固定为：

```text
Runtime.Handle()
  -> Router.Match()
  -> Route.Compiled(ActionContext)
```

## ActionContext

`ActionContext` 是 runtime context，不再只是 request object。

关键方法：

- `Abort()`
- `Next() error`
- `Reply(action string, data any) error`
- `PushUser(userID string, evt *Event) error`
- `PushRoom(roomID string, evt *Event) error`

pipeline 执行前会把当前 route 的 handler chain 写入 `ActionContext` 运行态。`Next()` 推进到下一个 middleware 或最终 handler，`Abort()` 终止后续执行。middleware 可使用 `return next(ctx)`，也可在需要后置逻辑时直接调用 `err := ctx.Next()`。

`Reply`、`PushUser`、`PushRoom` 都必须通过 `Gateway`，不得绕过 runtime 直接写 connection 或 client channel。

## Identity

Runtime 内用户标识统一使用 string：

```go
type Client struct {
    ID       string
    Identity *Identity
    UserID   string
}

func NewUserIdentity(userID string, tenantID int64) *Identity
```

认证边界可以从历史整数 subject 转换成 `AuthResult.UserID string`。registry、dispatcher、publisher 中禁止使用整数型 user id。

## Event

`Event` 的标准字段：

```go
type Event struct {
    Action    string
    RequestID string
    TraceID   string
    Timestamp int64
    Headers   map[string]string
    Data      any
    Target    *Target
}
```

兼容字段 `Event`、`RoomID`、`Payload`、`Time` 保留用于现有消息协议迁移；用户目标必须写入 `Target.UserID string`。

## Transport

`pkg/realtime/ws` 只保留协议层能力：

- conn
- read
- write
- heartbeat
- codec adapter
- lifecycle

禁止在 `pkg/realtime/ws` 放业务 handler、middleware、router、dispatch 或 runtime。

`pkg/realtime/transport` 提供协议层 helper：

- `codec.go`
- `heartbeat.go`
- `recover.go`
- `tracing.go`
- `metrics.go`

服务层 transport 约束：

- `app/realtime/transports/ws` 只负责 WebSocket 读写、心跳、action codec adapter、连接生命周期和 `runtime.Handle(ctx)` 调用。
- `app/realtime/transports/sse` 只负责 SSE 响应头、事件写出、heartbeat、连接注册清理。
- route、middleware、handler 只允许出现在 `app/realtime/router.go`、`app/realtime/middleware` 和 `app/realtime/handler`。

## Metrics

Gateway state runtime 当前提供：

- connection count
- online users
- room count / room member count
- heartbeat timeout count
- reconnect count
- last dispatch latency

`/realtime/status` 会返回 heartbeat timeout、reconnect 和 dispatch latency 等状态字段；底层采样由 state runtime 与 delivery dispatcher 写入。

## IM Demo 业务示例

`app/im` 是 realtime runtime 的最小业务示例，不实现完整 IM，只验证业务接入方式：

```text
client
  -> app/realtime
  -> runtime route
  -> app/im handler
  -> app/im domain event
  -> app/im consumer
  -> notify infra
  -> core/realtime publisher
  -> eventbus
  -> app/realtime delivery
  -> receiver
```

当前注册 action：

- `chat.join`：加入 runtime room，并发布 `room.joined` domain event。
- `chat.send`：校验 room/content，发布 `message.created` domain event，并回复 `chat.send.ack`。
- `chat.typing`：发布 `message.typing` domain event，并回复 `chat.typing.ack`。
- `presence.sync`：读取 gateway state presence，并推送 `presence.online`。

业务 handler 不直接写 connection。`app/im/infra/realtime` 提供包级业务语义入口，例如 `PushChatMessage(ctx, roomID, payload)`，内部直接复用 `core/realtime` Push API，避免 handler 自己拼 publisher 或 eventbus 中转。

## EventBus 边界

`pkg/realtime/publisher/eventbus` 把 realtime event 编码为 `core/eventbus.Event` payload。eventbus 只识别 topic、payload、headers、publish、subscribe，不解释 user、room、client、gateway 或 transport。

`core/realtime/facade` 不读取 `core/config.V`，也不假设业务项目的 eventbus 配置结构。启动层必须通过 `WithConfig` / `WithConfigLoader` 显式传入 realtime publisher 配置，或通过 `WithService` 注入已构造服务。

## 测试约束

新增或调整测试放在：

```txt
core/realtime/tests/
app/realtime/tests/
tests/realtime/
tests/eventbus/
pkg/realtime/*/tests/
```

当前 runtime 覆盖：

- router group/match/pipeline order
- pipeline compile
- middleware next/abort
- runtime reply and client dispatch
- registry connection/user index
- dispatcher user/client target
- publisher eventbus correlation
- transport codec and ws adapter lifecycle
- gateway state connection lifecycle、reconnect、presence、room、heartbeat timeout cleanup
- eventbus headers
- runtime pipeline、middleware chain、route match、dispatcher fanout、ws write benchmark
- state presence lookup、room membership、connection registry、heartbeat sweep benchmark
- IM demo join/send/typing/presence/worker push

## 更新记录

- 2026-05-28：Realtime facade 移除 `core/config.V` 隐式配置读取；业务项目必须显式注入 publisher 配置或服务实例。
- 2026-05-14：新增 `app/im` minimal realtime IM demo。业务 action 通过 runtime router 注册，消息先发布 IM domain event，再由 IM consumer 通过 notify infra 推送 realtime room event；补充 `examples/realtime/im-demo.html` 和 `app/realtime/tests/im`。
- 2026-05-16：新增 `core/realtime/facade` runtime capability 和 `core/realtime/binding.go`，实时推送统一通过 `realtime` capability 初始化，服务侧只保留 `infra/realtime` adapter。
- 2026-05-14：第六阶段第一步 gateway state runtime。新增 `app/realtime/internal/state/{connection,presence,room,session,heartbeat}`；registry 收敛为 connection index；WS/SSE 通过 lifecycle 触发 connect/disconnect/activity；disconnect cleanup 统一由 state runtime 处理；status 增加 heartbeat timeout、reconnect 和 dispatch latency 指标。
- 2026-05-14：第五阶段第二步 runtime finalize。`Route` 增加 middleware、handler 和 compiled handler；Router 在注册阶段编译 pipeline；Runtime 改为 `Match()` 后执行 `Route.Compiled`；`ActionContext.Next()/Abort()` 成为 middleware runtime state 的唯一推进入口；补充 runtime benchmark。
- 2026-05-14：第五阶段第一步服务统一。删除独立 realtime service 入口，`app/realtime` 成为唯一 gateway；服务层 transport 拆到 `app/realtime/transports/ws`、`app/realtime/transports/sse`；旧 WebSocket demo 和 Redis EventBus 集成测试迁移到 `app/realtime/tests`。
- 2026-05-14：第四阶段服务层迁移。`app/realtime` 改为服务私有 registry + room index + `pkg/realtime/gateway.Runtime` 接线；WebSocket 上行只解码 action、构造 `ActionContext` 并调用 runtime；SSE 接入改为 registry + local dispatcher；worker realtime demo 和 SSE publisher user id 改为 string。
- 2026-05-14：第三阶段 breaking refactor。删除 `core/realtime/manager.go`、旧 action router dispatch、core direct action response helper 和 core codec 文件；建立 gateway runtime、pure router、middleware pipeline、memory registry、local dispatcher、eventbus publisher 和 transport codec/helper 文件。
