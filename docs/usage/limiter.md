# 限流

基于 `pkg/ratelimit` 的多策略限流库，提供令牌桶、滑动窗口、固定窗口、漏桶、BBR 自适应等限流策略。

## 类型与 API

### Limiter 接口

```go
type Limiter interface {
    Allow(key string) bool
    AllowN(key string, n int) bool
}
```

`TokenBucket`、`SlidingWindow`、`PerKey` 均实现此接口。`BBR` 使用独立接口。

### TokenBucket — 令牌桶

```go
func NewTokenBucket(rate float64, burst int) *TokenBucket
```

每 key 独立令牌桶，基于 `golang.org/x/time/rate`。`rate` 为每秒生成令牌数，`burst` 为桶容量（允许的突发流量）。后台每 5 分钟清理空闲桶（令牌满的 key）。

```go
tb := strategy.NewTokenBucket(100, 200)   // 每 IP 100/s，突发 200
tb.Allow("192.168.1.1")                  // true
tb.Allow("192.168.1.1")                  // true（突发内）
tb.AllowN("192.168.1.1", 50)             // 批量通过 50 个
```

### SlidingWindow — 滑动窗口

```go
func NewSlidingWindow(window time.Duration, limit int) *SlidingWindow
```

按时间窗口精确计数，内部按时间戳切片实现。`window` 内最多 `limit` 个请求。后台每 5 分钟清理过期 key。

```go
sw := strategy.NewSlidingWindow(time.Minute, 60) // 每分钟 60 次
sw.Allow("192.168.1.1")                         // true（第 1 次）
// ... 60 次后
sw.Allow("192.168.1.1")                         // false
```

### PerKey — Per-Key 装饰器

```go
func NewPerKey(inner func() Limiter) *PerKey
```

为每个 key 动态创建独立 `Limiter` 实例。`inner` 为工厂函数，仅在首次遇到新 key 时调用。

```go
pk := ratelimit.NewPerKey(func() ratelimit.Limiter {
    return strategy.NewTokenBucket(50, 100)
})
pk.Allow("user-123") // 为该用户创建独立令牌桶
```

### BBR — CPU 自适应限流

```go
func NewBBR(opts ...BBROption) *BBR
func (l *BBR) Allow() (func(), error)
func (l *BBR) Stop()
```

#### 配置选项

| 选项 | 默认值 | 说明 |
|------|--------|------|
| `WithCPUThreshold(800)` | 800 | CPU 阈值（0-1000），800=80% |
| `WithWindow(1s)` | 1s | maxInFlight 采样间隔 |
| `WithDecay(0.95)` | 0.95 | maxInFlight 衰减因子（0-1） |

#### 算法原理

