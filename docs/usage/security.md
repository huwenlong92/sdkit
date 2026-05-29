# Security 使用指南

## 风控引擎

`core/security/risk` 用于应用侧后台可配置风控。core 只提供引擎和接口，应用实现 Store：

```go
engine := risk.NewEngine(store)
decision, err := engine.Evaluate(ctx, risk.Event{
    Service: "admin",
    Scene:   "login",
    Event:   "login_failed",
    IP:      "127.0.0.1",
    Extra: map[string]any{
        "account": "admin",
    },
})
```

Gin 入口可以使用 `core/gin/security/risk`：

```go
decision, err := riskgin.Evaluate(c, engine, risk.Event{
    Service: "admin",
    Scene:   "login",
    Event:   "login_failed",
})
if err != nil {
    return err
}
if !decision.Passed {
    riskgin.Abort(c, decision, riskgin.WithResponder(appResponder))
    return nil
}
```

前置拦截场景可直接使用 middleware：

```go
r.POST("/api/order/create",
    riskgin.Middleware(engine, risk.Event{
        Service: "api",
        Scene:   "order",
        Event:   "create",
    }, riskgin.WithResponder(appResponder)),
    handler,
)
```

Store 实现负责读取场景、黑白名单、频率规则并写入事件与命中记录。Gorm、业务表和 SQL 不进入 core。

## 规则响应

名单和频率规则可以携带前台处置配置：

```go
risk.FrequencyRule{
    Code:            "login_failed_account_limit",
    Event:           "login_failed",
    TargetType:      "account",
    WindowSeconds:   300,
    LimitCount:      5,
    Action:          risk.ActionLimit,
    Score:           80,
    ResponseCode:    410101,
    ResponseMessage: "账号登录失败次数过多，请 5 分钟后再试",
    ResponseAction:  risk.ResponseActionRetryLater,
    ResponsePayload: map[string]any{"retry_after": 300},
    HTTPStatus:      429,
}
```

一次命中多个规则时，Engine 会按 `action` 严重程度选择主规则；`action` 一样时取 `score` 更高的规则；再一样时保留先命中规则。最终 `Decision` 上的 `response_code`、`response_message`、`response_action`、`response_payload`、`http_status` 来自主规则。

## 配置缓存与频率计数

运行时风控检查通常会读取场景、黑白名单、频率规则，并更新频率统计。推荐分工：

- 配置读取走 `core/cache` 短 TTL，减少每次 check 对 DB 的配置查询。
- 频率统计走 Redis counter，避免按事件日志表做高频 `COUNT`。
- 事件日志和命中记录继续写 DB，作为审计与后台查询数据；高频服务建议用 `risk.Logger` 异步批量写。
- Redis 不可用时可以不传 counter，Engine 会回退到 Store 的 DB `CountEvents`。

示例：

```go
c := cache.Default()
store := riskcache.New(appStore, c, riskcache.WithTTL(10*time.Second))

var opts []risk.Option
if client := redis.Raw(); client != nil {
    opts = append(opts, risk.WithCounter(risk.NewRedisCounter(client)))
}
engine := risk.NewEngine(store, opts...)
```

`NewRedisCounter` 使用 Redis Lua + ZSET 做滑动窗口计数。每个统计维度由 service、scene、event、target_type、target_value、window_seconds 组成，适合类似“同账号 60 秒内登录失败 5 次”这类规则。Redis key 使用 hash tag，让 ZSET key 与序列 key 在 Redis Cluster 中落到同一个 slot。

`risk/cache.New` 缓存的是 Store 查询结果，不改变业务配置来源。后台修改配置后，最迟在 TTL 到期后生效；如果业务需要立即生效，可以由应用层在配置变更后删除对应缓存 key，core 不内置业务侧失效逻辑。

`NewCacheCounter` 仍然存在，但只建议用于测试或单进程临时场景。生产多实例不要用内存 cache 做频率统计，否则不同实例之间的窗口计数不一致。

## 事件日志写入

默认情况下，Engine 会调用 Store 的 `SaveDecision` 同步写入事件日志和命中记录。高频服务可以配置异步 logger：

```go
logger := risk.NewLogger(appDecisionWriter, risk.LoggerConfig{
    QueueSize:     2048,
    BatchSize:     100,
    FlushInterval: 200 * time.Millisecond,
})
logger.Start(ctx)

engine := risk.NewEngine(store,
    risk.WithCounter(risk.NewRedisCounter(redisClient)),
    risk.WithLogger(logger),
)
```

配置 logger 后，请求路径只负责把 `DecisionRecord` 推入内存队列；后台 goroutine 按批次调用应用层 `DecisionWriter.WriteDecisionBatch`。如果 logger 未配置或队列已满，Engine 会同步回退到 Store 的 `SaveDecision`，避免直接丢失审计日志。

