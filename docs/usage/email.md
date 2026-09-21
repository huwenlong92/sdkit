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
  templates:
    verify_code:
      subject: 验证码 {{.code}}
      text_file: verify_code.txt
      html_file: verify_code.html
```

`default` 必须指向 `providers` 中存在的配置。`fallback` 只适合邮件这种内容通用的发送场景：默认发送方失败后，按顺序尝试备用发送方。

## 初始化

```go
import (
    "os"

    emailcap "github.com/huwenlong92/sdkit/core/email/facade"
    "github.com/huwenlong92/sdkit/core/runtime"
)

if err := emailcap.Use(
    emailcap.WithConfigLoader(func(app *runtime.App) (emailcap.Config, error) {
        templates, err := emailcap.LoadTemplates(os.DirFS("templates/email"), cfg.Email.Templates)
        if err != nil {
            return emailcap.Config{}, err
        }
        cfg.Email.Templates = templates
        return cfg.Email, nil
    }),
).Register(app); err != nil {
    return err
}
```

facade 不会从 `core/config.V` 隐式读取配置。应用需要通过 `WithConfig` 或 `WithConfigLoader` 显式传入配置。

模板文件只在初始化时读取一次。也可以使用 `embed.FS`：

```go
//go:embed templates/email/*
var emailTemplates embed.FS

templates, err := emailcap.LoadTemplates(emailTemplates, cfg.Email.Templates)
```

全局启动场景可以使用 `WithOptional()`：未显式传入配置时跳过绑定，配置存在但内容错误时仍返回错误。

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

数据库动态账号场景使用常驻动态 manager。Resolver 在每次发送时按稳定账号名读取当前配置：

```go
manager, err := email.NewDynamicManager(
    email.ProviderResolverFunc(func(ctx context.Context, account string) (email.ProviderConfig, error) {
        return accountRepository.ResolveEmailProvider(ctx, account)
    }),
    nil,
)
if err != nil {
    return err
}

_, err = manager.SendVia(ctx, message, accountCode)
```

动态模式不使用隐式默认账号，调用方必须使用 `SendVia`。Resolver 每次读取当前账号配置，provider 在该次发送结束后关闭；SMTP 网络连接不会常驻。需要异步投递时，应用先通过 `core/queue` 入队，再由 Worker 调用动态 manager，`core/email` 本身不负责队列。

## 发送

直接内容邮件：

```go
result, err := email.Send(ctx, email.DirectMessage{
    To:      []string{"user@example.com"},
    Subject: "验证码",
    Text:    "您的验证码是 123456",
})
if err != nil {
    return err
}
_ = result.Provider
_ = result.Result
_ = result.Error
```

带附件时传入可重复打开的 `AttachmentSource`。每个 Provider 尝试都会重新调用
`Open`，成功打开的流由 Provider 关闭：

```go
result, err := email.Send(ctx, email.DirectMessage{
    To:      []string{"user@example.com"},
    Subject: "月度报告",
    HTML:    "<p>报告见附件。</p>",
    Attachments: []email.Attachment{{
        Name:        "report.pdf",
        ContentType: "application/pdf",
        Size:        object.Size,
        Source: email.AttachmentSourceFunc(
            func(ctx context.Context) (io.ReadCloser, error) {
                return objectStorage.Get(ctx, object.Key)
            },
        ),
    }},
})
```

`AttachmentSource` 只约束 `Open(context.Context) (io.ReadCloser, error)`，内容可以来自
OSS、COS、MinIO、本地文件或调用方自己的数据源。SMTP driver 会边读取、边做
Base64/MIME 编码、边写入 SMTP DATA，不会把整封含附件邮件缓存在内存或放进
`ProviderResult.Raw`。如果附件在 SMTP 连接建立前无法打开，返回值可通过
`errors.Is(err, email.ErrAttachmentOpen)` 判断为确定未发送。

固定 HTML 模板邮件：

```go
result, err := email.Send(ctx, email.TemplateMessage{
    To:       []string{"user@example.com"},
    Template: "verify_code",
    Data: map[string]any{
        "code": "123456",
    },
})
if err != nil {
    return err
}
_ = result.Provider
_ = result.Result
_ = result.Error
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
