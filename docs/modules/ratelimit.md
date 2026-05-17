# RateLimit 模块方案

## 目标

`core/ratelimit` 提供 HTTP 接口限流能力，统一封装限流策略、限流 key 提取、状态存储和 Gin 中间件适配。

目标能力：

- 支持按 IP、用户、用户 + 路由维度限流
- 支持令牌桶、滑动窗口、固定窗口、漏桶策略
- 支持基于 CPU 负载的 BBR 自适应限流
- 支持内存和 Redis 两类限流状态存储
- 统一 Gin 429 响应格式
- 允许业务通过 `Limiter` 接口接入自定义策略

## 模块边界

`core/ratelimit` 负责：

- 定义通用 `Limiter` 接口
- 提供通用限流策略实现
- 提供限流状态存储接口和内存 / Redis 实现
- 提供常用 Gin 中间件预设
- 提供 IP、用户、用户 + 路由 key 提取函数
- 在限流拒绝时返回统一 429 JSON

`core/ratelimit` 不负责：

- 初始化全局 Redis 客户端
- 解析业务鉴权 token
- 决定具体业务接口的限流阈值
- 记录限流日志或告警
- 对非 Gin 框架做适配

用户维度限流依赖上游鉴权中间件写入 `auth.Identity`。`keyer.User` 通过 `authgin.GetIdentity(c)` 读取 `SubjectType` 和 `SubjectID`，生成 `subject:<type>:<id>` 格式的 key。未取到认证主体时，当前用户维度中间件会直接放行。

## 当前目录

```txt
core/ratelimit/
  config.go
  facade/
    config.go
    client.go
    use.go
    default.go
  limiter.go
  keyer/
    ip.go
    user.go
  middleware/
    gin.go
    middleware.go
    ip.go
    user.go
    bbr.go
  store/
    store.go
    memory_store.go
    redis_store.go
  strategy/
    fixed_window.go
    sliding_window.go
    token_bucket.go
    leaky_bucket.go
    bbr.go
    bbr_cpu.go
    bbr_middleware.go
```

## Runtime Capability

`core/ratelimit` 是限流实现包，Runtime Capability 接入层统一放在 `core/ratelimit/facade`：

```go
import ratelimitcap "github.com/huwenlong92/sdkit/core/ratelimit/facade"

runtimeApp.RegisterCapabilities(
    ratelimitcap.Use(),
)
```

bootstrap 默认注册 `ratelimitcap.Use()`。Redis 已初始化时 facade 会设置共享 RedisStore；Redis 不可用时设置 MemoryStore。业务侧仍通过 `core/ratelimit/middleware` 使用中间件预设。

## 核心接口

```go
type Limiter interface {
    Allow(key string) bool
    AllowN(key string, n int) bool
}
```

所有普通策略实现 `Limiter`，Gin 适配层通过 `Middleware` 或 `MiddlewareWithKey` 注入。

存储接口：

```go
type Store interface {
    Counter(key string, n int, ttl time.Duration) (int, error)
    WindowAdd(key string, ts int64, n int, window time.Duration) (int, error)
    TakeToken(key string, rate float64, burst int) (bool, error)
    Cleanup()
    Close() error
}
```

RedisStore 和 MemoryStore 同时实现 `ContextStore`，Gin middleware 会优先使用 `c.Request.Context()` 调用 context-aware 方法，便于 Redis hook 把限流 Redis span 挂到当前 HTTP trace 下。

策略和存储的关系：

| 策略 | 使用的 Store 方法 | 说明 |
|------|-------------------|------|
| TokenBucket | `TakeToken` | 按 key 维护令牌 |
| SlidingWindow | `WindowAdd` | 按 key 记录窗口内时间戳 |
| FixedWindow | `Counter` | 按 key 在固定窗口内计数 |
| LeakyBucket | 内部内存状态 | 当前实现不使用 Store 读写状态 |

## 中间件方案

通用入口：

```go
middleware.Middleware(limiter)
middleware.MiddlewareWithKey(limiter, keyFn)
middleware.LimiterStrategy(limiter)
```

内置预设：

