# Auth 模块说明

## 职责

`core/auth` 负责认证流程编排和 Guard 抽象：

- JWT Guard
- 主体语义的 `Identity`
- 服务侧 Hooks 接口
- 基础权限判断

`core/auth` 根包不依赖 Gin。Gin 的 context 读写、header/cookie 解析、登录 cookie 写入和 HTTP middleware 放在 `core/auth/adapter/gin`。

`core/auth` 不负责业务用户模型、后台管理员语义、商户语义、业务 RBAC、菜单权限或权限加载。

JWT 的通用签发和解析在 `pkg/jwtx`。

Session 登录态由 HTTP 层通过 `core/session` / `gin-contrib/sessions` 处理，不再作为 `core/auth` Guard。

## Identity 约束

```go
type Identity struct {
    SubjectID   int64
    SubjectType string
    Username    string
    RoleID      int64
    Permissions []string
    Extra       map[string]any
}
```

- `SubjectID` 只表达认证主体 ID。
- `SubjectType` 只表达主体类型，例如 `user`、`admin`、`merchant`。
- app 层负责把 `SubjectID` 解释为 `UserID`、`AdminID` 或其他业务 ID。
- core 提供 `UserID(c)` 读取认证主体 ID，但不解释业务语义；`GetAdminID`、`auth_user_id` 等业务语义入口仍由 app 层提供。

## API DX Helper

Gin middleware 会把 `*auth.Identity` 写入 `auth.identity`。handler 可直接读取：

```go
identity := auth.CurrentIdentity(c)
subjectID := auth.UserID(c)
claims := auth.Claims(c)
```

`auth.UserID(c)` 返回的是认证主体 ID，不等同于 admin/user/merchant 业务 ID。需要区分主体类型时仍应检查 `identity.SubjectType`。

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
    JWT auth.JWTConfig `mapstructure:"jwt"`
}
```

## 更新记录

- 2026-05-15：新增 `CurrentIdentity(c)`、`UserID(c)`、`RoleID(c)` 和 `Claims(c)`，API handler 可直接读取认证主体信息；JWT claims 类型改名为 `JWTClaims`。
- 2026-05-11：`JWTConfig` 从应用配置中心迁入 `core/auth`，`NewJWTGuard` 不再依赖 `core/config.JWT`。
- 2026-05-13：`Identity` 使用 `SubjectID` / `SubjectType` 主体语义；JWT 底层能力放在 `pkg/jwtx`；Session 对外能力收敛到 `core/session`；core 层不写入用户语义 context。
- 2026-05-13：`core/auth` 根包不包含 Gin 依赖，Gin 入口统一放到 `core/auth/adapter/gin`。
