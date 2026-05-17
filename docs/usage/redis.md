# Redis 基础设施

`core/redis` 是项目统一的 Redis capability gateway，负责全局 Redis 初始化、关闭、健康检查、日志 hook 和 tracing hook。

业务代码优先使用上层 capability：

- `core/cache`：缓存读写
- `core/session`：会话存储
- `core/ratelimit`：限流状态
- `core/queue`：Asynq 队列

只有确实需要 Redis 基础能力时，才直接使用 `core/redis`。

## 配置

```yaml
redis:
  addr: 127.0.0.1:6379
  username: ""
  password: ""
  db: 0
  prefix: sdkitgo
  pool_size: 20
  min_idle_conns: 5
```

## Runtime Capability 引入

`core/redis` 保留 Redis 实现和业务 API；Runtime 接入门面放在 `core/redis/facade`：

```go
import rediscap "github.com/huwenlong92/sdkit/core/redis/facade"

app := runtime.New()
app.RegisterCapabilities(
    rediscap.Use(rediscap.WithConfig(appCfg.Redis)),
)
```

bootstrap 会在 `BootConfig.Redis=true` 时通过 `core/redis/facade` 注册 Redis 能力。业务代码仍然直接使用根包 `github.com/huwenlong92/sdkit/core/redis`。
根包 `core/redis` 不直接提供 runtime `Use`，runtime 接入统一走 `core/redis/facade`。根包的 `Key/From/Bind` 约定统一放在 `binding.go`；真正的 runtime `Use` 只在 `core/redis/facade/use.go`。

独立入口也可以直接初始化 Redis：

```go
err := redis.Init(ctx, redis.Config{
    Addr:         appCfg.Redis.Addr,
    Username:     appCfg.Redis.Username,
    Password:     appCfg.Redis.Password,
    DB:           appCfg.Redis.DB,
    Prefix:       appCfg.Redis.Prefix,
    PoolSize:     appCfg.Redis.PoolSize,
    MinIdleConns: appCfg.Redis.MinIdleConns,
}, logger.L)
```

声明 Redis 为必需依赖时，初始化失败会直接返回 error，服务不会以半初始化状态继续运行。

## 全局客户端

初始化后，全局客户端在：

```go
redis.Default // *redis.RuntimeClient wrapper
redis.RDB     // *go-redis.Client
```

示例：

```go
val, err := redis.Client(ctx).Get(ctx, "user:1").Result()
if err != nil {
    return err
}
```

Gin handler 中可以直接使用：

```go
rdb := redis.Client(c)
if rdb == nil {
    response.Fail(c, redis.ErrNotInitialized)
    return
}
```

Runtime provider 内读取底层 go-redis client：

```go
rdb := redis.ClientFrom(app)
```

## 独立客户端

需要隔离连接池、不同 DB 或不同 Redis 实例时，可以在 core 或入口层创建独立客户端：

```go
client := redis.New(redis.Config{
    Addr:   "127.0.0.1:6379",
    DB:     1,
    Prefix: "sdkitgo",
}, logger.L)
defer client.Close()

if err := client.Ping(ctx); err != nil {
    return err
}
```

底层 driver 能力位于 `pkg/redisx`，不负责 bootstrap、全局实例或框架级日志链路。

## Pipeline

```go
pipe := redis.Client(ctx).Pipeline()
pipe.Set(ctx, redis.Default.Key("cache", "a"), "1", time.Minute)
pipe.Set(ctx, redis.Default.Key("cache", "b"), "2", time.Minute)
_, err := pipe.Exec(ctx)
```

Redis hook 会记录 pipeline 命令数、命令名、耗时和错误。

## 链路追踪

tracing 启用后，Redis hook 会自动创建 span：

- 普通命令：`redis.<cmd>`
- Pipeline：`redis.pipeline`

Redis span 会写入 `trace_id`、`span_id`、`track_id`、`request_id`、`traceparent`。

业务必须把上游 `ctx` 传给 Redis 命令：

```go
val, err := redis.RDB.Get(ctx, key).Result()
```

## 关闭

进程退出时由命令入口统一关闭：

```go
if err := redis.Close(); err != nil {
    return err
}
```

`cache.Close()` 不关闭 Redis client，Redis 生命周期由 `core/redis` 管理。
