# Realtime 使用

Realtime 当前是 runtime infrastructure，不再是 WebSocket/SSE manager 工具层。

核心路径：

```text
transport
  -> gateway runtime
  -> router match
  -> compiled middleware pipeline
  -> handler
```

业务侧跨节点推送只依赖 publisher；gateway 本地只做 registry lookup 和 local dispatch。

## 初始化 Runtime

```go
registry := memory.NewRegistry()
dispatcher := gateway.NewDispatcher(registry)

router := gateway.NewRouter()
router.Use(AuthMiddleware())

chat := router.Group("chat", ChatMiddleware())
chat.On("send", SendMessage)
chat.On("typing", Typing)

runtime := gateway.NewRuntime(
    gateway.WithRouter(router),
    gateway.WithDispatcher(dispatcher),
)
```

`gateway.Runtime` 负责 action orchestration、router lookup、middleware pipeline 和 handler invoke。

`router.On()` 注册 action 时会直接编译 middleware chain。后续 action 进入 runtime 时只执行 `Router.Match()` 和 `Route.Compiled(ctx)`，不会在每条消息上重新拼接 middleware。

## Gateway State

`app/realtime` 服务启动时会创建 gateway state runtime：

```go
rooms := room.NewMemoryIndex()
registry := registry.New(clientBufferSize)
stateRuntime, _ := state.New(state.Options{
    Registry: registry,
    Rooms:    rooms,
})

runtime := gateway.NewRuntime(
    gateway.WithDispatcher(dispatcher),
    gateway.WithLifecycle(stateRuntime),
)
```

生命周期由 state runtime 统一处理：

- connect：注册 connection、刷新 presence、建立 session、初始化 heartbeat。
- activity：更新 last active、heartbeat 和 presence last seen。
- reconnect：同用户同设备新连接会替换旧 session，并清理旧 connection。
- disconnect：移除 connection、离开 room、更新 presence、删除 heartbeat/session。
- heartbeat timeout：通过 sweep 找到超时连接并复用 disconnect cleanup。

业务 handler 不直接操作 connection cleanup。房间业务只调用 room membership API，presence 和 session 不放进 registry。

## Action Handler

```go
func Ping(c *realtime.ActionContext) error {
    return c.Reply("pong", map[string]any{"ok": true})
}
```

`ActionContext` 提供：

- `Context()` / `SetContext(context.Context)`
- `Bind(any)` / `ShouldBindJSON(any)`
- `Abort()`
- `Next() error`
- `Reply(action string, data any) error`
- `PushUser(userID string, evt *Event) error`
- `PushRoom(roomID string, evt *Event) error`

Handler 不直接写 WebSocket/SSE connection，也不向 client channel 手工写响应；回复和推送统一走 `Gateway`。

## Middleware

middleware 使用统一签名：

```go
func AuthMiddleware() realtime.MiddlewareFunc {
    return func(next realtime.HandlerFunc) realtime.HandlerFunc {
        return func(c *realtime.ActionContext) error {
            if c.UserID() == "" {
                c.Abort()
                return realtime.NewActionError("identity_required", "identity required", realtime.ErrEmptyIdentity)
            }
            return next(c)
        }
    }
}
```

需要后置逻辑时可以直接使用 `ActionContext.Next()`：

```go
func TraceMiddleware() realtime.MiddlewareFunc {
    return func(next realtime.HandlerFunc) realtime.HandlerFunc {
        return func(c *realtime.ActionContext) error {
            err := c.Next()
            return err
        }
    }
}
```

`Abort()` 会终止后续 middleware 和 handler。

## Router 与 Group

```go
router := gateway.NewRouter()
router.Use(GlobalMiddleware())

chat := router.Group("chat", AuthMiddleware())
chat.On("send", SendHandler)

route, ok := router.Match("chat.send")
```

Router 只负责：

- route register
- route lookup
- group route

Router 不做 runtime dispatch，不直接执行 transport codec，也不发布 eventbus。

`Match()` 返回的 route 已带 `Compiled` handler，可用于测试或诊断；业务入口仍应统一调用 `runtime.Handle(ctx)`。

## 本地 Registry

`pkg/realtime/memory` 提供 gateway 进程内 registry、presence 和 room store：

```go
store := memory.NewRegistry()

_ = store.Add(&realtime.Client{
    ID: "client-1",
    Identity: &realtime.Identity{ID: "1001", Type: "user"},
    Ch: make(chan realtime.Event, 64),
})

clients := store.GetUserClients("1001")
```

Registry 契约只包含：

```go
type Registry interface {
    Add(*Client) error
    Remove(clientID string) error
    Get(clientID string) (*Client, bool)
    GetUserClients(userID string) []*Client
    GetRoomClients(roomID string) []*Client
}
```

Registry 不提供 Push、Dispatch、Handle 或 Publish。

