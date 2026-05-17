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

触发风控时接口仍返回统一结构，前端应优先按 `err_code` 分支处理：

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
  "err_code": 4602,
  "msg": "captcha required",
  "data": {
    "Passed": false,
    "Score": 50,
    "Actions": ["captcha"],
    "Reasons": [],
    "NeedCaptcha": true,
    "NeedVerify": false,
    "Blocked": false
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

## 自定义 Captcha Provider

```go
type Provider struct{}

func (Provider) Name() string { return "turnstile" }
func (Provider) Verify(ctx context.Context, token string, ip string) error {
    // 调用外部服务校验 token
    return nil
}

captchaManager := captcha.NewManager(Provider{})
```

## 安全响应头

```go
r.Use(securitymw.SecureHeaders(securitymw.SecureHeaderOption{
    CSP: "default-src 'self'",
}))
```

## 测试命令

```bash
go test ./core/security/...
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
