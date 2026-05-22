# EventBus 模块

## 2026-05 契约更新

`core/eventbus` 已切换为纯事件对象契约：

```go
type Event struct {
    ID        string
    Topic     string
    Headers   map[string]string
    Payload   []byte
    Timestamp time.Time
}

type Handler func(context.Context, *Event) error

type Subscription interface {
    Close() error
}

type Bus interface {
    Publish(context.Context, *Event) error
    Subscribe(context.Context, string, Handler) (Subscription, error)
    Close() error
    Capability() Capability
}
```

`Event` 不再包含 `Name`、`TraceID`、`CreatedAt`，也不包含用户、房间、连接或 action 字段。事件关联信息通过 `Headers` 传播；业务路由语义由 payload 或上层 `core/realtime.Event` 承载。`PublishOption` / `SubscribeOption` 保留在代码中作为迁移残留，不再是 driver 主路径。

## 模块目标

`core/eventbus` 是框架级轻量事件广播 capability core，当前主要为 realtime gateway 提供跨进程事件流。业务代码发布事件到 eventbus，gateway consumer 订阅 eventbus 后再分发给本实例内的客户端。

```text
业务代码 -> core/eventbus.Publish(...) -> realtime gateway consumer -> local dispatcher -> transport
```

它用于广播、通知、解耦和实时消息分发，不负责可靠任务执行。

## 分层结构

`core/eventbus` 只保留 capability core：

```txt
core/eventbus/
  eventbus.go
  event.go
  option.go
  codec.go
  middleware.go
  default.go
  binding.go
  manager.go
  errors.go
  facade/
```

具体 driver 位于 `pkg/eventbus`：

```txt
pkg/eventbus/
  memory/
  redis/
  redisstream/
```

依赖方向：

```txt
app/realtime、worker、core/realtime
  ↓
core/eventbus
  ↓
pkg/eventbus/*
```

服务装配层可以通过 `core/eventbus/facade` 创建具体 driver；业务 handler 应继续依赖 `core/eventbus` 或 `core/realtime`。

通用运行时 facade 位于：

```txt
core/eventbus/facade
```

它负责在服务启动时根据框架配置创建或复用 bus、设置默认实例并管理 Close 生命周期。`core/eventbus` 根包只保留事件契约、默认实例、`From(app)` / `Bind(app, bus)` 等最小运行时原语，不提供 `Use(...)`；这些绑定原语统一放在 `binding.go`，`Use(...)` 只允许出现在 `core/eventbus/facade/use.go`。

## Transport / Adapter 边界

`core/eventbus` 是 transport-free、protocol-free 的事件标准层。它只定义事件、发布订阅、headers/correlation、middleware、默认实例和 driver capability。

`core/eventbus` 不承载：

- websocket、SSE、mqtt、grpc stream server
- 连接、房间、在线状态或连接级推送
- gin router、HTTP handler、app/realtime gateway
- Redis PubSub / Redis Stream 具体协议实现

具体 driver 位于 `pkg/eventbus/*`，负责 memory、Redis PubSub、Redis Stream 等事件传输后端。实时 transport adapter 后续位于 `pkg/realtime/*`，负责 websocket/SSE/mqtt 与 `core/realtime` 的适配。`app/realtime` gateway 负责真正运行服务、认证、路由、consumer 和 handler。

依赖方向固定为：

```txt
业务 / service / realtime gateway
  -> core/eventbus
  -> pkg/eventbus/*

app/realtime
  -> core/realtime
  -> pkg/realtime/*
```

`pkg/eventbus/*` 和 `pkg/realtime/*` 不反向依赖 app、worker、crontab 业务包。

## 对外接口

```go
type Service interface {
    Bus
    Close() error
    Capability() Capability
}
```

`Event.Payload` 是编码后的原始 bytes，默认使用 JSON codec。`Publish` 传入 `[]byte` 或 `json.RawMessage` 时会保留原始内容，不会被 JSON 编成 base64。

`Subscribe` 返回 `Subscription`，调用方在不再接收事件时必须执行 `Close`；订阅 context 取消或 bus `Close` 时也会停止接收。

