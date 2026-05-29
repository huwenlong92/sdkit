# Security 使用指南

## 初始化

测试或单进程场景可使用内存状态：

```go
store := state.NewMemoryStore()
writer := audit.NewMemoryWriter()
manager := risk.NewManager(writer,
    checkers.NewLoginFailChecker(store),
    checkers.NewSMSChecker(store),
)
```

生产环境建议使用 Redis：

```go
store := state.NewRedisStore(redisClient)
```

安全事件可写入 DB：

```go
writer := audit.NewGormWriter(database.DB)
```

## 登录防爆破

```go
result, err := manager.Check(ctx, &risk.Context{
    Scene: risk.SceneLogin,
    UID:   userID,
    IP:    ip,
    Extra: map[string]any{"event": "login_failed"},
})
if err != nil {
    return err
}
if result.Blocked {
    // 拒绝登录
}
if result.NeedCaptcha {
    // 要求验证码
}
```

默认规则：UID 15 分钟失败 5 次要求验证码，10 次封禁 UID 30 分钟；IP 15 分钟失败 20 次封禁 IP 30 分钟。

## 短信验证码防轰炸

```go
result, err := manager.Check(ctx, &risk.Context{
    Scene: risk.SceneSMS,
    IP:    ip,
    Phone: phone,
    Extra: map[string]any{
        "event": "sms_send",
        "code":  "123456",
    },
})
```

默认规则：同手机号 60 秒一次，每天最多 20 次，同 IP 每小时最多 50 次，验证码 TTL 5 分钟。

校验验证码：

```go
result, err := manager.Check(ctx, &risk.Context{
    Scene: risk.SceneSMS,
    Phone: phone,
    Extra: map[string]any{"event": "sms_verify", "code": inputCode},
})
```

## OpenAPI 签名

签名内容：

```txt
METHOD + "\n" + path + "\n" + timestamp + "\n" + nonce + "\n" + canonicalBody
```

签名生成：

```go
sig := crypto.SignHMACSHA256(secret, "POST", "/openapi/order/create", ts, nonce, body)
```

Gin 中间件：

```go
r.POST("/openapi/order/create",
    securitymw.Signature(store, secret),
    handler,
)
```

请求头：

```txt
U-Timestamp: 1710000000
U-Nonce: random-nonce
U-Signature: hmac-sha256-hex
Authorization: Bearer token
```

## 风控中间件

```go
r.POST("/login",
    securitymw.Risk(risk.SceneLogin, manager),
    loginHandler,
)
```

中间件会从请求提取 IP、UA、DeviceID、Method、Path，并根据 `Result` 返回统一 response 结构。

如果场景需要额外业务信息，使用 `RiskWithContext` 注入。比如登录失败计数需要业务侧告诉风控当前事件是 `login_failed`：

```go
r.POST("/login/fail-demo",
    securitymw.RiskWithContext(risk.SceneLogin, manager, func(c *gin.Context, rc *risk.Context) {
        rc.UID = 1001
        rc.IP = c.ClientIP()
        rc.Extra["event"] = "login_failed"
    }),
    handler,
)
```

生产接口建议由 middleware 统一处理拦截和响应；handler 只负责业务成功路径。对于“登录密码校验失败”这类只有 handler 才知道的结果，可以在 handler 确认失败后直接调用 `manager.Check`，或拆成失败回调后再复用相同 checker。

## 可配置风控 risk2

`risk2` 用于应用侧后台可配置风控。core 只提供引擎和接口，应用实现 Store：

```go
engine := risk2.NewEngine(store)
decision, err := engine.Evaluate(ctx, risk2.Event{
    Service: "admin",
    Scene:   "login",
    Event:   "login_failed",
    IP:      "127.0.0.1",
    Extra: map[string]any{
        "account": "admin",
    },
})
```

Gin 入口可以使用 `core/gin/security/risk2`：

```go
decision, err := riskgin.Evaluate(c, engine, risk2.Event{
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
    riskgin.Middleware(engine, risk2.Event{
        Service: "api",
        Scene:   "order",
        Event:   "create",
    }, riskgin.WithResponder(appResponder)),
    handler,
)
```

Store 实现负责读取场景、黑白名单、频率规则并写入事件与命中记录。Gorm、业务表和 SQL 不进入 core。

名单和频率规则可以携带前台处置配置：

```go
risk2.FrequencyRule{
    Code:            "login_failed_account_limit",
    Event:           "login_failed",
    TargetType:      "account",
    WindowSeconds:   300,
    LimitCount:      5,
    Action:          risk2.ActionLimit,
    Score:           80,
    ResponseCode:    410101,
    ResponseMessage: "账号登录失败次数过多，请 5 分钟后再试",
    ResponseAction:  risk2.ResponseActionRetryLater,
    ResponsePayload: map[string]any{"retry_after": 300},
    HTTPStatus:      429,
}
```

一次命中多个规则时，Engine 会按 `action` 严重程度选择主规则；`action` 一样时取 `score` 更高的规则；再一样时保留先命中规则。最终 `Decision` 上的 `response_code`、`response_message`、`response_action`、`response_payload`、`http_status` 来自主规则。

### 配置缓存与频率计数

运行时风控检查通常会读取场景、黑白名单、频率规则，并更新频率统计。推荐分工：

