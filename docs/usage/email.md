# Email 邮件

`core/email` 提供多邮件发送方管理、默认发送方、失败转移和发送 middleware。

## 配置

```yaml
email:
  default: smtp_main
  fallback:
    - smtp_backup
  providers:
    smtp_main:
      driver: smtp
      host: smtp.example.com
      port: 587
      username: ${SMTP_USERNAME}
      password: ${SMTP_PASSWORD}
      from_address: noreply@example.com
      from_name: 系统通知
      reply_to: support@example.com
      encryption: starttls
      timeout: 10s
    smtp_backup:
      driver: smtp
      host: smtp2.example.com
      port: 465
      username: ${SMTP2_USERNAME}
      password: ${SMTP2_PASSWORD}
      from_address: noreply@example.com
      encryption: tls
```

`default` 必须指向 `providers` 中存在的配置。`fallback` 只适合邮件这种内容通用的发送场景：默认发送方失败后，按顺序尝试备用发送方。

## 初始化

```go
import emailcap "github.com/huwenlong92/sdkit/core/email/facade"

if err := emailcap.Use().Register(app); err != nil {
    return err
}
```

全局启动场景可以使用 `WithOptional()`：未配置 `email` 时跳过绑定，配置存在但内容错误时仍返回错误。

已经有配置对象时可以直接传入：

```go
capability := emailcap.Use(emailcap.WithConfig(emailcap.Config{
    Default: "smtp_main",
    Providers: map[string]emailcap.ProviderConfig{
        "smtp_main": {
            Driver:      "smtp",
            Host:        "smtp.example.com",
            Port:        587,
            Username:    "user",
            Password:    "pass",
            FromAddress: "noreply@example.com",
            Encryption:  "starttls",
        },
    },
}))
```

## 发送

```go
result, err := email.Send(ctx, email.Message{
    To:      []string{"user@example.com"},
    Subject: "验证码",
    Text:    "您的验证码是 123456",
})
if err != nil {
    return err
}
_ = result.Provider
```

`Send` 使用默认发送方和 `fallback`。需要指定发送方时：

```go
_, err := email.SendVia(ctx, msg, "smtp_backup")
```

也可以单次指定失败转移顺序：

```go
_, err := email.SendVia(ctx, msg, "smtp_main", "smtp_backup")
```

## Middleware

middleware 运行在发送编排层，可用于限流、审计、开关判断等。middleware 不应打印密码、验证码等敏感内容。

```go
manager, err := email.NewManager(cfg, func(next email.Sender) email.Sender {
    return email.SenderFunc(func(ctx context.Context, req email.Request) (*email.SendResult, error) {
        if disabled {
            return nil, errors.New("email disabled")
        }
        return next.Send(ctx, req)
    })
})
```