模块提供 `SetDefault`、`SetDefaultWithDriver`、`Default`、`DefaultWithDriver`、`Publish`、`Subscribe`、`CloseDefault`，用于同进程服务共享默认 bus。订阅统一使用 `Handler(ctx, Event)`；业务层仍推荐通过依赖注入或 capability 获取 `Publisher`，避免 publish-only 代码依赖完整 `Bus`。

## Event

```go
type Event struct {
    ID        string            `json:"id"`
    Topic     string            `json:"topic"`
    Payload   []byte            `json:"payload,omitempty"`
    Headers   map[string]string `json:"headers,omitempty"`
    Timestamp time.Time         `json:"timestamp"`
}
```

`Topic` 用于路由，`Headers` 用于透传关联元信息，`Payload` 是编码后的业务载荷。

发布事件时，`core/eventbus` 会从 publish context 写入标准 headers：

- `traceparent` / `tracestate` / `baggage`
- `trace_id`
- `span_id`
- `X-Track-ID`
- `X-Request-ID`
- `connection_id`
- `session_id`

这些字段语义不由 eventbus 自行定义：`trace_id` / `span_id` 来自 `core/tracing` 和 OpenTelemetry span context，`track_id` 来自 `core/tracking` 的 `X-Track-ID`，`request_id` 来自 `core/requestid` 的 `X-Request-ID`。`EventFlowHeaderKeys()` 只返回 eventbus 识别和转发的基础字段清单，实际注入和恢复仍走 `core/tracing` helper。

`connection_id` 和 `session_id` 是 realtime gateway 扩展关联字段，只能放在 `Event.Headers` 中透传。`core/eventbus.Event` 不提供 `ConnectionID`、`SessionID` 顶层字段，避免 eventbus 绑定连接、房间、在线状态等 realtime transport 语义。

订阅 handler 执行前，`ContextWithEvent` 会从 `Headers` 恢复 trace、track 和 request context。eventbus 不再提供顶层 `TraceID` 字段；发布侧应优先依赖 context 和 `Headers`。

默认 driver 会挂载 `eventbus.Tracing()` middleware。每次 handler 处理事件时会创建 `eventbus.handle <event_name|topic>` consumer span，并写入 `trace_id/span_id/track_id/request_id/traceparent` correlation attributes。SSE 订阅 eventbus 转发单条实时事件时，会落到这个单事件 span 下，不再只依赖长连接 HTTP span。

## Driver

### memory

`pkg/eventbus/memory` 适合同进程开发、单元测试和 `sdkitgo serve` 单进程模式。

- 支持同 topic 多 subscriber 广播
- `Publish` 会复制 subscriber 快照后执行 handler，避免锁内执行业务逻辑
- 支持 `Subscription.Close`、context cancel 和 `Close`
- handler panic 会被 recover，不影响 bus
- 支持 `Headers` 和 context correlation
- 默认启用 Recover 和 Tracing middleware
- 不支持持久化、延迟、重试和消费组

### redis

`pkg/eventbus/redis` 基于 `github.com/redis/go-redis/v9` PubSub，适合多进程 SSE 广播。

- `Publish` 使用 `rdb.Publish`
- `Subscribe` 使用 `rdb.Subscribe`
- topic 会统一加 `topic_prefix`
- 订阅 goroutine 受 context、`unsubscribe` 和 `Close` 控制
- 消息体为 JSON 编码的 `eventbus.Event`

Redis PubSub 不提供可靠性保证：订阅者离线会丢消息，没有 ACK、重试、offset 和持久化。这符合 SSE 实时广播场景，但不能作为 queue 使用。

### redis_stream

`pkg/eventbus/redisstream` 是轻量 Redis Stream 广播 driver。它声明 `Persistent` 和 `ConsumerGrp` 能力，但仍不提供完整 MQ 能力，不做 pending 自动重投、死信队列、复杂重试、延迟调度和长期离线补偿。

redis_stream driver 和 memory/redis 一样默认启用 Recover 和 Tracing middleware，保证 SSE 单事件处理 span 一致。

### nats

