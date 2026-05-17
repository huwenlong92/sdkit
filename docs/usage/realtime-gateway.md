# Realtime Gateway 使用

## 定位

`app/realtime` 是统一实时网关服务。业务服务不需要关心最终使用 WebSocket、SSE、MQTT 或其他 transport，统一通过 `core/realtime/facade` 或 `core/realtime.Publisher` 发布事件。

当前已落地的事件流：

```txt
业务服务
  -> core/realtime/facade
  -> core/eventbus topic: rt:events
  -> app/realtime consumer
  -> app/realtime delivery
  -> app/realtime registry + local dispatcher
  -> pkg/realtime/ws | pkg/realtime/sse
  -> client
```

`app/realtime` 只负责网关连接、订阅、分发和状态查询，不承载 IM 会话、离线消息、未读数、IoT 设备协议等业务逻辑。

## 配置

默认配置文件：

```yaml
realtime:
  enabled: true
  addr: :8092
  websocket_path: /realtime/ws
  sse_path: /realtime/sse
  status_path: /realtime/status
  heartbeat_interval: 25s
  write_timeout: 10s
  client_buffer_size: 64
  allow_events:
    - task.update
    - notify
    - ai.delta
    - log.line
  auth:
    enabled: false
    token_query: token
    allow_cookie: false
```

`allow_events` 是客户端可订阅事件白名单。为空表示不限制；非空时，连接参数中的 `events` 只能订阅白名单内事件。

`auth.enabled=false` 时允许匿名连接，WebSocket 上行 action 可以再通过 `app/realtime/middleware` 做 message 级拦截，例如 `room.join` 必须有身份。`auth.enabled=true` 时连接阶段会先做认证。

服务是否随 `serve all` 启动由 `configs/services.yaml` 控制。迁移期默认：

```yaml
services:
  realtime:
    type: realtime
    enabled: true
```

`realtime` 是唯一实时网关服务实例。`serve all` 不再启动独立 SSE 或 WebSocket gateway。

## 启动

独立启动：

```bash
go run ./cmd/sdkitgo realtime
```

随全量服务启动：

```yaml
services:
  realtime:
    type: realtime
    enabled: true
```

全量启动时会按 `configs/services.yaml` 构建 `realtime` 服务，并挂载 WebSocket、SSE 和 status 路由。

## WebSocket 接入

连接示例：

```text
ws://127.0.0.1:8092/realtime/ws?events=notify,task.update&token=xxx
```

WebSocket 上行消息使用 action envelope：

```json
{"action":"ping","request_id":"req-1"}
```

加入房间：

```json
{"action":"room.join","request_id":"join-1","data":{"room_id":"room-1"}}
```

离开房间：

```json
{"action":"room.leave","request_id":"leave-1","data":{"room_id":"room-1"}}
```

当前注册的 action：

- `ping`：返回 `pong` 控制事件。
- `room.join`：加入服务私有 room index，需要已绑定身份。
- `room.leave`：离开服务私有 room index，需要已绑定身份。

transport 只负责 WebSocket 连接、读写和生命周期。action payload 由服务层解码为 `ActionContext` 后交给 `pkg/realtime/gateway.Runtime`，业务逻辑放在 `app/realtime/handler`，策略放在 `app/realtime/middleware`。

handler 返回 `core/realtime.ActionError` 或普通 error 后，由 WebSocket adapter 统一写回 action error event。handler 本身不直接决定错误帧协议。

## SSE 接入

连接示例：

```text
http://127.0.0.1:8092/realtime/sse?events=notify&token=xxx
```

SSE 是单向 server push，不进入 gateway router。它复用连接级认证、事件订阅和服务私有 registry 注册逻辑。

服务会按 `heartbeat_interval` 写出 heartbeat，避免长连接被代理或浏览器静默断开。

## 业务推送

业务侧推荐通过 `core/realtime` 默认 publisher 或注入的 `core/realtime.Publisher` 发布事件：

```go
import "github.com/huwenlong92/sdkit/core/realtime"

err := realtime.PushUser(ctx, "1001", "notify", payload)
if err != nil {
    return err
}

err = realtime.PushRoom(ctx, "room-1", "task.update", payload)
if err != nil {
    return err
}

err = realtime.Broadcast(ctx, "system.notice", payload)
if err != nil {
    return err
}
```

也可以依赖 `core/realtime.Publisher` 接口：

```go
type Service struct {
    realtime realtime.Publisher
}
```

业务代码只表达目标：

- `PushUser`：推给指定用户当前在线连接。
- `PushRoom`：推给当前 gateway 实例内指定 room 成员。
- `Broadcast`：推给当前 gateway 实例内所有在线连接。

业务代码不需要判断客户端当前是 WebSocket 还是 SSE。

## 状态接口

状态接口默认：

```text
GET /realtime/status
```

返回在线连接数、在线用户、room 数量、room 成员数、eventbus driver/topic、node name 等运行信息，不返回 token、Redis 地址、JWT secret 等敏感配置。

## Trace 与 Headers

连接入口使用 Gin middleware 接入 recovery、tracking、tracing、requestid、cors。

业务通过 realtime publisher 发布事件时，EventFlow headers 会进入 eventbus 外层 event，并随 `core/realtime.Event.Headers` 传到 gateway。`trace_id` 只表示 OpenTelemetry trace id；`track_id`、`request_id`、`connection_id`、`session_id` 等通过 headers 传播。

message 级 tracing、tenant、rate limit 等策略不内置在 `core/realtime`，应在 `app/realtime/middleware` 或具体业务服务中实现。

## 当前边界

- room membership 是当前 gateway 进程内内存状态。
- 多实例精确 room 投递、presence store、跨节点在线状态需要后续计划。
- MQTT、gRPC stream、IoT adapter 只保留扩展方向，当前未实现。
- IM 私聊、离线消息、会话、未读数不属于 gateway，应该由独立 IM 服务实现，并通过 realtime capability 触达客户端。
