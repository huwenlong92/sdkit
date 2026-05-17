# Session 模块说明

## 职责

`core/session` 提供 Cookie + Store 的会话 capability：

- 全局默认 store 初始化
- 服务级独立 store
- request context 中传递当前 store
- Gin 登录态中间件
- Cookie 写入和清理
- 对 session 数据结构、Store 和存储实现的 capability 封装

底层存储实现负责：

- `Session` 数据结构
- `Store` 接口
- MemoryStore
- RedisStore
- Cookie 基础结构

`core/session` 不负责加载应用配置，也不解释业务用户模型。

业务侧统一通过 `core/session` 使用会话能力，不直接依赖底层存储实现包。

## 配置

```go
type Config struct {
    Prefix string `mapstructure:"prefix" yaml:"prefix"`
}
```

初始化入口：

```go
session.Init(redis.RDB, &cfg.Session)
```

`cfg.Session` 由应用或服务配置组合 `session.Config` 得到。

## Runtime Capability

`core/session` 是会话实现包，Runtime Capability 接入层统一放在 `core/session/facade`：

```text
core/session/
  cookie.go
  middleware.go
  session.go
  facade/
    config.go
    client.go
    use.go
    default.go
```

启动时由主 Runtime 注册：

```go
import sessioncap "github.com/huwenlong92/sdkit/core/session/facade"

runtimeApp.RegisterCapabilities(
    sessioncap.Use(sessioncap.WithConfig(cfg.Session)),
)
```

bootstrap 使用 `sessioncap.WithConfigLoader(...)`，确保配置能力先初始化，再由 `sessioncap.Use` 读取最终配置。Redis 已初始化时使用 RedisStore，否则降级为 MemoryStore。

同一个 facade 可以按注册位置决定作用域：

- bootstrap 注册 `sessioncap.Use(...)`：能力名 `session`，`ScopeGlobal`，初始化并绑定全局默认 store。
- 服务 `RuntimeCapabilities` 注册 `sessioncap.Use(sessioncap.WithName(ctx.LocalName(sessioncap.Name)))`：能力名如 `api.session`、`admin.session`，`ScopeServiceLocal`，只创建并绑定服务独立 store，不写入全局默认 store。

服务 router 应通过 `sessioncap.MiddlewareFromServiceContext(ctx)` 注入当前服务的 store；handler 使用 `session.FromContext(c.Request.Context())` 或 `session.StoreFromContext(...)` 获取。runtime-managed 服务应保证 session capability 已注册；直接构造 router 的测试场景未注册 session 时，该 middleware 退化为 no-op。

## Session 结构

```go
type Session struct {
    ID          string
    SubjectID   int64
    SubjectType string
    Username    string
    RoleID      int64
    Permissions []string
    ExpiresAt   time.Time
    Extra       map[string]any
}
```

`SubjectID` / `SubjectType` 由上游认证模块解释。Session 本身只保存和透传。

## 更新记录

- 2026-05-16：Session facade 支持 `WithName` 服务级能力，并提供 `FromServiceContext` / `MiddlewareFromServiceContext` 统一读取服务本地能力；`core/session` 新增 `ContextWithStore`、`FromContext`、`StoreFromContext` 和 `WithStore`，支持 API/Admin 使用不同 session store。
- 2026-05-16：新增 `core/session/facade` Runtime Capability 接入层，按 `config.go/client.go/use.go/default.go` 组织，根包保留 Session 实现和业务 API。
- 2026-05-11：`session.Config` 归属回 `core/session`，`Init` 不再依赖 `core/config.SessionConfig`。
- 2026-05-13：Session 字段从用户语义迁移为主体语义；middleware 不再写入 `session_user_id`。
- 2026-05-13：会话对外类型和 Store 入口统一收敛到 `core/session`。
