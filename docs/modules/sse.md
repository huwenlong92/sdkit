# SSE Transport

## 模块目标

SSE 作为 `app/realtime` 的 server push transport 提供用户级多连接、事件订阅过滤、心跳和 EventBus 转发。

## 架构

```text
api / admin / worker / crontab
        |
        | service infra/realtime or core/realtime/facade
        v
core/eventbus
        |
        | memory / redis / redis_stream
        v
app/realtime consumer + delivery
        |
        | app/realtime/transports/sse
        v
browser
```

单进程模式使用 memory EventBus；多进程或多节点模式使用 Redis PubSub 或 Redis Stream。Redis PubSub 下所有 realtime gateway 节点订阅同一 topic；Redis Stream 下每个 gateway 节点使用自己的 consumer group。两种 Redis 模式都只转发给本节点维护的 SSE 连接。

`app/realtime` 是接收方服务。需要向 SSE/WebSocket 推消息的服务统一复用 `core/realtime/facade` 和各服务自己的 `infra/realtime` adapter。

声明 `eventbus` 或 `realtime` 的服务必须显式提供 `eventbus` 配置。缺少 `eventbus` key、`eventbus.driver` 或 `eventbus.topic` 时应启动失败，防止独立部署时消息静默落到本进程 memory bus。

## Event

标准事件结构：

```go
type Event struct {
	Event    string          `json:"event"`
	UID      int64           `json:"uid,omitempty"`
	TenantID int64           `json:"tenant_id,omitempty"`
	Data     json.RawMessage `json:"data"`
	TraceID  string          `json:"trace_id,omitempty"`
	Time     int64           `json:"time"`
}
```

SSE 的 `event:` 使用 `Event.Event`，`data:` 写出完整 JSON。

`TraceID` 只表示 OpenTelemetry trace ID。业务追踪 ID 和请求 ID 不写入该字段，而是通过底层 `eventbus.Event.Headers` 传播，并在 SSE 订阅 handler context 中恢复。

SSE 订阅 eventbus 后，每条事件转发都会运行在 `eventbus.handle <event_name|topic>` span 下。这个 span 表示单条实时事件处理，不等同 `/sse` 长连接 HTTP root span。

## Registry 设计

`app/realtime/internal/registry` 维护两类索引：

- `clients`: `client_id -> client`
- `userClients`: `uid -> client_id -> client`

发送规则：

- `SendToUser` 会投递到同一用户的所有连接
- 只向已订阅该事件的 client 投递
- client channel 满时直接丢弃事件并记录 warn 日志，避免阻塞业务流程
- 连接关闭后由 SSE handler 调用 `Remove` 清理

## Client 设计

每个浏览器 Tab 对应一个 client：

```go
type Client struct {
	ID       string
	UserID   string
	TenantID int64
	Events   map[string]bool
	Ch       chan Event
}
```

## SSE Handler 流程

1. 调用 `Authenticator` 鉴权
2. 解析 `events` 查询参数
3. 按 `allow_events` 过滤订阅事件
4. 创建并注册 client 到 realtime registry
5. 设置 SSE 响应头
6. 写出 connected 注释并 flush
7. 循环处理事件、心跳和连接关闭
8. 退出时清理 client

响应头会设置：

```text
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

## 鉴权设计

默认配置启用 JWT 鉴权，支持两种 token 来源：

- `Authorization: Bearer <token>`
- `/realtime/sse?token=<short_token>`

`token_query` 可配置。跨域场景不建议在 URL 中传长期 access token，应使用短期 token。

## 安全注意事项

- 生产环境必须使用 HTTPS
- `allow_events` 应只开放当前前端需要的事件类型
- 事件 payload 必须由业务层脱敏
- 不要推送密码、token、secret、完整手机号、身份证、支付信息
- `/realtime/status` 返回统一 gateway 状态，生产环境建议放在内网或额外加管理权限

## Nginx

```nginx
location /realtime/sse {
    proxy_pass http://realtime_gateway;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header Connection '';
    proxy_buffering off;
    proxy_cache off;
    gzip off;
    proxy_read_timeout 1h;
    proxy_send_timeout 1h;
}
```

## 后续扩展

- Redis Stream 离线补偿
- 客户端动态 subscribe/unsubscribe
- 管理端连接列表和连接踢出

## 更新记录

- 2026-05-13：SSE eventbus 单事件转发通过 `eventbus.handle <name|topic>` span 表示，避免只依赖长连接 HTTP span。
- 2026-05-13：SSE realtime event 的 `TraceID` 只承载 OpenTelemetry trace ID，`track_id/request_id` 改由 eventbus headers 传播。
- 新增 SSE 连接管理、Publisher、Writer、JWT 鉴权适配和 SSE 服务入口。