`pkg/eventbus/nats` 基于 `github.com/nats-io/nats.go` 普通 PubSub，适合已有 NATS 服务的多进程实时广播。

- `Publish` 使用 `Conn.Publish`
- `Subscribe` 使用 `Conn.Subscribe`
- topic 会按 `subject_prefix` 转成 NATS subject
- 订阅受 `Subscription.Close` 和 bus `Close` 控制
- 消息体为 JSON 编码的 `eventbus.Event`
- 默认启用 Recover 和 Tracing middleware

NATS eventbus 不使用 JetStream，不提供持久化、ACK、重试和离线补偿；需要可靠任务时继续使用 queue NATS driver。

## Capability

每个 driver 通过 `Capability()` 声明能力：

```go
type Capability struct {
    Fanout      bool
    Wildcard    bool
    Persistent  bool
    Delay       bool
    Retry       bool
    ConsumerGrp bool
}
```

当前建议：

- memory：`Fanout=true`
- redis PubSub：`Fanout=true`
- nats PubSub：`Fanout=true`
- redis stream：`Persistent=true`、`ConsumerGrp=true`

调用方设置当前 driver 不支持的重能力选项时返回 `ErrUnsupported`。

## Runtime Facade

`core/eventbus/facade` 是通用 EventBus runtime facade：

- `Config.Driver` 支持 `memory`、`redis`、`redis_stream`、`nats`。
- driver 实例来自 `pkg/eventbus/memory`、`pkg/eventbus/redis`、`pkg/eventbus/redisstream`、`pkg/eventbus/nats`。
- `memory` 可直接创建。
- 手动调用 `New` 时，`redis` 和 `redis_stream` 必须通过 `WithRedisClient(*redis.Client)` 显式传入外部 Redis client；facade 不自行创建 Redis 连接。
- `nats` 通过 `Config.Addr` 创建连接，`Config.SubjectPrefix` 控制 subject 前缀；未设置时由 `TopicPrefix` 转换得到。
- runtime 调用 `Use` 时，会优先使用 `UseWithRedisClient` 注入的 client；未注入时从 `core/redis/facade.From(app)` 复用 Redis runtime client。
- `New` 默认设置 `core/eventbus` default bus，且校验已有 default 的 driver 是否与当前配置一致。
- `WithoutDefault()` 可创建不注册 default 的局部 bus。
- `WithBus()` 注入外部 bus，默认不关闭外部 bus；`WithOwnedBus()` 表示把注入 bus 的生命周期交给 capability。
- `Close` 会关闭 capability 自己创建或显式托管的 bus；复用已有 default 时不会关闭外部实例。

该 facade 不 import app、worker、crontab 服务包；服务私有配置必须在服务侧通过 mapper 映射为 facade `Config`。

## 与 Queue 的边界

eventbus 负责：

- 广播
- 通知
- 解耦
- 为实时推送提供事件流

queue 负责：

- 耗时任务
- 可靠执行
- 重试
- 延迟任务
- 唯一任务
- 限流任务
- 死信

不要把可靠任务能力塞进 eventbus。未来可以做 `eventbus -> queue bridge`，例如 `order.paid` 发布后同时触发发票任务和 SSE 通知，但 bridge 不属于当前模块职责。

## 与 Realtime 的边界

eventbus 只承载事件流，不直接维护 realtime 连接、在线状态、room membership、gateway router 或 transport。`core/realtime` 定义 Push API、registry、room、presence、protocol 等实时能力抽象；`pkg/realtime/*` 承接 gateway runtime、dispatcher 和 websocket/SSE/mqtt transport adapter；`app/realtime` 运行统一 realtime gateway。

业务侧不应调用 websocket/SSE manager 直推消息。业务只发布事件或调用 realtime publisher，后续由 realtime gateway consumer 订阅 eventbus 并分发到具体 transport。

## Topic 规范

推荐命名：

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

当前 SSE 默认 topic 使用 `rt:events`。

## 内部约束

