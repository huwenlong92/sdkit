# EventBus 使用

## 当前契约

EventBus 现在只接收已经成型的事件对象，driver 不再负责把业务 payload 和 publish option 拼装成事件：

```go
event, err := eventbus.NewJSONEvent(ctx, "rt:events", payload, map[string]string{
    eventbus.HeaderConnectionID: "conn-1",
})
if err != nil {
    return err
}
err = bus.Publish(ctx, event)
```

订阅返回 `Subscription`，停止订阅时调用 `Close`：

```go
subscription, err := bus.Subscribe(ctx, "rt:events", func(ctx context.Context, ev *eventbus.Event) error {
    var payload map[string]any
    return eventbus.JSONCodec{}.Unmarshal(ev.Payload, &payload)
})
if err != nil {
    return err
}
defer subscription.Close()
```

`Event` 只包含 `ID`、`Topic`、`Headers`、`Payload`、`Timestamp`。业务事件名、用户、房间、action 等语义必须放在 payload 或上层 realtime event 中，不能放进 eventbus 顶层字段。

## 配置示例

本地开发或单进程：

```yaml
eventbus:
  driver: memory
  topic: rt:events
  topic_prefix: sdkitgo:rt
  node_name: local-dev
```

多进程 SSE 广播：

```yaml
eventbus:
  driver: redis
  topic: rt:events
  topic_prefix: sdkitgo:rt
  node_name: sse-01
```

`redis` driver 使用 Redis PubSub，只保证在线广播，不保证离线补偿、ACK、重试和持久化。

已有 NATS 服务时可直接使用 NATS PubSub：

```yaml
eventbus:
  driver: nats
  addr: 192.168.1.126:4222
  topic: rt:events
  subject_prefix: sdkitgo.rt
  node_name: realtime-01
```

`nats` driver 使用普通 NATS PubSub，适合跨进程实时广播；它不使用 JetStream，也不提供 ACK、重试和离线补偿。

显式配置 `driver: redis` 或 `driver: redis_stream` 时，必须先通过 `core/redis/facade.Use` 或手动 Redis 初始化提供 Redis client。Redis client 不存在时初始化会直接返回错误，不会降级为 `memory`。

## 初始化

服务启动层推荐通过 `core/eventbus/facade` 初始化 eventbus：

```go
import eventbuscap "github.com/huwenlong92/sdkit/core/eventbus/facade"

capability, err := eventbuscap.New(eventbuscap.Config{
    Driver:      "memory",
    TopicPrefix: "sdkitgo:rt",
    NodeName:    "local-dev",
})
if err != nil {
    return err
}
defer capability.Close()
```

Redis PubSub / Redis Stream 场景必须显式传入外部 Redis client，capability 不会自行创建连接：

```go
capability, err := eventbuscap.New(eventbuscap.Config{
    Driver:      "redis",
    TopicPrefix: "sdkitgo:rt",
}, eventbuscap.WithRedisClient(redisClient))
```

NATS 场景直接通过配置传入 `Addr`，facade 会创建并托管 NATS 连接：

```go
capability, err := eventbuscap.New(eventbuscap.Config{
    Driver:        "nats",
    Addr:          "192.168.1.126:4222",
    SubjectPrefix: "sdkitgo.rt",
})
```

`eventbuscap.New` 默认会设置 `core/eventbus` default bus。需要临时或局部 bus 时使用 `WithoutDefault()`；需要把外部注入 bus 的生命周期交给 capability 时使用 `WithOwnedBus()`。

runtime app 中使用 `Use` 时，配置必须由启动层显式传入。facade 不读取 `core/config.V`，只会从 `core/redis/facade` 复用已注册的 Redis client：

```go
app.RegisterCapabilities(
    rediscap.Use(rediscap.WithConfig(redisCfg)),
    eventbuscap.Use(eventbuscap.WithConfig(eventbusCfg)),
)
```

如果配置来自业务项目自己的文件结构，在业务侧用 `WithConfigLoader` 读取并映射为 `eventbuscap.Config`。

低层手动初始化仍可用于测试或特殊装配：

```go
import eventbusmemory "github.com/huwenlong92/sdkit/pkg/eventbus/memory"

bus := eventbusmemory.New()
eventbus.SetDefaultWithDriver(bus, "memory")
defer eventbus.CloseDefault()
```

Redis PubSub：

```go
import (
    coreredis "github.com/huwenlong92/sdkit/core/redis"
    eventbusredis "github.com/huwenlong92/sdkit/pkg/eventbus/redis"
)

bus := eventbusredis.New(coreredis.RDB, "sdkitgo:rt")
eventbus.SetDefaultWithDriver(bus, "redis")
```

服务内优先通过 capability 或构造函数注入 `eventbus.Bus` 或更窄的自定义接口，避免业务代码直接依赖具体 driver。

## 职责边界

`core/eventbus` 是 transport-free 的事件发布/订阅标准层，只负责事件模型、headers 透传、handler middleware 和默认 bus 入口。

它不负责：

- websocket / SSE / mqtt 连接管理
- room、online presence、connection push
- HTTP handler、gin router 或 gateway server
- Redis PubSub / Redis Stream / NATS PubSub 具体实现细节

