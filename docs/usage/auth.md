# Auth 统一认证

## 概述

`core/auth` 只做认证流程编排，不依赖任何服务的用户表或用户模型。

core 负责：

- 定义 `Credentials`
- 定义主体语义的 `Identity`
- 定义 `Hooks`
- 定义 `Guard`
- 提供 `JWTGuard`
- 提供基础权限判断 `HasPermission`

服务负责：

- 实现自己的 `Hooks`
- 设置 `SubjectType`
- 启动时配置 `JWTGuard`
- 注册登录接口
- 通过 `core/auth/adapter/gin` 注册 Gin 鉴权中间件
- 在 app 内解释 `SubjectID` 的业务含义

Session 登录态不放在 `core/auth` 的 Guard 内。需要基于 cookie session 的服务，在 HTTP 层使用 `core/session` / `gin-contrib/sessions` 读取登录用户，再写入服务自己的身份上下文。

## 核心类型

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

`SubjectID` 是认证主体 ID，不表达业务语义。API 用户、后台管理员、商户、OpenAPI Client 都使用该字段。

`SubjectType` 用于区分主体类型，例如：

```text
user
admin
merchant
service
openapi
```

`core/auth` 不提供 `GetUserID`、`GetAdminID` 这类业务方法。业务服务在自己的包内解释身份，例如 admin 服务的 `AdminID(c)` 或 API 服务的 `UserID(c)`。

## 服务实现 Hooks

以 admin 为例：

```go
func (h *Hooks) BuildIdentity(ctx context.Context, user any) (*auth.Identity, error) {
    systemUser := user.(*models.SystemUser)
    return &auth.Identity{
        SubjectID:   systemUser.ID,
        SubjectType: "admin",
        Username:    systemUser.Username,
        RoleID:      systemUser.RoleID,
        Extra: map[string]any{
            "avatar": systemUser.Avatar,
        },
    }, nil
}
```

Gin 适配层会传入 `c.Request.Context()`：

```go
func (h *Hooks) FindUser(ctx context.Context, username string) (any, error) {
    var user models.SystemUser
    if err := h.db.WithContext(ctx).Where("username = ?", username).First(&user).Error; err != nil {
        return nil, auth.ErrInvalidCredentials
    }
    return &user, nil
}
```

## 启动时选择 Guard

### JWT 模式

```go
auth.Default = auth.NewWithGuard(
    auth.NewJWTGuard(&cfg.JWT),
    adminauth.NewHooks(database.DB),
)
```

JWT 底层签发和解析由 `pkg/jwtx` 提供，`core/auth` 负责将 claims 映射为 `Identity`。

登录成功返回 token，后续请求使用：

```http
Authorization: Bearer <token>
```

## handler 取身份

Gin handler 通过根包 helper 获取通用身份：

```go
func Me(c *gin.Context) {
    identity := auth.CurrentIdentity(c)
    if identity == nil {
        response.Error(c, apperrors.NewCodeWithData(apperrors.CodeUnauthorized, "未登录", nil))
        return
    }

    subjectID := identity.SubjectID
    subjectType := identity.SubjectType
}
```

如果只需要主体 ID 或 JWT 风格 claims：

```go
subjectID := auth.UserID(c)
claims := auth.Claims(c)
```

app 内仍然需要按 `SubjectType` 解释业务身份：

```go
func AdminID(c *gin.Context) int64 {
    identity := auth.CurrentIdentity(c)
    if identity == nil || identity.SubjectType != "admin" {
        return 0
    }
    return identity.SubjectID
}
```

中间件写入以下 context key：

| key | 说明 |
|------|------|
| `auth.identity` | `*auth.Identity` |

## 权限判断

`core/auth` 只提供纯内存判断，不负责权限加载、RBAC、菜单权限或 Casbin policy：

```go
if !auth.HasPermission(authgin.GetIdentity(c), "system:user:list") {
    response.Error(c, apperrors.NewCodeWithData(apperrors.CodeForbidden, "无权限", nil))
    return
}
```

## 登出

```go
func Logout(c *gin.Context) {
    _ = authgin.Logout(c, auth.Default)
    response.Success(c, nil)
}
```

JWT 模式下服务端无状态，`Logout` 不做额外处理。

Session 模式下会删除 session 并清除 `sid` cookie。

## 错误约定

| 错误 | 说明 |
|------|------|
| `auth.ErrInvalidCredentials` | 用户名或密码错误 |
| `auth.ErrAccountDisabled` | 账号禁用 |
| `auth.ErrUnauthorized` | 未登录或认证无效 |
| `auth.ErrHookNotImplemented` | hook 未实现 |
