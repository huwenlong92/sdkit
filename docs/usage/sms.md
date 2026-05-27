# SMS 短信

`core/sms` 提供多短信发送方管理、指定发送、消息级失败转移和发送 middleware。

## Build Tag

短信 driver 按需编译，`core/sms` 不会默认拉入任何短信 SDK。

| driver | build tag |
| --- | --- |
| aliyun | `sdkit_sms_aliyun` |
| feige | `sdkit_sms_feige` |

示例：

```bash
go build -tags sdkit_sms_aliyun ./cmd/server
```

启动接线层需要 import 对应 driver：

```go
import _ "github.com/huwenlong92/sdkit/pkg/sms/driver/aliyun"
```

## 配置

```yaml
sms:
  default: aliyun_main
  providers:
    aliyun_main:
      driver: aliyun
      access_key_id: ${ALIYUN_ACCESS_KEY_ID}
      access_key_secret: ${ALIYUN_ACCESS_KEY_SECRET}
      sign_name: 示例签名
      region_id: cn-hangzhou
    feige_backup:
      driver: feige
      account: ${FEIGE_ACCOUNT}
      password: ${FEIGE_PASSWORD}
      sign_id: ${FEIGE_SIGN_ID}
```

短信不提供全局 `fallback`。不同平台模板需要单独审核，变量规则也不同。只有消息本身明确声明支持哪些 provider 时，才会按声明顺序失败转移。

## 初始化

```go
import (
    "github.com/huwenlong92/sdkit/core/runtime"
    smscap "github.com/huwenlong92/sdkit/core/sms/facade"
)

if err := smscap.Use(
    smscap.WithConfigLoader(func(app *runtime.App) (smscap.Config, error) {
        return cfg.SMS, nil
    }),
).Register(app); err != nil {
    return err
}
```

facade 不会从 `core/config.V` 隐式读取配置。应用需要通过 `WithConfig` 或 `WithConfigLoader` 显式传入配置。

全局启动场景可以使用 `WithOptional()`：未显式传入配置时跳过绑定，配置存在但内容错误时仍返回错误。

## 消息

简单模板消息：

```go
msg := sms.TemplateMessage{
    Template: "SMS_001",
    Data: []sms.Param{
        {Key: "code", Value: "123456"},
    },
}
```

如果这个消息已经在多个平台申请了模板，可以声明 provider 顺序：

```go
msg := sms.TemplateMessage{
    Template: "SMS_001",
    Data: []sms.Param{
        {Key: "code", Value: "123456"},
    },
    ProviderNames: []string{"aliyun_main", "feige_backup"},
}
```

复杂场景可以实现 `Message` 接口，根据 provider 返回不同模板和变量：

```go
type CodeMessage struct {
    Code string
}

func (m CodeMessage) Providers(context.Context) []string {
    return []string{"aliyun_main", "feige_backup"}
}

func (m CodeMessage) Resolve(ctx context.Context, provider sms.ProviderConfig) (sms.Payload, error) {
    switch provider.Name {
    case "aliyun_main":
        return sms.Payload{
            Template: "SMS_001",
            Data: []sms.Param{{Key: "code", Value: m.Code}},
        }, nil
    case "feige_backup":
        return sms.Payload{
            Template: "122949",
            Data: []sms.Param{{Key: "value", Value: m.Code}},
        }, nil
    default:
        return sms.Payload{}, errors.New("unsupported provider")
    }
}
```

## 发送

```go
result, err := sms.Send(ctx, []string{"13800138000"}, msg)
if err != nil {
    return err
}
_ = result.Provider
```

`Send` 的 provider 选择规则：

- 调用方显式 `SendVia` 时，只使用调用方传入的 provider 列表
- 否则如果消息实现了 `Providers(ctx)` 且返回非空，按消息声明的 provider 列表尝试
- 否则只使用 `sms.default`

指定发送方：

```go
_, err := sms.SendVia(ctx, []string{"13800138000"}, msg, "feige_backup")
```

单次指定 fallback：

```go
_, err := sms.SendVia(ctx, []string{"13800138000"}, msg, "aliyun_main", "feige_backup")
```

## Middleware

middleware 可用于短信限频、发送开关、审计等。限频依赖应用注入 `RateLimiter`，core 不绑定 Redis。

```go
manager, err := sms.NewManager(cfg, sms.RateLimitMiddleware(limiter, sms.RateLimitRule{
    Key:    sms.PhoneKey,
    Limit:  3,
    Window: time.Minute,
}))
```