在 `app/realtime` 服务内，registry 只作为 connection index。room membership 使用 `app/realtime/internal/state/room`，presence 使用 `app/realtime/internal/state/presence`。

## Dispatcher

`gateway.Dispatcher` 只负责本进程 local dispatch：

```go
_ = runtime.DispatchClient(ctx, "client-1", realtime.NewEvent("notify", payload))
_ = runtime.PushUser(ctx, "1001", realtime.NewEvent("notify", payload))
_ = runtime.PushRoom(ctx, "room-1", realtime.NewEvent("notify", payload))
```

Dispatcher 不发布 eventbus。跨节点发布交给 publisher。

## Publisher

Publisher 只负责向 eventbus publish：

```go
publisher := eventbuspublisher.New(bus, realtime.DefaultTopic)

_ = publisher.PushUser(ctx, "1001", realtime.NewEvent("notify", data))
_ = publisher.PushRoom(ctx, "room-1", realtime.NewEvent("notify", data))
_ = publisher.Broadcast(ctx, realtime.NewEvent("system.notice", data))
```

Publisher 不持有 registry，不查找本地连接，也不写 transport。

包级默认入口仍可用于业务侧发布：

```go
_ = realtime.PushUser(ctx, "1001", "notify", data)
_ = realtime.PushRoom(ctx, "room-1", "notify", data)
_ = realtime.Broadcast(ctx, "system.notice", data)
```

这些入口要求启动层先设置默认 publisher。`sdkitgo serve` 会在启用 admin、worker、crontab 或 realtime 服务时自动注册 `core/eventbus/facade` 和 `core/realtime/facade`；手动组装 runtime 时需要显式加入：

```go
app := bootstrap.New(
    eventbusfacade.Use(),
    realtimefacade.Use(),
)
```

服务业务代码不直接调用 `facade.From(app)`。在 handler/task 中优先使用 `core/realtime` 包级 Push API，或者通过当前服务的 `infra/realtime` adapter 保留服务侧定制逻辑。

## Codec 与 Transport

JSON action codec 已下沉到：

```go
pkg/realtime/transport/codec.go
```

WebSocket adapter 只负责连接、读写、heartbeat、codec 接入和生命周期。收到上行 payload 后由调用方组装 `ActionContext` 并交给 gateway runtime，不在 transport 中维护业务 router 或 middleware。

`app/realtime` 服务层的 transport adapter 位于：

```txt
app/realtime/transports/ws
app/realtime/transports/sse
```

`ws` adapter 只解码 action、构造 `ActionContext` 并调用 `runtime.Handle(ctx)`；SSE adapter 只负责单向事件流写出、心跳和连接生命周期。router、middleware、handler 都在 `app/realtime` 服务层统一注册。

WS/SSE transport 只通过 lifecycle 回调通知：

```txt
on connect
on disconnect
on activity
```

presence、room、session 和 heartbeat cleanup 都由 gateway state runtime 统一执行。

## 用户标识

Runtime 内用户标识统一使用 string：

```go
client := &realtime.Client{
    ID:     "client-1",
    UserID: "1001",
}

identity := realtime.NewUserIdentity("1001", 0)
```

`AuthResult` 在认证边界可从现有整数 subject 转成 `UserID string`。runtime、registry、dispatcher 和 publisher 不再使用整数型 user id。

## 文档记录

- 2026-05-14：第六阶段第一步完成 gateway state runtime。新增 connection、presence、room、session、heartbeat 状态子模块；registry 与 presence/room cleanup 分离；WS/SSE 统一 connect/disconnect/activity lifecycle；status 增加 heartbeat timeout、reconnect、dispatch latency 指标。
- 2026-05-14：第五阶段第二步完成 runtime finalize。route 注册阶段编译 middleware pipeline，runtime 只做 match 和 compiled handler 执行；`ActionContext.Next()/Abort()` 承载 middleware runtime state；补充 runtime pipeline 相关测试和 benchmark。
- 2026-05-14：第五阶段第一步完成服务入口统一。`app/realtime` 成为唯一 realtime gateway 服务，`serve` 不再注册独立 SSE/WebSocket 服务；服务层 transport 拆到 `app/realtime/transports/ws` 和 `app/realtime/transports/sse`，旧服务测试迁移到 realtime gateway 测试。
- 2026-05-14：第四阶段服务层迁移完成。`app/realtime` 和 worker 推送入口不再依赖旧 Manager、旧 action dispatch 或旧整数 UID 字段；服务层统一通过 registry、dispatcher、gateway runtime 和 publisher 接线。
- 2026-05-14：第三阶段 breaking refactor。删除旧 core manager、旧 action router dispatch、core action response helper 和 core JSON codec；建立 `pkg/realtime/gateway` runtime/router/pipeline/dispatcher，`pkg/realtime/memory` registry，`pkg/realtime/transport` codec/heartbeat/recover/tracing/metrics。
