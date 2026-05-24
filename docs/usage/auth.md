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

`SubjectID` 是兼容数字 ID 的认证主体 ID；`Subject` 是字符串主体 ID，适合 UUID、外部账号 ID、设备 ID 等非数字标识。`SubjectKey()` 优先返回 `Subject`，为空时再回退到 `SubjectID`。

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

如果业务用户 ID 是 UUID 或其他字符串，写入 `Subject`：

```go
login, err := apiAuth.Login(ctx, &auth.Identity{
    Subject:     user.UUID,
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

Session 认证器提供生命周期 hooks。core 只负责调用时机；单点登录校验、滑动续期、失败清理等操作由接入方自己实现。

```go
adminAuth := auth.NewSessionAuthenticator(auth.SessionAuthenticator{
    Provider: "admin_session",
    Key:      "admin_login",
    Reader:   authgin.SessionReader{},
    Mapper:   mapAdminSession,
    Validators: []auth.IdentityHook{
        validateSingleLogin,
    },
    Refreshers: []auth.IdentityHook{
        refreshSessionTTL,
    },
    Failures: []auth.IdentityFailureHook{
        clearInvalidSession,
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

`authgin.Required` / `authgin.Optional` 会把 Gin context 注入 request context。接入方如果手动调用 `AuthenticateRequest`，需要先注入：

```go
c.Request = c.Request.WithContext(authgin.WithContext(c.Request.Context(), c))
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

Realtime 如果不挂 Gin session middleware，也可以由接入方实现自己的 `SessionReader`，直接从 request cookie 和 session store 中读取 payload。

Realtime 适配器会把认证身份转换成带类型的 subject key：`<subject_type>:<subject_key>`。例如后台管理员 `admin:1`、UUID 用户 `user:550e8400-e29b-41d4-a716-446655440000`。没有 `SubjectType` 时保留原始 `SubjectKey()`，用于兼容旧调用。

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