- 调用方必须透传 `context.Context`
- Handler 必须快速返回，避免阻塞发布流程
- payload 必须是业务层已脱敏的数据
- `trace_id` 只能表示 OpenTelemetry trace ID，业务追踪 ID 必须使用 `track_id` / `X-Track-ID`
- 新 driver 必须完整保留 `Event.Headers`，不得自行定义 trace/track/request 字段语义
- Redis 不允许暴露到公网
- NATS 不允许暴露到公网
- 新 driver 必须实现 `Close`、`Capability` 和 `unsubscribe`
- 新 driver 应默认启用 `eventbus.Recover` 和 `eventbus.Tracing`，或者在 Subscribe 路径显式提供等价能力
- `pkg/eventbus/*` 不主动读取 `core/redis.RDB`，Redis client 由装配层传入
- `pkg/eventbus/*` 不依赖 `pkg/realtime/*`，也不解释 websocket/SSE/mqtt transport 语义
- 显式配置 `redis` 或 `redis_stream` 时，装配层必须提供已初始化的 Redis client，禁止静默降级为 `memory`
- 服务复用默认 bus 时必须校验默认 bus driver 与当前配置一致，避免配置被已有全局 bus 掩盖
- 通用 EventBus capability 只能做 driver 选择、default 设置和 Close 生命周期管理，不允许承载业务 topic、身份或 realtime transport 语义
- EventBus payload 可以承载 realtime event，但 eventbus driver 不允许感知 `PushUser`、`PushRoom`、gateway、manager 或 transport 细节

## 更新记录

- 2026-05-16：通用 EventBus runtime capability 从原 bootstrap 装配包下沉到 `core/eventbus/facade`；`core/eventbus` 根包只保留 `KeyEventBus`、`From`、`Bind` 等最小原语，统一放在 `binding.go`，避免 root 与 facade 同时实现 `Use`。
- 2026-05-22：新增 `pkg/eventbus/nats` 普通 PubSub driver，eventbus facade 支持 `driver=nats`、`addr` 和 `subject_prefix`。
- 2026-05-14：第三阶段 realtime runtime breaking refactor 后，删除独立 capability 文件，`Capability` 并入核心 bus 契约；driver 能力字段改为 `Fanout`，避免 eventbus 暴露实时推送语义。
- 2026-05-14：第二阶段 realtime runtime 整改后，eventbus 文档补充边界：eventbus 只保存和转发事件，realtime target、gateway router、本地 dispatcher、presence/room 均留在 `core/realtime` 与 `pkg/realtime/*`。
- 2026-05-14：新增通用 eventbus capability 边界说明，明确 driver 来源、Redis client 外部传入、default bus 和 Close 生命周期规则。
- 2026-05-14：补齐 transport-free 边界、driver/adapter 分工和 eventbus 与 realtime 的职责边界说明。
- 2026-05-14：拆出 `Publisher`、`Subscriber`、`Service` 契约，`Bus` 作为组合接口保留，便于 publish-only 调用方依赖更窄接口。
- 2026-05-14：明确 EventFlow 基础 headers 清单，trace/span/request/track 字段继续来源于 tracing、tracking、requestid 既有 helper。
- 2026-05-14：新增 `connection_id`、`session_id` headers 约束，字段只在 `Event.Headers` 透传，不进入 `Event` 顶层结构。
- 2026-05-13：EventBus handler 默认创建 `eventbus.handle <name|topic>` span；redisstream 补齐 middleware 链。
- 2026-05-13：EventBus 新增基于 `Headers` 的 tracing/tracking/request id 传播；`TraceID` 不再写入 `track_id`。
- 2026-05-14：订阅入口统一为 `Handler(ctx, *Event)`，返回 `Subscription.Close()` 生命周期。
- 2026-05-13：显式 Redis EventBus 初始化失败时改为返回错误，不再降级为 memory；默认 bus 增加 driver 元数据。
- 2026-05-13：memory、redis、redisstream driver 从 `core/eventbus/*` 迁移到 `pkg/eventbus/*`。
- EventBus 升级为 `Bus` 接口，新增 `Event`、Codec、Capability、Middleware 和 `Subscription`。
- memory/redis/redisstream driver 适配 `Event` 事件模型。
- SSE 订阅 eventbus 后由上层 dispatcher 转发到本地连接，业务发布统一走 eventbus/realtime publisher。
