# SSE 使用

## 配置

```yaml
realtime:
  enabled: true
  addr: :8092
  sse_path: /realtime/sse
  heartbeat_interval: 25s
  write_timeout: 10s
  client_buffer_size: 64
  allow_events:
    - task.update
    - notify
    - ai.delta
    - log.line
  auth:
    enabled: true
    token_query: token
    allow_cookie: false

eventbus:
  driver: memory
  topic: rt:events
  topic_prefix: sdkitgo:rt
  node_name: local-dev
```

单进程开发使用 `eventbus.driver=memory`。多进程或多节点部署必须使用 Redis：

```yaml
eventbus:
  driver: redis
  topic: rt:events
  topic_prefix: sdkitgo:rt
  node_name: realtime-01
```

多节点时每个进程配置不同的 `node_name`，例如 `worker-01`、`realtime-01`、`realtime-02`。`/realtime/status` 会返回当前 gateway 节点名称，日志中也会带 `node_name`。

如果需要 Redis Stream，可以改为：

```yaml
eventbus:
  driver: redis_stream
  topic: rt:events
  topic_prefix: sdkitgo:rt
  node_name: realtime-01
  stream_max_len: 10000
```

`redis_stream` 模式下 `node_name` 同时用于 consumer group。多个 realtime gateway 节点必须配置不同 `node_name`，否则消息会在同名节点之间分摊。

## 启动服务

独立启动：

```bash
go run ./cmd/sdkitgo realtime
```

随 Admin、API、Worker 一起启动：

```bash
go run ./cmd/sdkitgo serve
```

默认 SSE 地址：

```text
http://127.0.0.1:8092/realtime/sse
```

## 前端连接

```js
const es = new EventSource('/realtime/sse?events=task.update,notify&token=SHORT_TOKEN')

es.addEventListener('task.update', (e) => {
  const msg = JSON.parse(e.data)
  console.log(msg.data)
})

es.addEventListener('notify', (e) => {
  const msg = JSON.parse(e.data)
  console.log(msg.data)
})
```

如果使用 `Authorization` 头，需要用支持自定义 header 的 SSE polyfill；原生 `EventSource` 不支持自定义 header。

## 后端推送用户事件

服务内推荐通过通用 realtime capability 或 `core/realtime` Push API 推送。Admin/API/Worker/Crontab 可以通过各自的 `infra/realtime` 默认入口做服务侧适配：

```go
publisher, err := adminrealtime.DefaultPublisher()
if err != nil {
    return err
}
err = publisher.PushUser(ctx, "1001", "task.update", map[string]any{
	"task_id": "task_001",
	"status":  "running",
})
```

## 后端广播事件

```go
publisher, err := apirealtime.DefaultPublisher()
if err != nil {
    return err
}
err = publisher.Broadcast(ctx, "notify", map[string]any{
	"title": "system maintenance",
})
```

`core/realtime/facade` 是服务启动层推荐入口；`core/realtime.PushUser` 可作为底层 API 使用。框架服务层应优先通过 capability 接入，便于启动信息、独立部署和后续 CLI 生成保持一致。

实时事件里的 `trace_id` 只表示 OpenTelemetry trace ID。调用方只需要透传 `ctx`，`track_id/request_id` 会通过 eventbus headers 传播，不需要写入事件 payload。

SSE 服务收到 eventbus 事件并转发到本机连接时，会创建 `eventbus.handle <event_name|topic>` span。排查单条推送延迟或错误时，看这个 span；`/sse` HTTP span 只代表长连接生命周期。

旧 SSE publisher bridge 已删除。如果某个服务需要事件前缀、租户补充、脱敏、审计等特殊逻辑，可以在本服务 `infra/realtime` 中包一层，但底层仍走 `core/realtime`。

服务声明了 `eventbus` 或 `realtime` 后，eventbus 必须先完成初始化。配置必须显式写出 `eventbus.driver` 和 `eventbus.topic`；缺少 `configs/eventbus.yaml` 或没有在 `configs/config.yaml.imports` 引入它时，服务加载阶段会直接报错，避免消息误发到进程内 memory bus。

## Service Demo

