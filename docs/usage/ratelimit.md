# RateLimit 限流

`pkg/ratelimit` 用于通用限流算法和存储，`core/ratelimit` 用于 HTTP 接口限流接入，当前主要通过 Gin 中间件使用。常用维度包括 IP、用户、用户 + 路由；常用策略包括令牌桶、滑动窗口、固定窗口、漏桶和 BBR 自适应限流。

## 引入

项目内置，无需额外安装。按需选择内存或 Redis 存储后端。

Redis 客户端说明见 [redis.md](redis.md)；常规服务经 bootstrap 初始化后会自动复用。

Runtime 接入门面放在 `core/ratelimit/facade`：

```go
import ratelimitcap "github.com/huwenlong92/sdkit/core/ratelimit/facade"

app := runtime.New()
app.RegisterCapabilities(
    ratelimitcap.Use(),
)
```

bootstrap 会通过 `ratelimitcap.Use()` 注册公共 ratelimit store。Redis 已初始化时使用 RedisStore，否则使用 MemoryStore。业务中间件仍然直接使用根包下的 `core/ratelimit/middleware`。

`ratelimitcap.Use()` 默认是内部底座能力。只有需要把 ratelimit capability 展示给外部启动信息或 CLI 时，才传入 `ratelimitcap.WithExternal()`。facade 只初始化共享 store；限流 rate、burst、BBR 等策略由具体 middleware 或业务配置控制。

```go
import rlMiddleware "github.com/huwenlong92/sdkit/core/ratelimit/middleware"
```

## 快速使用

全局按 IP 限流：

```go
r := gin.New()
r.Use(rlMiddleware.LimiterNormal())
```

登录接口防暴力破解：

```go
r.POST("/login", rlMiddleware.LimiterLogin(), loginHandler)
```

写接口按用户限流：

```go
authorized := r.Group("/api")
authorized.Use(func(c *gin.Context) {
    // 从你的认证中间件结果中取 subject 信息。
    keyer.SetSubject(c, "user", userID)
    c.Next()
})
authorized.Use(rlMiddleware.LimiterPerUserWrite())
```

## 中间件预设

```go
// === Per-IP ===
r.Use(rlMiddleware.Limiter(rate, burst))      // 令牌桶，自定义参数
r.Use(rlMiddleware.LimiterLoose())             // 200/s 突发 400
r.Use(rlMiddleware.LimiterNormal())            // 100/s 突发 200
r.Use(rlMiddleware.LimiterStrict())            // 20/s 突发 50
r.Use(rlMiddleware.LimiterLogin())             // 每分钟 5 次（防暴力破解）
r.Use(rlMiddleware.LimiterUpload())            // 10/s 突发 30
r.Use(rlMiddleware.LimiterLeaky(rate, cap))    // 漏桶

// === Per-User（须在认证中间件写入 keyer.SetSubject 后注册） ===
authorized.Use(rlMiddleware.LimiterPerUser(rate, burst))
authorized.Use(rlMiddleware.LimiterPerUserNormal())   // 100/s 突发 200
authorized.Use(rlMiddleware.LimiterPerUserStrict())   // 30/s 突发 60
authorized.Use(rlMiddleware.LimiterPerUserWrite())    // 10/s 突发 20

// === Per-User+Route ===
write.Use(rlMiddleware.LimiterPerUserRoute(rate, burst))

// === BBR 自适应（仅 Linux） ===
r.Use(rlMiddleware.BBRNormal())       // CPU 80% 触发
r.Use(rlMiddleware.BBRSensitive())    // CPU 60% 触发
```

## 自定义策略

```go
import (
    "time"

    rlMiddleware "github.com/huwenlong92/sdkit/core/ratelimit/middleware"
    "github.com/huwenlong92/sdkit/pkg/ratelimit/strategy"
)

// 令牌桶
tb := strategy.NewTokenBucket(100, 200)
r.Use(rlMiddleware.Middleware(tb))

// 滑动窗口
sw := strategy.NewSlidingWindow(time.Minute, 100)
r.Use(rlMiddleware.Middleware(sw))

// 固定窗口
fw := strategy.NewFixedWindow(time.Second, 50)
r.Use(rlMiddleware.Middleware(fw))

// 漏桶
lb := strategy.NewLeakyBucket(100, 50)
r.Use(rlMiddleware.Middleware(lb))

// 自定义 key
r.Use(rlMiddleware.MiddlewareWithKey(tb, func(c *gin.Context) string {
    return c.GetHeader("X-API-Key")
}))
```

