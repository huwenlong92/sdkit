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
- 实现具体短信 driver，例如阿里云、飞鸽

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

全部 provider 失败时返回 `NoProviderAvailableError`，其中包含所有 `AttemptResult`。

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

## Driver

第一版内置：

- `aliyun`
- `feige`

学校或业务定制 provider 不放进 core，应用可通过 `RegisterDriver` 注入，例如 `cqu`、`nuaa`。

## Middleware

middleware 运行在 manager 编排层。内置 `RateLimitMiddleware`，依赖应用提供 `RateLimiter`：

```go
type RateLimiter interface {
    Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, error)
}
```

## 更新记录

- 2026-05-26：facade 移除 `core/config.V` 隐式配置读取，默认内部注册；新增 `WithExternal()`，全局启动通过显式 `WithConfigLoader` 注入配置。
