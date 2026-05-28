# SMS 模块方案

## 目标

`core/sms` 提供统一短信发送入口，支持多个命名 provider、默认 provider、指定发送、消息级失败转移和 middleware。

目标能力：

- 从 `sms` 配置初始化默认 manager
- 支持多个命名 provider
- 默认发送只走 `sms.default`
- 消息可声明支持的 provider 顺序，按顺序失败转移
- 单次调用可指定 provider 列表
- 支持发送 middleware 和限频扩展
- Runtime capability 放在 `core/sms/facade`
- facade 默认作为内部 capability 注册；需要对外展示时显式使用 `WithExternal()`
- facade 不从 `core/config.V` 隐式读取配置，应用必须通过 `WithConfig` 或 `WithConfigLoader` 注入配置
- facade 支持 `WithOptional()`，用于全局启动时在未显式传入配置的环境跳过绑定

## 模块边界

`core/sms` 负责：

- 管理 provider 配置和实例
- 按名称懒加载 provider
- 编排默认发送、指定发送和消息级失败转移
- 统一发送结果和失败结果
- 提供 middleware 扩展点

`pkg/sms` 负责：

- 定义底层 provider 接口、消息 payload 和发送结果
- 管理 driver 注册表
- 实现具体短信 driver，例如阿里云、飞鸽；driver 必须通过 build tag 按需编译

`core/sms` 不负责：

- 业务验证码生成和校验
- 短信日志落库
- 异步队列投递
- 绑定具体 Redis 实现
- 保存短信模板配置到数据库

## 为什么不做全局 fallback

短信模板需要按平台审核，不同 provider 的模板 ID、签名和变量格式可能不同。全局 fallback 容易导致未申请模板的平台被误用。

因此短信 fallback 必须由调用方或消息本身显式声明：

```go
sms.SendVia(ctx, phones, msg, "aliyun_main", "feige_backup")
```

或：

```go
func (m CodeMessage) Providers(context.Context) []string {
    return []string{"aliyun_main", "feige_backup"}
}
```

## 对外 API

```go
func NewManager(cfg Config, middleware ...Middleware) (*Manager, error)
func Send(ctx context.Context, to []string, msg Message) (*SendResult, error)
func SendVia(ctx context.Context, to []string, msg Message, providers ...string) (*SendResult, error)
func Use(name string) (Provider, error)
func Close() error
```

`SendResult.Provider` 和 `SendResult.Result` 记录最终成功的 provider 及结果。`SendResult.Error` 记录最终错误。全部 provider 失败时第二返回值为 `NoProviderAvailableError`，同时返回的 `SendResult` 里也会保留 `Error` 和 `Attempts`。

## Message

```go
type Message interface {
    Resolve(ctx context.Context, provider ProviderConfig) (Payload, error)
}
```

消息需要跨 provider fallback 时实现：

```go
type ProviderMessage interface {
    Message
    Providers(ctx context.Context) []string
}
```

`Payload.Data` 使用有序参数列表，兼容飞鸽这类按变量顺序拼接的接口；阿里云 driver 会转换成 JSON map。

复杂业务短信推荐由业务侧定义模板结构体实现 `Message` / `ProviderMessage`。core 只提供 `ResolvePayload` 这类小 helper，帮助业务按 provider 名称选择 `Payload`：

```go
return sms.ResolvePayload(provider.Name, map[string]sms.Payload{
    "aliyun_main": {
        Template: "SMS_001",
        Data: []sms.Param{{Key: "code", Value: code}},
    },
    "debug_text": {
        Content: "您的验证码是 " + code,
    },
})
```

`core/sms` 不提供模板文件配置中心。短信模板 ID、变量名、变量顺序和直发文本由业务模板结构体维护。

## Driver 与 Build Tag

内置 driver：

| driver | package | build tag |
| --- | --- | --- |
| `aliyun` | `pkg/sms/driver/aliyun` | `sdkit_sms_aliyun` |
| `feige` | `pkg/sms/driver/feige` | `sdkit_sms_feige` |
| `twilio` | `pkg/sms/driver/twilio` | `sdkit_sms_twilio` |
| `tencentcloud` | `pkg/sms/driver/tencentcloud` | `sdkit_sms_tencentcloud` |
| `huawei` | `pkg/sms/driver/huawei` | `sdkit_sms_huawei` |

`core/sms` 和 `core/sms/facade` 不 blank import 任何 driver。应用需要某个 driver 时，必须：

- 构建时启用对应 build tag。
- 在启动接线层 import 对应 driver 包，或调用 driver 包的 `Register()`。

示例：

```go
import _ "github.com/huwenlong92/sdkit/pkg/sms/driver/aliyun"
```

Twilio driver 面向国际短信，使用 `Payload.Content` 作为短信正文，不处理 `Payload.Template`。

TencentCloud driver 使用腾讯云 SendSms API 和 TC3-HMAC-SHA256 签名，支持国内短信与国际/港澳台短信。`ProviderConfig.SmsSdkAppID` 对应短信应用 ID，`SignName` 对应签名，`Sender` 对应国际/港澳台 Sender ID。

Huawei driver 使用华为云消息&短信发送短信 API 和 X-WSSE 鉴权，国内和国际/港澳台通过不同短信应用接入地址、通道号和模板配置为不同 provider。`Sender` 对应华为云通道号，`Endpoint` 对应 APP 接入地址。

学校或业务定制 provider 不放进 core，应用可通过 `RegisterDriver` 注入，例如 `cqu`、`nuaa`。

## Middleware

middleware 运行在 manager 编排层。内置 `RateLimitMiddleware`，依赖应用提供 `RateLimiter`：

```go
type RateLimiter interface {
    Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, error)
}
```

## 更新记录

- 2026-05-28：新增 TencentCloud、Huawei 短信 driver，build tag 分别为 `sdkit_sms_tencentcloud`、`sdkit_sms_huawei`。
- 2026-05-28：新增 Twilio 短信 driver，build tag 为 `sdkit_sms_twilio`。
- 2026-05-28：新增 `ResolvePayload` helper，推荐业务侧表驱动实现短信模板。
- 2026-05-27：短信 driver 改为 build tag 按需编译；`core/sms/facade` 不再默认引入 aliyun、feige。
- 2026-05-26：facade 移除 `core/config.V` 隐式配置读取，默认内部注册；新增 `WithExternal()`，全局启动通过显式 `WithConfigLoader` 注入配置。
