# Email 模块方案

## 目标

`core/email` 提供统一邮件发送入口，支持多个命名发送方、默认发送方、失败转移和 middleware。

目标能力：

- 从 `email` 配置初始化默认 manager
- 支持多个命名 provider
- 默认发送方失败后按 `fallback` 顺序重试
- 支持单次指定 provider 列表
- 支持发送 middleware
- Runtime capability 放在 `core/email/facade`
- facade 默认作为内部 capability 注册；需要对外展示时显式使用 `WithExternal()`
- facade 不从 `core/config.V` 隐式读取配置，应用必须通过 `WithConfig` 或 `WithConfigLoader` 注入配置
- facade 支持 `WithOptional()`，用于全局启动时在未显式传入配置的环境跳过绑定

## 模块边界

`core/email` 负责：

- 管理 provider 配置和实例
- 按名称懒加载 provider
- 编排默认发送、指定发送和失败转移
- 提供 middleware 扩展点
- 绑定 manager 到 runtime container

`pkg/email` 负责：

- 定义底层 provider 接口和发送结果
- 管理 driver 注册表
- 实现具体邮件 driver，例如 SMTP

`core/email` 不负责：

- 业务邮件模板渲染
- 异步队列投递
- 发送日志落库
- 敏感信息打印

异步发送应由应用层或 `core/queue` 承担。

## 配置模型

```yaml
email:
  default: smtp_main
  fallback: [smtp_backup]
  providers:
    smtp_main:
      driver: smtp
      host: smtp.example.com
      port: 587
      username: ${SMTP_USERNAME}
      password: ${SMTP_PASSWORD}
      from_address: noreply@example.com
      encryption: starttls
    smtp_backup:
      driver: smtp
      host: smtp2.example.com
      port: 465
      username: ${SMTP2_USERNAME}
      password: ${SMTP2_PASSWORD}
      from_address: noreply@example.com
      encryption: tls
```

`fallback` 是邮件级全局备用链。短信模板存在平台审核差异，因此短信不使用全局 fallback。

## 对外 API

```go
func NewManager(cfg Config, middleware ...Middleware) (*Manager, error)
func Send(ctx context.Context, msg Message) (*SendResult, error)
func SendVia(ctx context.Context, msg Message, providers ...string) (*SendResult, error)
func Use(name string) (Provider, error)
func Close() error
```

`SendResult.Attempts` 记录尝试过的 provider。全部失败时返回 `NoProviderAvailableError`，其中保留每次尝试的错误。

## Driver

第一版内置：

- `smtp`：基于 Go 标准库 SMTP 实现，位于 `pkg/email/driver/smtp`

第三方或应用内自定义发送方通过 `RegisterDriver` 注册。

## 更新记录

- 2026-05-26：facade 移除 `core/config.V` 隐式配置读取，默认内部注册；新增 `WithExternal()`，全局启动通过显式 `WithConfigLoader` 注入配置。
