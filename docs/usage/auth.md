# Auth 统一认证

## 概述

`core/auth` 负责请求级认证编排，不依赖业务用户表或具体服务模型。

core 负责：

- 定义统一身份 `Identity`
- 从请求中提取凭证 `Extractor`
- 提供 JWT / Session / Chain / Anonymous 认证器
- 提供 Gin 和 Realtime 适配器
- 提供基础权限判断 `HasPermission`

服务负责：

- 登录接口和密码校验
- 设置 `SubjectType`，例如 `user`、`admin`、`merchant`
- 把 session payload 或 JWT claims 映射成 `Identity`
- 在业务层解释 `SubjectID`
- 权限加载、菜单权限、RBAC 或 Casbin policy

## Identity

```go
type Identity struct {
    SubjectID   int64
    Subject     string
    SubjectType string
    TenantID    int64
    Username    string
    RoleID      int64
    Roles       []string
    Permissions []string
    Method      string
    Provider    string
    SessionID   string
    TokenID     string
    ExpiresAt   time.Time
    Extra       map[string]any
}
```

`SubjectID` 是认证主体 ID，不表达业务语义。API 用户、后台管理员、商户、OpenAPI Client 都使用该字段。

`SubjectType` 用于区分主体类型。`Provider` 用于区分认证来源，例如 `api_jwt`、`web_session`、`admin_session`。

## JWT

```go
apiAuth := auth.NewJWTAuthenticator(&cfg.JWT,
    auth.WithJWTProvider("api_jwt"),
    auth.WithJWTExtractor(auth.FirstExtractor(
        auth.BearerTokenExtractor(),
        auth.QueryTokenExtractor("token"),
    )),
)
```

Gin 可选认证：

```go
r.Use(authgin.Optional(apiAuth))
```

必须登录：

```go
authorized := r.Group("/api")
authorized.Use(authgin.Required(apiAuth))
```

签发 token：

```go
login, err := apiAuth.Login(ctx, &auth.Identity{
    SubjectID:   user.ID,
    SubjectType: "user",
    Username:    user.Username,
})
```

## Session

Session 认证器不关心业务 payload，只负责调用业务传入的 mapper。

```go
webAuth := auth.NewSessionAuthenticator(auth.SessionAuthenticator{
    Provider: "web_session",
    Key:      "web_login",
    Reader:   authgin.SessionReader{},
    Mapper: func(ctx context.Context, raw any) (*auth.Identity, error) {
        user := raw.(WebSessionUser)
        return &auth.Identity{
            SubjectID:   user.ID,
            SubjectType: "user",
            Username:    user.Username,
        }, nil
    },
})
```

Gin 中使用 session 时，需要先挂 `core/session` middleware，再挂 auth middleware。

```go
sessionMiddleware, err := session.Middleware(cfg.Session)
if err != nil {
    return err
}
r.Use(sessionMiddleware)
r.Use(authgin.Optional(webAuth))
```

## Chain

Realtime 或混合入口可以组合多种认证方式。

```go
rtAuth := auth.NewChainAuthenticator(
    webSessionAuth,
    apiJWTAuth,
)
```

`ChainAuthenticator` 会按顺序尝试认证。业务 action 仍应按 `SubjectType` 或 `Provider` 做二次约束。

## Realtime 适配

`core/realtime` 不再直接解析 JWT。Realtime gateway 通过适配器复用 `core/auth`。

```go
authenticator := authrealtime.From(rtAuth)
```

如果 Realtime 需要读取 Web session cookie，Gin 路由必须挂同一个 session store，并注入 Gin context。

```go
r.Use(sessionMiddleware)
r.Use(authgin.ContextMiddleware())
```

## Handler 取身份

```go
identity := auth.CurrentIdentity(c)
if identity == nil {
    return
}

subjectID := identity.SubjectID
subjectType := identity.SubjectType
provider := identity.Provider
```

如果只需要主体 ID 或 JWT claims：

```go
subjectID := auth.UserID(c)
claims := auth.Claims(c)
```

## 错误约定

| 错误 | 说明 |
|------|------|
| `auth.ErrInvalidCredentials` | 用户名或密码错误 |
| `auth.ErrAccountDisabled` | 账号禁用 |
| `auth.ErrUnauthorized` | 未登录或认证无效 |