普通策略实现同一个接口：

```go
type Limiter interface {
    Allow(key string) bool
    AllowN(key string, n int) bool
}
```

## Redis 存储

默认使用内存存储。单实例服务或测试可以直接使用默认配置；多实例服务需要使用 Redis 共享限流状态。

```go
import (
    rlMiddleware "github.com/huwenlong92/sdkit/core/ratelimit/middleware"
    coreredis "github.com/huwenlong92/sdkit/core/redis"
    "github.com/huwenlong92/sdkit/pkg/ratelimit/store"
)

rlMiddleware.SetStore(store.NewRedisStore(coreredis.RDB))
```

需要隔离不同服务或环境的 key 前缀：

```go
rlMiddleware.SetStore(store.NewRedisStoreWithPrefix(coreredis.RDB, "admin:ratelimit:"))
```

需要独立 Redis 客户端时见 [redis.md](redis.md)。Redis 客户端应由外层统一初始化和关闭，`RedisStore.Close()` 不关闭外部传入的客户端。

Gin middleware 会把 `c.Request.Context()` 透传给 RedisStore。tracing 启用后，限流产生的 Redis span 会挂到当前 HTTP trace 下，并带 `track_id/request_id/traceparent`。

## 配置项

`pkg/ratelimit/config.go` 提供基础配置结构，可用于业务配置映射：

```yaml
ratelimit:
  rate: 100
  burst: 200

bbr:
  cpu_threshold: 800
  window: 1
  decay: 0.95
```

字段说明：

| 字段 | 说明 |
|------|------|
| `rate` | 令牌桶每秒生成速率 |
| `burst` | 令牌桶突发容量 |
| `cpu_threshold` | BBR CPU 阈值，0-1000 表示 0%-100% |
| `window` | BBR 采样窗口，可按业务配置转换为 `time.Duration` |
| `decay` | BBR 过往峰值衰减因子 |

## 策略对比

| 策略 | 适用场景 | 特点 |
|------|---------|------|
| TokenBucket | 通用限流 | 允许突发，平滑速率 |
| SlidingWindow | 精确窗口 | 时间窗内精确计数 |
| FixedWindow | 简单限流 | 固定时间窗口，边界毛刺 |
| LeakyBucket | 匀速处理 | 队列无突发，超出容量拒绝 |
| BBR | 自适应限流 | 基于 CPU 负载动态调整 |

## BBR 使用

BBR 适合作为服务过载保护：

```go
r.Use(rlMiddleware.BBRNormal())
```

需要自定义参数时直接使用策略：

```go
bbr := strategy.NewBBR(
    strategy.WithCPUThreshold(700),
    strategy.WithWindow(time.Second),
    strategy.WithDecay(0.95),
)
defer bbr.Stop()

r.Use(rlMiddleware.BBRMiddleware(bbr))
```

BBR 通过 Linux `/proc/stat` 读取 CPU 使用率。非 Linux 环境不建议启用 BBR。

## 注意事项

- 用户维度限流必须注册在鉴权中间件之后。
- 用户维度限流通过 `keyer.SetSubject(c, subjectType, subjectID)` 或 `keyer.SetSubjectKey(c, key)` 接收认证主体。
- 限流 key 使用 `subject:<type>:<id>`，避免不同主体类型 ID 冲突。
- 未取到认证主体时，用户维度限流会放行请求。
- 多实例部署必须使用 Redis 存储，否则每个实例只限制本进程流量。
- 内存存储会启动清理协程，手动创建长期独立 store 时退出前调用 `Close()`。
- 自定义 key 不要直接包含 token、手机号、身份证等敏感信息。
- BBR 是全局过载保护，不承担 IP、用户、登录接口等业务维度限流职责。