参考 [hertz-contrib/limiter](https://github.com/hertz-contrib/limiter) 的 Sentinel BBR 算法：

1. 维护 `inFlight`（当前并发请求数）和 `maxInFlight`（过往最大并发数）
2. 定时采样 `inFlight` 更新 `maxInFlight`：超过过往值则快速攀升，低于则按 `decay` 衰减
3. `Allow()` 判断逻辑：
   - **CPU > 阈值** → `inFlight > maxInFlight` 则拒绝
   - **CPU <= 阈值** → 1s 冷却期内继续限制，冷却期过后全放行
4. 返回 `done` 回调，请求处理完成后调用，减少 `inFlight`

```go
bbr := strategy.NewBBR(
    strategy.WithCPUThreshold(800),       // CPU 80% 触发
    strategy.WithWindow(time.Second),     // 采样间隔 1s
    strategy.WithDecay(0.95),             // 衰减因子
)

done, err := bbr.Allow()
if err != nil {
    // 拒绝
}
defer done() // 请求处理完成后调用
```

#### 适用场景

- 全局限流：不按 IP/用户区分，保护整个服务不过载
- 过载保护：CPU 飙升时自动拒绝请求，防止雪崩
- 与 per-IP 限流配合：BBR 在先（保服务不挂），per-IP 在后（防单 IP 滥用）

#### 限制

- **仅 Linux**：BBR 通过 `/proc/stat` 读取 CPU 使用率，macOS/Windows 上 `cpuReader` 无法工作（usage 始终为 0，相当于永不触发限流）。非 Linux 环境需自行注入 mock CPU。

## Gin 中间件

### 按 IP 限流

```go
// 使用 Limiter 接口的任意实现
r.Use(rlMiddleware.Middleware(strategy.NewTokenBucket(100, 200)))
r.Use(rlMiddleware.Middleware(strategy.NewSlidingWindow(time.Minute, 60)))

// 自定义 key（按用户 ID、API Key 等）
r.Use(rlMiddleware.MiddlewareWithKey(
    strategy.NewTokenBucket(50, 100),
    func(c *gin.Context) string {
        return c.GetString("auth_user_id")
    },
))
```

### 按用户限流

从认证中间件注入的用户 ID 限流，每个用户独立限流。需在认证中间件调用 `keyer.SetSubject` 或 `keyer.SetSubjectKey` 之后注册。

```go
// 路由中注册
authorized := r.Group("")
authorized.Use(func(c *gin.Context) {
    keyer.SetSubject(c, "user", userID)
    c.Next()
})
{
    authorized.Use(rlMiddleware.LimiterPerUserNormal()) // 每用户 100/s
    authorized.Use(rlMiddleware.LimiterPerUserStrict()) // 每用户 30/s
    authorized.Use(rlMiddleware.LimiterPerUserWrite())  // 写操作：每用户 10/s

    // 自定义参数
    authorized.Use(rlMiddleware.LimiterPerUser(50, 100))
}
```

### 按用户 + 路由限流

同一用户对不同路由独立计数。需在认证中间件写入 ratelimit subject 之后注册。

```go
// key 格式：subject:user:1001:POST:/admin/v1/users
write := authorized.Group("")
write.Use(rlMiddleware.LimiterPerUserRoute(10, 20)) // 每用户每路由 10/s
{
    write.POST("/users", system.CreateUser)
    write.POST("/users/update", system.UpdateUser)
}
```

### 预设中间件一览

| 预设 | key | 算法 | 参数 |
|------|-----|------|------|
| `LimiterLoose()` | IP | 令牌桶 | 200/s, burst 400 |
| `LimiterNormal()` | IP | 令牌桶 | 100/s, burst 200 |
| `LimiterStrict()` | IP | 令牌桶 | 20/s, burst 50 |
| `LimiterLogin()` | IP | 滑动窗口 | 5/min |
| `LimiterUpload()` | IP | 令牌桶 | 10/s, burst 30 |
| `LimiterPerUserNormal()` | 用户 | 令牌桶 | 100/s, burst 200 |
| `LimiterPerUserStrict()` | 用户 | 令牌桶 | 30/s, burst 60 |
| `LimiterPerUserWrite()` | 用户 | 令牌桶 | 10/s, burst 20 |
| `LimiterPerUserRoute(r,b)` | 用户+路由 | 令牌桶 | 自定义参数 |
| `BBRNormal()` | 全局 | BBR | CPU 80% |
| `BBRSensitive()` | 全局 | BBR | CPU 60% |

### BBR 全局限流

```go
bbr := strategy.NewBBR(
    strategy.WithCPUThreshold(800),
    strategy.WithWindow(time.Second),
)
r.Use(rlMiddleware.BBRMiddleware(bbr))
```

### 自定义中间件

`core/gin/ratelimit/middleware.Limiter(r, burst)` 和 `LimiterPerUser(r, burst)` 是 Gin 接入层预设；纯算法和 Store 位于 `pkg/ratelimit`。

```go
r.Use(rlMiddleware.Limiter(100, 200))
authorized.Use(rlMiddleware.LimiterPerUser(50, 100))
```

## 超限响应

| 限流类型 | HTTP | err_code | msg |
|----------|------|----------|-----|
| TokenBucket / SlidingWindow | 429 | 429 | 请求太频繁，请稍后再试 |
| BBR | 429 | 429 | 服务繁忙，请稍后再试 |

## 配置文件

```yaml
# configs/admin.yaml
admin:
  limiter:
    enabled: true
    rate: 100          # 每秒令牌数
    burst: 200         # 突发上限

# configs/api.yaml
api:
  limiter:
    enabled: true
    rate: 200
    burst: 400

# configs/limiter.yaml
bbr:
  enabled: false     # BBR 进程级过载保护（默认关闭，仅 Linux 有效）
  cpu_threshold: 800 # CPU 阈值 (0-1000)
  window: 1          # maxInFlight 采样间隔（秒）
  decay: 0.95        # 衰减因子
```

配置加载与路由注册：

```go
// server.go
cfg, _ := adminconfig.Load("configs/config.yaml", "admin", bootCfg)
router := SetupRouter(&cfg.Limiter, &cfg.BBR, accessLogger)

// router.go
func SetupRouter(cfg *adminconfig.LimiterConfig, bbrCfg *adminconfig.BBRConfig, accessLogger *accesslog.Logger) *gin.Engine {
    r := gin.New()
    r.Use(appmiddleware.BBR(bbrCfg))
    r.Use(adminmiddleware.RateLimit(cfg))
    // ...
}
```

## 并发安全

所有限流器均安全用于并发环境：

- `TokenBucket`：`sync.RWMutex` 保护桶 map，`x/time/rate.Limiter` 本身线程安全
- `SlidingWindow`：`sync.RWMutex` 保护时间戳切片
- `PerKey`：`sync.RWMutex` 保护限流器 map
- `BBR`：`sync/atomic` 无锁操作 `inFlight`/`maxInFlight`/`prevDrop`

## 性能参考

Apple M1 Max，benchtime=1s：

| 类型 | ns/op |
|------|-------|
| BBR | ~18 |
| SlidingWindow | ~90 |
| TokenBucket | ~94 |

## 选型指南

| 场景 | 推荐策略 |
|------|----------|
| 防单 IP 刷接口 | TokenBucket（简单）或 SlidingWindow（精确） |
| 全局过载保护 | BBR（CPU 自适应） |
| 按用户/租户限流 | PerKey + TokenBucket |
| 两者都要 | BBR 在先（全局）→ TokenBucket 在后（per-IP） |
