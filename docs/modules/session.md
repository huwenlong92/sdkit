# Session 模块说明

## 职责

`core/session` 只封装 `gin-contrib/sessions`，不再实现自研 session 库，也不再作为 runtime capability 注册。

模块负责：

- 定义 `session.Config` 和 Redis store 配置
- 创建 `sessions.Store`
- 创建 Gin middleware
- 暴露 `sessions.Session` 类型别名
- 提供少量请求级 helper
- 支持调用方传入通用 `Hook`

模块不负责：

- 认证流程和权限判断
- 业务用户模型
- 全局 runtime 生命周期
- 非 HTTP 场景的 session 注入
- 单点登录、辅助 cookie、审计日志等业务动作

## 设计边界

Session 是 HTTP transport 层能力。普通 HTTP 请求、WebSocket 握手和 SSE 建连都可以通过 Gin middleware 读取 cookie session；连接建立后的长期状态应转交给对应业务模块维护。

`core/auth` 不再提供 `SessionGuard`。需要 session 登录态时，由 HTTP 服务自己的 middleware 通过 `session.Get(c, key)` 读取业务定义的 key，并写入服务需要的身份上下文。

## 核心类型

```go
type Store = sessions.Store
type Session = sessions.Session
type Options = sessions.Options
type Hook func(c *gin.Context, s Session, opts Options) error
```

```go
type Config struct {
    Type     string
    Key      string
    Secret   string
    Path     string
    Domain   string
    MaxAge   int
    Secure   bool
    HTTPOnly bool
    SameSite http.SameSite
    Redis    RedisConfig
}
```

`Type` 支持：

- `cookie`
- `memory`，兼容旧配置，实际使用 cookie store
- `redis`

空值按 `cookie` 处理。

## Hook 约束

Hook 在 session `Save()` 成功后执行。Hook 可以做业务侧附加动作，但不要反向修改 session 核心配置。Hook 返回错误时，调用方应自行决定是否把这次业务操作视为失败。

Hook 第三个参数是当前操作实际使用的 cookie options，用于业务侧辅助 cookie 复用 session 的 path、domain、secure、max_age 等配置。

`Set` / `Delete` / `Clear` 使用 middleware 注册时 store 上的默认 options；需要单次覆盖 options 时使用 `SetWith` / `DeleteWith` / `ClearWith`。

## 更新记录

- 2026-05-19：Session 收敛为 `gin-contrib/sessions` 薄封装，移除 runtime facade、自研 store、`SessionGuard` 依赖；新增 `Hook` 支持业务侧额外操作并透出当前 cookie options；`Set` / `Delete` / `Clear` 默认不接 options，单次覆盖使用 `SetWith` / `DeleteWith` / `ClearWith`。
- 2026-05-16：Session facade 支持 `WithName` 服务级能力，并提供 `FromServiceContext` / `MiddlewareFromServiceContext` 统一读取服务本地能力。
- 2026-05-13：Session 字段从用户语义迁移为主体语义；middleware 不再写入 `session_user_id`。
