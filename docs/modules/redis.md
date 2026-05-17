# Redis 模块方案

## 作用

`core/redis` 是 Redis capability gateway，负责 Redis 实例治理，不实现 Redis 命令细节。

它负责：

- 全局 Redis client 初始化和关闭
- 默认实例管理
- Ping 和 health check
- key prefix wrapper
- logger hook
- tracing hook
- 向 cache、session、ratelimit、eventbus、websocket 等模块提供 Redis client

底层 SDK/driver 位于 `pkg/redisx`：

- `pkg/redisx` 只封装 go-redis client、配置、key helper 和 hook 注入点
- `pkg/redisx` 不依赖 `core/*`
- `pkg/redisx` 不负责 bootstrap、全局变量、日志和 tracing 语义

## 初始化

入口层通过 `redis.Init(ctx, cfg, log)` 初始化全局客户端。初始化会先执行 `Ping`，成功后写入：

```go
redis.Default
redis.RDB
```

重复初始化会关闭已有全局客户端。

## Runtime Capability

`core/redis` 是 Redis 实现包，Runtime Capability 接入层统一放在 `core/redis/facade`：

```text
core/redis/
  binding.go
  client.go
  hook.go
  facade/
    config.go
    client.go
    use.go
    default.go
```

启动时由主 Runtime 注册：

```go
import rediscap "github.com/huwenlong92/sdkit/core/redis/facade"

runtimeApp.RegisterCapabilities(
    rediscap.Use(rediscap.WithConfig(cfg.Redis)),
)
```

bootstrap 使用 `rediscap.WithConfigLoader(...)`，确保配置能力先初始化，再由 `rediscap.Use` 读取最终配置。
根包不实现 runtime `Use`；根包只保留 Redis 实现、全局快捷入口和 `Bind(app, client)` 容器绑定能力，避免与 facade 重复实现 capability 注册。根包的 `KeyRedis`、`From(app)`、`Bind(app, client)` 统一放在 `binding.go`；`Use(...)`、`WithConfig(...)`、生命周期关闭只允许放在 `facade/use.go`。

## 配置项

| 字段 | 说明 |
|------|------|
| `Addr` | Redis 地址 |
| `Username` | Redis 用户名 |
| `Password` | Redis 密码 |
| `DB` | Redis DB |
| `Prefix` | 业务 key 前缀 |
| `PoolSize` | 连接池大小 |
| `MinIdleConns` | 最小空闲连接数 |

## 对外接口

```go
redis.New(cfg, log)
redis.Init(ctx, cfg, log)
redis.Close()
redis.Health(ctx)
redis.Bind(app, client)
redis.Raw()
redis.Client(ctx)
redis.ClientFrom(app)
redis.Default
redis.RDB
client.Ping(ctx)
client.Close()
client.Key(parts...)
```

## Hook

`core/redis.New` 会为底层 `pkg/redisx` client 注入框架 hook：

- 普通命令记录命令名、耗时、错误和 context 链路字段。
- Pipeline 记录命令数、命令名、耗时、错误和 context 链路字段。
- `redis.Nil` 视为正常未命中，不按 warn 记录。
- tracing 启用后，普通命令创建 `redis.<cmd>` span，pipeline 创建 `redis.pipeline` span。
- Redis span 会写入 `trace_id`、`span_id`、`track_id`、`request_id`、`traceparent`。

## 内部约束

- 业务代码优先使用上层 capability，不要散落 Redis 命令。
- API handler 需要直接使用 Redis 时，优先通过 `redis.Client(c)` 获取底层 go-redis client。
- Runtime provider 需要读取 Redis 时，优先通过 `redis.ClientFrom(app)` 或 `redis.From(app)` 明确实例边界。
- Redis client 生命周期归 `core/redis` 或入口层管理。
- context 必须透传到 Redis 命令。
- `pkg/redisx` 禁止依赖 `core/*`。
- Redis 业务入口统一使用 `core/redis`。
- maintenance notifications 由 `pkg/redisx` 显式关闭，避免不支持该命令的 Redis 服务端记录 `CLIENT MAINT_NOTIFICATIONS` warn。

## 更新记录

- 2026-05-16：`core/redis/facade` 作为唯一 Runtime Capability 接入层；根包移除重复的 `Use/UseOption`，保留 `Bind/From/Client/Health` 等 Redis 本体 API；根包运行时绑定原语统一放在 `binding.go`。
- 2026-05-15：包装类型改名为 `RuntimeClient`，新增 `redis.Client(ctx)` 和 `redis.ClientFrom(app)` 作为 API DX 快捷入口。
- 2026-05-13：新增 `core/redis` gateway，Redis 业务入口统一收敛到 `core/redis`；底层 driver 位于 `pkg/redisx`。
- 2026-05-12：Redis hook 增加 OpenTelemetry span，普通命令和 pipeline 都会写入链路追踪字段。
- 2026-05-11：关闭 go-redis maintenance notifications，避免不支持该命令的 Redis 服务端记录 `CLIENT MAINT_NOTIFICATIONS` warn。
