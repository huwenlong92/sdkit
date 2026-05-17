# WebSocket 使用说明

WebSocket 由 `app/realtime` 统一提供，默认入口为 `/realtime/ws`。业务服务不持有长连接，只发布 realtime event；真正的连接、用户索引、房间索引保存在 Realtime Gateway 进程内。

## 启动

配置位于 `configs/realtime.yaml`，由 `configs/config.yaml.imports` 引入：

```yaml
realtime:
  enabled: true
  addr: :8092
  websocket_path: /realtime/ws
  write_timeout: 10s
  ping_interval: 25s
  client_buffer_size: 64
```

启动独立网关：

```bash
go run ./cmd/sdkitgo realtime
```

启动全套服务：

```bash
go run ./cmd/sdkitgo serve
```

## 客户端连接

当前 demo 支持通过 query 指定用户 ID：

```txt
ws://127.0.0.1:8092/realtime/ws?uid=42
```

客户端可以在 upgrade 请求中传入标准关联 headers：

```text
traceparent: 00-...
X-Track-ID: track-1
X-Request-ID: request-1
```

服务端会把 `X-Track-ID` 和 `X-Request-ID` 写回 101 response headers；缺失时会自动生成。每条 WebSocket message 的 `request_id` 是消息级 ID，只用于请求/响应配对，不会覆盖 HTTP upgrade 的 `X-Request-ID`。

客户端发送 action 消息：

```json
{
  "action": "room.join",
  "request_id": "join-1",
  "data": {
    "room_id": "demo-room"
  }
}
```

服务端返回：

```json
{
  "event": "room.joined",
  "request_id": "join-1",
  "data": {
    "room_id": "demo-room"
  }
}
```

心跳：

```json
{
  "action": "ping",
  "request_id": "hb-1"
}
```

返回事件为 `pong`。

## API Demo

`api` 服务提供公开 demo 入口：

```http
POST /api/v1/websocket/demo
Content-Type: application/json

{
  "target": "user",
  "user_id": "42",
  "event": "api.demo",
  "data": {
    "message": "from api"
  }
}
```

## Admin Demo

`admin` 服务提供登录后 demo 入口：

```http
POST /admin/v1/websocket/demo
Content-Type: application/json

{
  "target": "room",
  "room_id": "demo-room",
  "event": "admin.demo",
  "data": {
    "message": "from admin"
  }
}
```

## Worker Demo

worker 任务类型为 `websocket:demo`，payload：

```json
{
  "target": "broadcast",
  "event": "worker.demo",
  "data": {
    "message": "from worker"
  }
}
```

业务代码可通过 `taskdef.NewWebSocketDemoTask` 或 `taskdef.EnqueueWebSocketDemo` 投递任务。当前 demo handler 已把 `target=user|room|broadcast` 转换为 `core/realtime.PushUser`、`PushRoom`、`Broadcast`，通过 EventFlow 发布实时事件。

worker 不会启动 WebSocket gateway，也不持有连接。线上独立部署时，需要启动 `sdkitgo realtime`，并保证 worker 与 Realtime Gateway 使用相同 EventBus 配置。

发布侧会自动从 `ctx` 写入 `core/realtime.Event.Headers`，包括 `traceparent`、`X-Track-ID`、`X-Request-ID`。业务代码只需要透传 context，不要在业务 payload 里自定义 `trace_id/track_id/request_id`。

读取消息级 request id 时使用当前 `ActionContext` 或事件字段中的 `RequestID`。它只用于单条 WebSocket action 的请求/响应配对，不覆盖 HTTP upgrade 的 `X-Request-ID`。

## 测试与日志

真实流程测试：

```bash
GOCACHE=/Users/huwenlong/data/lab/sdkitgo/.cache/go-build go test ./app/realtime/tests/gateway -run TestRealtimeGatewayDemoFlowFromAPIAdminWorker -count=1 -v
```

该测试会建立真实 WebSocket 连接，依次验证：

- 客户端 `ping` 得到 `pong`
- 客户端加入房间
- API demo 推送到指定用户
- Admin demo 推送到指定房间
- Worker demo 广播到连接

测试日志写入：

```txt
logs/websocket-flow-test/websocket-flow-test.log
```
