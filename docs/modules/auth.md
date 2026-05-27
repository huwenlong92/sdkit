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

JWT 的签发和解析复用 `pkg/jwtx`。Session 存储仍由 `core/gin/session` 负责，`core/auth` 只通过 `SessionReader` 读取登录态并映射为 `Identity`，并提供生命周期 hooks 给接入方挂载业务操作。

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

- `SubjectID` / `Subject` 表达认证主体。`SubjectID` 保留数字 ID 兼容；`Subject` 用于 UUID、外部账号 ID、设备 ID 等字符串主体。
- `SubjectType` 表达主体类型，例如 `user`、`admin`、`merchant`、`service`。
- `Provider` 表达认证来源，例如 `api_jwt`、`web_session`。
- app 层负责把主体解释为 `UserID`、`AdminID` 或其他业务 ID。
- `SubjectKey()` 优先返回 `Subject`，为空时回退到 `SubjectID`。

## 认证器

### JWTAuthenticator

从请求中提取 token，解析为 `Identity`。默认读取 `Authorization: Bearer`，可通过 `WithJWTExtractor` 改成 header、query、cookie 或组合提取。

### SessionAuthenticator

通过 `SessionReader` 读取 session payload。payload 到 `Identity` 的映射由业务传入，core 不感知具体结构。

- `Mapper`：把业务 session payload 映射成 `Identity`。
- `Validators`：认证成功前的业务态校验，例如单点登录 token、账号状态。
- `Refreshers`：认证成功后的续期动作，例如重写 session、刷新 Redis TTL。
- `Failures`：认证失败后的清理动作，例如删除脏 session、清理辅助 cookie。

### ChainAuthenticator

按顺序组合多个认证器，适合 Realtime、WebSocket、兼容 cookie 和 token 的入口。

## 适配器

### Gin

- `authgin.Optional(authenticator)`
- `authgin.Required(authenticator)`
- `authgin.ContextMiddleware()`
- `authgin.WithContext(ctx, c)`
- `authgin.ContextFrom(ctx)`
- `authgin.SessionReader{}`

### Realtime

`core/auth/adapter/realtime` 把 `auth.RequestAuthenticator` 转为 `core/realtime.Authenticator`。`core/realtime` 不再内置 JWT 解析逻辑。

适配器输出的 realtime `UserID` 使用带类型 subject key：`<subject_type>:<subject_key>`，避免 admin、web user、OpenAPI client 等不同主体使用相同数字 ID 时互相串线。没有 `SubjectType` 时保留原始 `SubjectKey()`，兼容旧调用。

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

- 2026-05-24：JWT claims 新增字符串 `Subject`，Realtime 适配器统一输出带 `SubjectType` 的 subject key。
- 2026-05-23：SessionAuthenticator 新增生命周期 hooks，支持接入方实现业务态校验、滑动续期和失败清理。
- 2026-05-23：移除旧 `Auth` manager / `Guard` / Gin middleware 入口，统一改为 request authenticator；新增 JWT、Session、Chain、Realtime 适配能力。
- 2026-05-15：新增 `CurrentIdentity(c)`、`UserID(c)`、`RoleID(c)` 和 `Claims(c)`，API handler 可直接读取认证主体信息；JWT claims 类型改名为 `JWTClaims`。
- 2026-05-13：`Identity` 使用 `SubjectID` / `SubjectType` 主体语义；JWT 底层能力放在 `pkg/jwtx`。
- 2026-05-13：`core/auth` 根包不包含 Gin 依赖，Gin 入口统一放到 `core/auth/adapter/gin`。
