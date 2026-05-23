# Auth 模块说明

## 职责

`core/auth` 负责请求级认证抽象：

- 统一身份 `Identity`
- 凭证提取 `Extractor`
- JWT 认证器
- Session 认证器
- Chain 认证器
- Gin / Realtime 适配器
- 基础权限判断

`core/auth` 不负责业务用户模型、后台管理员语义、商户语义、菜单权限、RBAC 或 Casbin policy。

JWT 的签发和解析复用 `pkg/jwtx`。Session 存储仍由 `core/session` 负责，`core/auth` 只通过 `SessionReader` 读取登录态并映射为 `Identity`。

## Identity 约束

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

- `SubjectID` / `Subject` 表达认证主体。
- `SubjectType` 表达主体类型，例如 `user`、`admin`、`merchant`、`service`。
- `Provider` 表达认证来源，例如 `api_jwt`、`web_session`。
- app 层负责把主体解释为 `UserID`、`AdminID` 或其他业务 ID。

## 认证器

### JWTAuthenticator

从请求中提取 token，解析为 `Identity`。默认读取 `Authorization: Bearer`，可通过 `WithJWTExtractor` 改成 header、query、cookie 或组合提取。

### SessionAuthenticator

通过 `SessionReader` 读取 session payload。payload 到 `Identity` 的映射由业务传入，core 不感知具体结构。

### ChainAuthenticator

按顺序组合多个认证器，适合 Realtime、WebSocket、兼容 cookie 和 token 的入口。

## 适配器

### Gin

- `authgin.Optional(authenticator)`
- `authgin.Required(authenticator)`
- `authgin.ContextMiddleware()`
- `authgin.SessionReader{}`

### Realtime

`core/auth/adapter/realtime` 把 `auth.RequestAuthenticator` 转为 `core/realtime.Authenticator`。`core/realtime` 不再内置 JWT 解析逻辑。

## 配置

JWT 配置归属在 auth 模块：

```go
type JWTConfig struct {
    Secret string `mapstructure:"secret" yaml:"secret"`
    Issuer string `mapstructure:"issuer" yaml:"issuer"`
    Expire int    `mapstructure:"expire" yaml:"expire"`
}
```

服务配置按需组合：

```go
type Config struct {
    JWT     auth.JWTConfig `mapstructure:"jwt"`
    Session session.Config `mapstructure:"session"`
}
```

## 更新记录

- 2026-05-23：移除旧 `Auth` manager / `Guard` / Gin middleware 入口，统一改为 request authenticator；新增 JWT、Session、Chain、Realtime 适配能力。
- 2026-05-15：新增 `CurrentIdentity(c)`、`UserID(c)`、`RoleID(c)` 和 `Claims(c)`，API handler 可直接读取认证主体信息；JWT claims 类型改名为 `JWTClaims`。
- 2026-05-13：`Identity` 使用 `SubjectID` / `SubjectType` 主体语义；JWT 底层能力放在 `pkg/jwtx`。
- 2026-05-13：`core/auth` 根包不包含 Gin 依赖，Gin 入口统一放到 `core/auth/adapter/gin`。