| 函数 | 限流维度 | 策略 | 默认阈值 |
|------|----------|------|----------|
| `Limiter(rate, burst)` | IP | 令牌桶 | 自定义 |
| `LimiterLoose()` | IP | 令牌桶 | 200/s，突发 400 |
| `LimiterNormal()` | IP | 令牌桶 | 100/s，突发 200 |
| `LimiterStrict()` | IP | 令牌桶 | 20/s，突发 50 |
| `LimiterLogin()` | IP | 滑动窗口 | 每分钟 5 次 |
| `LimiterUpload()` | IP | 令牌桶 | 10/s，突发 30 |
| `LimiterLeaky(rate, capacity)` | IP | 漏桶 | 自定义 |
| `LimiterPerUser(rate, burst)` | 用户 | 令牌桶 | 自定义 |
| `LimiterPerUserNormal()` | 用户 | 令牌桶 | 100/s，突发 200 |
| `LimiterPerUserStrict()` | 用户 | 令牌桶 | 30/s，突发 60 |
| `LimiterPerUserWrite()` | 用户 | 令牌桶 | 10/s，突发 20 |
| `LimiterPerUserRoute(rate, burst)` | 用户 + 路由 | 令牌桶 | 自定义 |
| `BBRNormal()` | 全局并发 | BBR | CPU 80% 触发 |
| `BBRSensitive()` | 全局并发 | BBR | CPU 60% 触发 |

限流拒绝时返回 HTTP 429：

```json
{
  "err_code": 429,
  "msg": "请求太频繁，请稍后再试"
}
```

BBR 拒绝时文案为：

```json
{
  "err_code": 429,
  "msg": "服务繁忙，请稍后再试"
}
```

## 存储方案

默认使用 `MemoryStore`。每个未显式传入 store 的策略会创建独立内存实例，状态只在当前进程内有效。

多实例服务需要使用 Redis：

```go
middleware.SetStore(store.NewRedisStore(redis.RDB))
```

Redis key 默认前缀为 `ratelimit:`。需要区分服务或环境时，使用：

```go
store.NewRedisStoreWithPrefix(redis.RDB, "admin:ratelimit:")
```

内存存储会启动定时清理协程，长期独立创建的 store 应在退出时调用 `Close()`。Redis 存储依赖 Redis key TTL 清理，`Close()` 不关闭外部传入的 Redis 客户端。

## BBR 方案

BBR 通过 `/proc/stat` 采样 CPU 使用率，维护当前 in-flight 请求数和衰减后的过往峰值。当 CPU 超过阈值且当前 in-flight 超过过往峰值时拒绝新请求。

配置项：

| 选项 | 说明 |
|------|------|
| `WithCPUThreshold(threshold)` | CPU 阈值，范围按 0-1000 表示 0%-100%，默认 800 |
| `WithWindow(d)` | in-flight 采样窗口，默认 1s，最小 100ms |
| `WithDecay(decay)` | 过往峰值衰减因子，默认 0.95 |

BBR 依赖 Linux `/proc/stat`。非 Linux 环境无法正确读取 CPU 使用率，不建议启用 BBR 预设。

手动创建 BBR 时，需要在服务退出时调用 `Stop()` 停止内部采样协程。

## 使用约束

- 用户维度限流必须注册在鉴权中间件之后。
- Redis 存储适合多实例共享限流状态，内存存储只适合单实例或测试。
- 登录、验证码、写操作等敏感接口优先使用更严格的限流参数。
- 自定义 key 不应包含敏感原文，可使用业务 ID、用户 ID、路由等稳定低敏字段。
- `AllowN` 当前普通策略内部忽略存储错误，Redis 异常时可能表现为拒绝或计数不准确，关键接口应结合监控发现 Redis 异常。
- BBR 是全局自适应保护，不承担 IP 或用户维度限流职责。

## 更新记录

- 2026-05-16：新增 `core/ratelimit/facade` Runtime Capability 接入层，按 `config.go/client.go/use.go/default.go` 组织，根包保留限流实现和业务 API。
- 2026-05-12：限流 Store 增加 context-aware 调用路径，Redis 限流命令可串联 OpenTelemetry HTTP trace。
- 2026-05-10：补充 RateLimit 模块设计、接口、策略、存储和使用约束。
