# Email 模块方案

## 目标

`core/email` 提供统一邮件发送入口，支持多个命名发送方、默认发送方、失败转移和 middleware。

目标能力：

- 从 `email` 配置初始化默认 manager
- 支持多个命名 provider
- 默认发送方失败后按 `fallback` 顺序重试
- 支持单次指定 provider 列表
- 支持直接内容邮件和固定模板邮件
- 支持发送 middleware
- 支持通过 `ProviderResolver` 在发送时动态加载账号配置
- 支持由调用方提供可重复打开的附件流，SMTP 按流生成 MIME
- Runtime capability 放在 `core/email/facade`
- facade 默认作为内部 capability 注册；需要对外展示时显式使用 `WithExternal()`
- facade 不从 `core/config.V` 隐式读取配置，应用必须通过 `WithConfig` 或 `WithConfigLoader` 注入配置
- facade 支持 `WithOptional()`，用于全局启动时在未显式传入配置的环境跳过绑定

## 模块边界

`core/email` 负责：

- 管理 provider 配置和实例
- 动态模式下按发送路由解析当前账号配置
- 按名称懒加载 provider
- 编排默认发送、指定发送和失败转移
- 渲染配置中的固定邮件模板
- 提供 middleware 扩展点
- 绑定 manager 到 runtime container

`pkg/email` 负责：

- 定义底层 provider 接口和发送结果
- 定义与具体存储无关的 `AttachmentSource`，并管理附件流生命周期
- 管理 driver 注册表
- 实现具体邮件 driver，例如 SMTP

`core/email` 不负责：

- 模板持久化、版本管理和后台配置
- 异步队列投递
- 发送日志落库
- 敏感信息打印

异步发送应由应用层或 `core/queue` 承担。

静态模式通过 `NewManager` 初始化并复用 provider。动态模式通过 `NewDynamicManager` 初始化常驻 manager，每次 `SendVia` 调用 Resolver 获取当前账号配置、创建 provider、完成发送并关闭 provider。SMTP driver 的网络连接仍然只在单次发送内建立和关闭。

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
  templates:
    verify_code:
      subject: 验证码 {{.code}}
      text_file: verify_code.txt
      html_file: verify_code.html
```

`fallback` 是邮件级全局备用链。短信模板存在平台审核差异，因此短信不使用全局 fallback。

## 对外 API

```go
func NewManager(cfg Config, middleware ...Middleware) (*Manager, error)
func NewDynamicManager(resolver ProviderResolver, renderer TemplateRenderer, middleware ...Middleware) (*Manager, error)
func Send(ctx context.Context, msg Message) (*SendResult, error)
func SendVia(ctx context.Context, msg Message, providers ...string) (*SendResult, error)
func Use(name string) (Provider, error)
func Close() error
```

动态 manager 没有隐式默认账号，必须通过 `SendVia` 传入稳定账号名。Resolver 可以读取数据库或配置中心，但不得把密码写入错误、日志或链路追踪。

`SendResult.Provider` 和 `SendResult.Result` 记录最终成功的 provider 及结果。`SendResult.Error` 记录最终错误。`SendResult.Attempts` 记录尝试过的 provider。全部失败时第二返回值为 `NoProviderAvailableError`，同时返回的 `SendResult` 里也会保留 `Error` 和 `Attempts`。

## Message

邮件支持两种消息：

- `DirectMessage`：调用方直接传入 `Subject`、`Text` 或 `HTML`
- `TemplateMessage`：调用方传入模板名和变量，由 manager 渲染配置中的固定模板

模板语法使用 Go template。`subject`、`text` 使用 `text/template`，`html` 使用 `html/template`。

`DirectMessage` 和 `TemplateMessage` 都可以携带 `[]Attachment`。附件只保存文件名、
内容类型、大小和 `AttachmentSource`；core 不保存附件 ID、对象键，也不绑定任何存储
实现。每次 provider 尝试前都会重新打开全部附件流，打开失败返回
`ErrAttachmentOpen`，已经打开的流由 provider 统一关闭。

固定模板支持直接写内容，也支持从文件加载：

```go
templates, err := email.LoadTemplates(os.DirFS("templates/email"), cfg.Email.Templates)
if err != nil {
    return err
}
cfg.Email.Templates = templates
```

文件加载只在初始化阶段执行。core 不负责监听文件变更，也不负责模板后台管理。

## Driver

第一版内置：

- `smtp`：基于 Go 标准库 SMTP 实现，位于 `pkg/email/driver/smtp`；正文和附件直接写入
  SMTP DATA，附件采用带 76 字符换行的 Base64 编码，不保留整封邮件原文

第三方或应用内自定义发送方通过 `RegisterDriver` 注册。

## 更新记录

- 2026-09-03：新增存储无关的附件流接口；SMTP 支持流式 MIME 附件和确定未发送的附件打开错误。
- 2026-09-02：新增动态邮件账号 Resolver；manager 常驻，账号配置按次解析，provider 与 SMTP 连接按次创建和关闭。
- 2026-05-28：邮件模板新增 `subject_file`、`text_file`、`html_file`，支持从 `fs.FS` 加载固定模板文件。
- 2026-05-27：新增 `DirectMessage` 和 `TemplateMessage`，邮件发送改为先解析成 `Payload` 再交给 provider。
- 2026-05-26：facade 移除 `core/config.V` 隐式配置读取，默认内部注册；新增 `WithExternal()`，全局启动通过显式 `WithConfigLoader` 注入配置。