Admin 直接推送：

```text
POST /admin/v1/sse/push
```

API 直接推送：

```text
POST /api/v1/sse/demo
```

请求体：

```json
{
  "user_id": 1001,
  "event": "notify",
  "data": {
    "title": "hello",
    "content": "pushed from service"
  }
}
```

Crontab 内置了一个默认关闭的本地任务模板：

```text
cron_sse_demo
```

它通过 `crontab/infra/realtime` 推送事件，适合验证定时任务服务也能独立向 SSE 发消息。

## Worker Demo

本仓库保留了一个真实推拉链路 demo：

```text
POST /admin/v1/sse/demo
```

请求体：

```json
{
  "user_id": 1001,
  "title": "integration-demo",
  "steps": 2,
  "delay": "1ms"
}
```

前端订阅：

```js
const es = new EventSource('/realtime/sse?events=task.update,notify&token=SHORT_TOKEN')
```

链路：

```text
Admin HTTP -> queue.Enqueue -> worker 消费任务 -> realtime capability -> EventBus -> app/realtime -> browser
```

本地测试命令：

```bash
GOCACHE=/Users/huwenlong/data/lab/sdkitgo/.cache/go-build go test ./worker/event -run TestHandleRealtimeDemoPublishesSSEEvents -count=1 -v
```

日志文件：

```text
logs/worker-realtime-demo/worker-realtime-demo.log
logs/sse/sse.log
logs/worker-eventbus/worker-eventbus.log
```

真实日志摘录：

```text
=== RUN   TestHandleRealtimeDemoPublishesSSEEvents
SSE event=task.update uid=1001 data={"max_retry":1,"progress":0,"queue":"default","retry_count":0,"status":"started","task_id":"demo-task-001","title":"integration-demo","type":"realtime:demo"}
2026-05-10 17:35:34.299 INFO task/realtime_demo.go:35 Realtime Demo推送进度 {"user_id": 1001, "task_id": "demo-task-001", "status": "started", "progress": 0}
SSE event=task.update uid=1001 data={"max_retry":1,"progress":50,"queue":"default","retry_count":0,"status":"running","task_id":"demo-task-001","title":"integration-demo","type":"realtime:demo"}
2026-05-10 17:35:34.300 INFO task/realtime_demo.go:53 Realtime Demo推送进度 {"user_id": 1001, "task_id": "demo-task-001", "status": "running", "progress": 50}
SSE event=task.update uid=1001 data={"max_retry":1,"progress":100,"queue":"default","retry_count":0,"status":"running","task_id":"demo-task-001","title":"integration-demo","type":"realtime:demo"}
2026-05-10 17:35:34.301 INFO task/realtime_demo.go:53 Realtime Demo推送进度 {"user_id": 1001, "task_id": "demo-task-001", "status": "running", "progress": 100}
SSE event=notify uid=1001 data={"content":"integration-demo","task_id":"demo-task-001","title":"Realtime demo completed"}
2026-05-10 17:35:34.302 INFO task/realtime_demo.go:68 Realtime Demo推送通知 {"user_id": 1001, "task_id": "demo-task-001", "event": "notify"}
2026-05-10 17:35:34.302 INFO task/realtime_demo.go:74 Realtime Demo任务完成 {"user_id": 1001, "task_id": "demo-task-001"}
--- PASS: TestHandleRealtimeDemoPublishesSSEEvents (0.00s)
```

## 状态接口

```text
GET /sse/status
```

响应：

```json
{
  "err_code": 200,
  "msg": "ok",
  "data": {
    "online_count": 1,
    "user_count": 1,
    "eventbus_driver": "memory",
    "node_name": "local-dev"
  }
}
```

## 常见问题

### 连接成功但收不到事件

检查 `events` 参数是否包含事件名，且事件名是否在 `allow_events` 中。

### Nginx 后面收不到实时输出

确认 `proxy_buffering off`、`gzip off`，并设置足够长的 `proxy_read_timeout`。

### 多实例部署如何投递

使用 `eventbus.driver=redis`。每个 SSE 实例都会收到 PubSub 消息，但只会投递给本实例内存里的在线连接。