注意：

- core 只定义 `DecisionWriter` 接口，不知道业务表结构。
- 异步写入成功前，返回给调用方的 `decision.event_id` 可能为空；如果接口必须立刻返回事件 ID，应使用同步 Store 写入。
- 服务关闭时需要 cancel logger 的 context，让 logger drain 队列并 flush 剩余记录。

## 拦截响应

触发风控时接口仍返回统一结构。`risk` 默认使用 `4601/security blocked`，如果规则配置了 `ResponseCode` 或 `ResponseMessage`，Gin 适配会优先使用规则配置：

| err_code | msg | 含义 | 前端建议 |
|---:|---|---|---|
| 4601 | security blocked | 命中封禁或强拒绝 | 停止重试，提示稍后再试或联系管理员 |
| 4602 | captcha required | 需要验证码 | 弹出验证码，完成后重试原操作 |
| 4603 | verify required | 需要二次验证 | 进入短信、邮箱、MFA 等二次验证流程 |
| 4600 | security check failed | 风控内部错误 | 提示系统繁忙，记录日志排查 |

示例响应：

```json
{
  "err_code": 410101,
  "msg": "账号登录失败次数过多，请 5 分钟后再试",
  "data": {
    "passed": false,
    "service": "admin",
    "scene": "login",
    "event": "login_failed",
    "action": "limit",
    "response_action": "retry_later",
    "response_payload": {
      "retry_after": 300
    },
    "hits": []
  }
}
```

## Captcha Provider

```go
type Provider struct{}

func (Provider) Name() string { return "turnstile" }
func (Provider) Kind() captcha.Kind { return captcha.KindImage }
func (Provider) Generate(ctx context.Context, opts captcha.GenerateOptions) (*captcha.Challenge, error) {
    return &captcha.Challenge{ID: "id", Kind: captcha.KindImage}, nil
}
func (Provider) Verify(ctx context.Context, id string, answer string) error {
    return nil
}

captchaManager := captcha.NewManager(Provider{})
```

内置 base64 图片验证码 Provider，示例中 `captchastore` 为 `core/security/captcha/store` 包：

```go
challengeStore := captchastore.NewMemoryStore()
captchaManager := captcha.NewManager(
    captcha.NewBase64Provider(
        captcha.WithBase64Store(captcha.NewBase64Store(challengeStore, 5*time.Minute)),
    ),
)

challenge, err := captchaManager.Generate(ctx, captcha.KindImage, captcha.GenerateOptions{})
err = captchaManager.Verify(ctx, captcha.KindImage, challenge.ID, inputCode)
```

正式滑块 Provider 会生成带缺口背景图、滑块图，并把正确偏移量写入短期 store：

```go
sliderStore := captchastore.NewPrefixedStore(challengeStore, "captcha:slider:")
captchaManager.Register(captcha.NewSliderProvider(
    sliderStore,
    5*time.Minute,
    captcha.WithSliderSize(320, 160),
    captcha.WithSliderPieceSize(42),
    captcha.WithSliderTolerance(6),
    captcha.WithSliderMaxAttempts(3),
    captcha.WithSliderMinDuration(250*time.Millisecond),
))
```

滑块 challenge 的 `image` 是带缺口背景图，`payload.piece` 是滑块图。校验时 `answer` 使用 JSON：

```json
{"x":123,"y":45,"duration_ms":820}
```

点选 Provider 会生成点选图，并把正确点位顺序写入短期 store：

```go
clickStore := captchastore.NewPrefixedStore(challengeStore, "captcha:click:")
captchaManager.Register(captcha.NewClickProvider(
    clickStore,
    5*time.Minute,
    captcha.WithClickSize(320, 160),
    captcha.WithClickTargets(3),
    captcha.WithClickTolerance(12),
    captcha.WithClickMaxAttempts(3),
))
```

点选 challenge 的 `payload.targets` 是前端需要提示用户依次点击的目标。校验时 `answer` 使用 JSON：

```json
{"points":[{"x":81,"y":44},{"x":142,"y":90},{"x":210,"y":72}]}
```

如果需要使用图片素材，给 Provider 配置 `captcha.NewFileBackgroundSource("resources/captcha/slider")` 或 `captcha.NewFileBackgroundSource("resources/captcha/click")`；未配置时使用内置生成背景。

## 安全响应头

```go
r.Use(securitymw.SecureHeaders(securitymw.SecureHeaderOption{
    CSP: "default-src 'self'",
}))
```

## 测试命令

```bash
go test ./pkg/security/... ./core/security/... ./core/gin/security/...
go test ./...
```
