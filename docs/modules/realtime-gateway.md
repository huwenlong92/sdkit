# Realtime Gateway 服务

## 模块目标

`app/realtime` 是 sdkitgo 的统一实时网关服务。它消费 EventBus 中的 realtime event，把事件分发到当前进程内的 WebSocket 或 SSE 客户端。

模块目标：

- 统一接收 `core/realtime.Event`。
- 统一管理在线连接和订阅过滤。
- 统一承载 WebSocket 上行 action pipeline。
- 统一输出 gateway status。
- 让业务服务只依赖 `core/realtime/facade` 或 `core/realtime.Publisher`，不依赖具体 transport。

## 目录职责

```txt
app/realtime/
  config/                 配置加载、默认值、ServiceInfo
  handler/                HTTP handler 与 action 业务 handler
  infra/realtime/         gateway runtime adapter
  internal/consumer/      EventBus consumer
  internal/delivery/      user / room / broadcast 分发
  internal/room/          服务私有 room index
  middleware/             服务私有 action middleware
  tests/                  服务集成测试
  transports/ws/          WebSocket transport adapter
  transports/sse/         SSE transport adapter
  router.go               Gin router 和 action 注册
  server.go               服务组装、启动、关闭
  provider.go             bootstrap provider
```

`handler` 目录承载业务逻辑，例如 `ping`、`room.join`、`room.leave`。WebSocket/SSE 实例化和读写循环放在 `transports/*`，不把 transport 细节混入业务 handler。

## 生命周期

启动顺序：

1. 加载 `realtime`、`eventbus`、`jwt` 等配置。
2. 通过 `core/eventbus/facade` 初始化或复用 EventBus。
3. 创建 `app/realtime/infra/realtime` runtime，包含当前 gateway 的 registry、local dispatcher、gateway runtime 和 room index。
4. 创建 delivery。
5. 创建 EventBus consumer，订阅 `core/realtime.DefaultTopic` 或配置 topic。
6. 创建 Gin router，挂载 WebSocket、SSE、status 路由。
7. 启动 HTTP server。

关闭顺序：

1. 停止 consumer subscription。
2. 关闭 HTTP server。
3. 关闭 gateway runtime，释放 registry、runtime 和 room index。
4. 关闭 EventBus capability。

所有后台循环必须受 context 或 Shutdown 控制，不能留下不可回收 goroutine。

## Router 与 Middleware

HTTP router 使用 Gin，并复用已有通用 middleware：

```go
r.Use(recovery.Middleware())
r.Use(gintracking.Middleware())
r.Use(gintracing.Middleware("realtime"))
r.Use(ginrequestid.Middleware())
r.Use(cors.Middleware())
```

默认路由：

- `GET /realtime/ws`：WebSocket gateway。
- `GET /realtime/sse`：SSE gateway。
- `GET /realtime/status`：gateway status。

WebSocket 上行 action 使用 `pkg/realtime/gateway.Router`，由 gateway runtime 统一匹配和执行 middleware pipeline。注册形式：

```go
router.On("ping", handler.Ping)

roomGroup := router.Group("room", middleware.RequireIdentity())
roomGroup.On("join", handler.JoinRoom)
roomGroup.On("leave", handler.LeaveRoom)
```

`core/realtime` 只定义 action context、router interface、middleware function 和错误模型，不内置身份、tracing、tenant、rate limit 等策略。当前服务自己的 `RequireIdentity` 放在 `app/realtime/middleware`。

## Action Handler

业务 action handler 使用统一形态：

```go
func JoinRoom(c *realtime.Context) error {
    var req struct {
        RoomID string `json:"room_id"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        return c.ActionError("invalid_payload", "invalid room payload", err)
    }
    return c.Control("room.joined", map[string]any{"room_id": req.RoomID})
}
```

约束：

- handler 只处理业务动作，不直接管理 WebSocket 帧协议。
- 参数从 `c.ShouldBindJSON` 绑定。
- 当前连接从 `c.Client` 获取。
- context 通过 `c.Context()` 透传给 DB、Redis、队列或其他调用。
- handler 返回 error，由 WebSocket adapter 统一转为 action error event。

SSE 没有客户端上行 action，不进入 action router。

## EventBus Consumer

consumer 订阅 realtime topic，解析 `core/realtime.Event` 后交给 delivery。

支持目标：

- `target=user`：按 `Target.UserID string` 投递给当前 gateway 内该用户连接。
- `target=room`：按 room index 投递给当前 gateway 内 room 成员。
- `target=broadcast`：投递给当前 gateway 内所有在线连接。
- `target=client`：按 client id 定向投递。

consumer 不实现业务重试、离线消息、会话同步或跨节点 presence。它只负责把已经进入 EventFlow 的 realtime event 分发到当前实例。

## Delivery 与 Room

delivery 只依赖 `core/realtime.Dispatcher`、`core/realtime.Registry` 和服务私有 `room.Index`。

服务私有 registry 负责当前进程内 client registry、user index、在线数和 client channel 索引；gateway dispatcher 只做本进程 local dispatch。

`room.Index` 是 `app/realtime` 服务私有状态，用来记录当前 gateway 内 client 与 room 的关系。它不下沉到 core，避免把 IM、IoT 或其他业务 room 语义固化到 framework。

当前 room 是内存实现，多实例场景下只保证本实例内精确投递。跨节点 room membership 或 presence store 需要单独规划。

## Runtime

服务私有 runtime adapter：

- `core/eventbus/facade`：接入通用 EventBus，用于 gateway 消费 realtime topic。
- `app/realtime/infra/realtime`：创建并暴露当前 gateway 的 registry、dispatcher、runtime 和 room index。

业务发布侧仍使用 framework 的 `core/realtime/facade`。这两类 capability 边界不同：

- `core/realtime/facade`：业务发布事件。
- `app/realtime/infra/realtime`：gateway 服务运行时接线。

## 配置与命令

默认配置在 `configs/realtime.yaml`，服务项在 `configs/services.yaml`。

独立启动命令：

```bash
go run ./cmd/sdkitgo realtime
```

`services.realtime.enabled` 默认开启。`command/serve` 只注册 `realtime` 作为实时网关服务入口。

## 测试覆盖

当前测试覆盖：

- EventBus -> consumer -> delivery -> dispatcher/registry 的 user / room / broadcast 分发。
- WebSocket 连接注册、订阅过滤、`ping`、`room.join`、`room.leave`、关闭回收。
- SSE 连接、heartbeat、事件写出、context cancel 后 remove 和 room cleanup。
- config load、缺失 eventbus、disabled service、provider ServiceInfo capabilities。
- command/serve 和顶层命令注册。
- API/Admin/Worker demo 通过 `core/realtime` 发布后，由 realtime consumer 和 WebSocket transport 完整送达客户端。

## 模块边界

本模块不做：

- 不实现 IM 私聊、离线消息、会话、未读数、召回、seq、sync。
- 不实现 MQTT、gRPC stream 或 IoT adapter。
- 不要求业务 handler 感知 WebSocket/SSE 具体连接协议。
- 不把 identity、tenant、rate limit 等策略内置到 `core/realtime`。

后续 IM、IoT、Notify 等业务服务应依赖 gateway 的发布契约，把业务事件发布到 EventFlow，由 gateway 负责推送到在线客户端。