具体 driver 在 `pkg/eventbus/*`。websocket、SSE、mqtt 等实时传输后续由 `pkg/realtime/*` adapter 和 `app/realtime` gateway 承接，eventbus 只把事件交给订阅者。

`core/eventbus/facade` 属于运行时 facade，只负责选择 driver、设置 default 和管理生命周期。它不解释业务 topic，不自行创建 Redis client，也不直接处理 websocket/SSE/mqtt 连接。`core/eventbus` 根包不提供 `Use`，只保留事件契约、默认实例和 `From` / `Bind` 原语。根包的 `Key/From/Bind` 约定统一放在 `binding.go`；真正的 runtime `Use` 只在 `core/eventbus/facade/use.go`。

## Publish

```go
event, err := eventbus.NewJSONEvent(ctx, "sse.user.1001", map[string]any{
	"type": "task.updated",
	"data": map[string]any{"status": "done"},
}, nil)
if err == nil {
	err = eventbus.Publish(ctx, event)
}
```

如果已经有编码后的 payload，可以直接发布 bytes：

```go
payload := []byte(`{"event":"notify","uid":1001,"data":{"title":"hello"}}`)
event, err := eventbus.NewEvent(ctx, "rt:events", payload, nil)
if err == nil {
	err = eventbus.Publish(ctx, event)
}
```

`NewEvent` / `NewJSONEvent` 会自动从 `ctx` 写入 `traceparent`、`X-Track-ID`、`X-Request-ID` 等 headers。不要把业务 `track_id` 或 `request_id` 放进 eventbus 顶层字段；关联信息统一走 `Headers`。

EventFlow 当前识别的基础关联 headers：

```text
traceparent
tracestate
baggage
trace_id
span_id
X-Track-ID
X-Request-ID
connection_id
session_id
```

其中 `trace_id` / `span_id` 来自 `core/tracing` 和 OpenTelemetry span context，`track_id` 来自 `core/tracking` 的 `X-Track-ID`，`request_id` 来自 `core/requestid` 的 `X-Request-ID`。eventbus 只负责保存和转发这些 headers，不重新定义字段语义。

`connection_id` 和 `session_id` 只作为 realtime gateway 的 EventFlow headers 透传。不要把它们加到 `eventbus.Event` 顶层字段，也不要让 eventbus 解释连接、房间或在线状态。

业务向实时通道推送时优先使用 `core/realtime/facade` 或 `core/realtime` Push API；服务侧需要额外业务逻辑时只在本服务 `infra/realtime` adapter 中包一层：

```go
err := realtime.PushUser(ctx, "1001", "notify", map[string]any{
	"title": "hello",
})
```

## Subscribe

```go
subscription, err := bus.Subscribe(ctx, "sse.user.1001", func(ctx context.Context, ev *eventbus.Event) error {
	var payload map[string]any
	if err := eventbus.JSONCodec{}.Unmarshal(ev.Payload, &payload); err != nil {
		return err
	}
	return nil
})
if err != nil {
	return err
}
defer subscription.Close()
```

订阅 handler 统一接收 `*eventbus.Event`，不再提供只接收 `topic` 和 `payload` 的订阅入口。handler context 会从 `Event.Headers` 恢复 trace、track 和 request 信息，日志可以继续使用 `logger.WithContext(ctx, log)`。

默认 memory、redis、redis_stream、nats driver 都会为单次 handler 调用创建 `eventbus.handle <event_name|topic>` span。handler 返回 error 或 panic 时，该 span 会记录错误；panic 仍由 Recover middleware 处理。

## SSE 接入

SSE 服务启动时按配置创建 memory 或 redis bus，并订阅配置中的 `eventbus.topic`。收到事件后由服务侧或后续 realtime gateway 转成具体 transport 推送；eventbus 本身不维护 SSE 连接：

```go
_, err := bus.Subscribe(ctx, topic, func(ctx context.Context, ev *eventbus.Event) error {
	var msg realtime.Event
	if err := json.Unmarshal(ev.Payload, &msg); err != nil {
		return nil
	}
	return dispatcher.DispatchLocal(ctx, &msg)
})
```

业务代码不要直接调用 SSE manager 发送消息，统一通过 eventbus/realtime publisher 发布事件。websocket、SSE、mqtt 的连接、鉴权、路由和写协议逻辑不属于 `core/eventbus`。

## Topic 命名

推荐：

```text
sse.user.{user_id}
sse.tenant.{tenant_id}
sse.room.{room_id}
sse.broadcast
user.created
task.updated
crontab.finished
```

规则：

- 小写
- 使用 `.` 分隔
- SSE 专属 topic 以 `sse.` 开头
- 业务 domain event 不要以 `sse.` 开头

当前项目的 SSE 默认 topic 是 `rt:events`。

## 新增 Driver

新增 driver 需要实现 `Bus` / `Service` 契约：

```go
type Service interface {
	Publish(ctx context.Context, event *Event) error
	Subscribe(ctx context.Context, topic string, handler Handler) (Subscription, error)
	Close() error
	Capability() Capability
}

type Bus = Service
```

要求：

- `Publish` 不能吞掉错误
- `Subscribe` 必须响应 context 取消并返回 `unsubscribe`
- `Close` 必须释放连接和 goroutine
- 不支持的能力必须返回 `ErrUnsupported`
- 不要在业务 handler 中直接依赖具体 driver