- 配置读取走 `core/cache` 短 TTL，减少每次 check 对 DB 的配置查询。
- 频率统计走 Redis counter，避免按事件日志表做高频 `COUNT`。
- 事件日志和命中记录继续写 DB，作为审计与后台查询数据；高频服务建议用 `risk2.Logger` 异步批量写。
- Redis 不可用时可以不传 counter，Engine 会回退到 Store 的 DB `CountEvents`。

示例：

```go
c := cache.Default()
store := risk2.NewCachedStore(appStore, c, risk2.WithStoreCacheTTL(10*time.Second))

var opts []risk2.Option
if client := redis.Raw(); client != nil {
    opts = append(opts, risk2.WithCounter(risk2.NewRedisCounter(client)))
}
engine := risk2.NewEngine(store, opts...)
```

`NewRedisCounter` 使用 Redis Lua + ZSET 做滑动窗口计数。每个统计维度由 service、scene、event、target_type、target_value、window_seconds 组成，适合类似“同账号 60 秒内登录失败 5 次”这类规则。Redis key 使用 hash tag，让 ZSET key 与序列 key 在 Redis Cluster 中落到同一个 slot。

`NewCachedStore` 缓存的是 Store 查询结果，不改变业务配置来源。后台修改配置后，最迟在 TTL 到期后生效；如果业务需要立即生效，可以由应用层在配置变更后删除对应缓存 key，core 不内置业务侧失效逻辑。

`NewCacheCounter` 仍然存在，但只建议用于测试或单进程临时场景。生产多实例不要用内存 cache 做频率统计，否则不同实例之间的窗口计数不一致。

### 事件日志写入

默认情况下，Engine 会调用 Store 的 `SaveDecision` 同步写入事件日志和命中记录。高频服务可以配置异步 logger：

```go
logger := risk2.NewLogger(appDecisionWriter, risk2.LoggerConfig{
    QueueSize:     2048,
    BatchSize:     100,
    FlushInterval: 200 * time.Millisecond,
})
logger.Start(ctx)

engine := risk2.NewEngine(store,
    risk2.WithCounter(risk2.NewRedisCounter(redisClient)),
    risk2.WithLogger(logger),
)
```

配置 logger 后，请求路径只负责把 `DecisionRecord` 推入内存队列；后台 goroutine 按批次调用应用层 `DecisionWriter.WriteDecisionBatch`。如果 logger 未配置或队列已满，Engine 会同步回退到 Store 的 `SaveDecision`，避免直接丢失审计日志。

注意：

- core 只定义 `DecisionWriter` 接口，不知道业务表结构。
- 异步写入成功前，返回给调用方的 `decision.event_id` 可能为空；如果接口必须立刻返回事件 ID，应使用同步 Store 写入。
- 服务关闭时需要 cancel logger 的 context，让 logger drain 队列并 flush 剩余记录。

触发风控时接口仍返回统一结构。`risk2` 默认使用 `4601/security blocked`，如果规则配置了 `ResponseCode` 或 `ResponseMessage`，Gin 适配会优先使用规则配置：

| err_code | msg | 含义 | 前端建议 |
|---:|---|---|---|
| 4601 | security blocked | 命中封禁或强拒绝 | 停止重试，提示稍后再试或联系管理员 |
| 4602 | captcha required | 需要验证码 | 弹出验证码，完成后重试原操作 |
| 4603 | verify required | 需要二次验证 | 进入短信、邮箱、MFA 等二次验证流程 |
| 4604 | invalid signature | OpenAPI 签名错误或 timestamp 过期 | 开放接口调用方重新生成签名，不建议普通用户重试 |
| 4605 | missing nonce | 缺少 nonce | 调用方补齐 `U-Nonce` |
| 4606 | nonce replay | nonce 重放 | 调用方生成新 nonce 后重试 |
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

## 自定义 Checker

```go
type MyChecker struct{}

func (MyChecker) Name() string { return "my_checker" }

func (MyChecker) Check(ctx context.Context, rc *risk.Context) (*risk.CheckResult, error) {
    if rc.Scene != "my_scene" {
        return nil, nil
    }
    return &risk.CheckResult{Passed: true}, ctx.Err()
}

manager.Register(MyChecker{})
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
    // 调用外部服务校验答案
    return nil
}

captchaManager := captcha.NewManager(Provider{})
```

内置 base64 图片验证码 Provider：

```go
captchaManager := captcha.NewManager(
    captcha.NewBase64Provider(
        captcha.WithBase64Store(captcha.NewStateStore(store, 5*time.Minute)),
    ),
)

challenge, err := captchaManager.Generate(ctx, captcha.KindImage, captcha.GenerateOptions{})
err = captchaManager.Verify(ctx, captcha.KindImage, challenge.ID, inputCode)
```

滑块验证码使用相同接口，后续 Provider 通过 `KindSlider` 返回背景图、滑块图和容错参数等 `Payload`。

## 安全响应头

```go
r.Use(securitymw.SecureHeaders(securitymw.SecureHeaderOption{
    CSP: "default-src 'self'",
}))
```

## 测试命令

```bash
go test ./pkg/security/... ./core/security/...
go test ./...
```

## API Demo

API 服务内置 demo 路由：

```txt
POST /api/v1/security/demo/login-fail
POST /api/v1/security/demo/sms/send
GET  /api/v1/security/demo/openapi/sign
POST /api/v1/security/demo/openapi/check
```

表结构已加入项目迁移入口：

```bash
sdkitgo migrate up security_event security_blacklist security_whitelist security_rule security_device
```
