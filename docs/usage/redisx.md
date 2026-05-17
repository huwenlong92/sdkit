# Redisx Driver

`pkg/redisx` 是 Redis driver/sdk 层，只封装 go-redis client 创建、配置、key helper 和 hook 注入点。

框架初始化、全局 Redis 实例、日志 hook、tracing hook 和生命周期管理都在 `core/redis`，业务使用方式见 [redis.md](redis.md)。

## 使用场景

通常不要在业务代码中直接使用 `pkg/redisx`。只有在 driver、底层组件或需要独立 Redis client 的基础设施代码中使用。

```go
client := redisx.New(redisx.Config{
    Addr:   "127.0.0.1:6379",
    DB:     0,
    Prefix: "sdkitgo",
})
defer client.Close()

if err := client.Ping(ctx); err != nil {
    return err
}

key := client.Key("cache", "a")
```

需要框架日志和 tracing 时，由 `core/redis` 创建 client 并注入 hook。

## 配置项

| 字段 | 说明 |
|------|------|
| `Addr` | Redis 地址 |
| `Username` | Redis 用户名 |
| `Password` | Redis 密码 |
| `DB` | Redis DB |
| `Prefix` | key 前缀 |
| `PoolSize` | 连接池大小 |
| `MinIdleConns` | 最小空闲连接数 |

## 约束

- `pkg/redisx` 不依赖 `core/*`。
- `pkg/redisx` 不维护全局 client。
- `pkg/redisx` 不负责 bootstrap、provider、metrics、logger 或 tracing 语义。
- maintenance notifications 默认关闭，避免不支持该命令的 Redis 服务端记录握手探测 warn。
