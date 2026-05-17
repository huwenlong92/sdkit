# Session 会话管理

## 概述

Cookie + Store 模式：登录时服务端创建 session 存入 Redis/内存，`sid` cookie 写入浏览器，后续请求自动携带。

`core/session` 统一提供 Session、Store、内存/Redis 存储构造函数和 Cookie 基础能力。

Session 只保存认证主体，不表达业务用户模型。

业务侧统一通过 `core/session` 使用会话能力，不直接依赖底层存储实现包。

## 配置

```yaml
session:
  prefix: "myapp:"   # key 前缀，默认 "session:"
```

bootstrap 自动初始化：Redis 可用时使用 RedisStore，否则使用 MemoryStore。

## Runtime Capability 引入

`core/session` 保留会话实现和业务 API；Runtime 接入门面放在 `core/session/facade`：

```go
import sessioncap "github.com/huwenlong92/sdkit/core/session/facade"

app := runtime.New()
app.RegisterCapabilities(
    sessioncap.Use(sessioncap.WithConfig(appCfg.Session)),
)
```

bootstrap 会在主 Runtime 中先加载配置，再通过 `core/session/facade` 的 `sessioncap.Use(sessioncap.WithConfigLoader(...))` 注册公共 session 能力。业务代码仍然直接使用根包 `github.com/huwenlong92/sdkit/core/session`。

## 服务级 Session

如果多个服务需要不同 session 配置，在服务 `Provider.RuntimeCapabilities` 中注册服务本地能力：

```go
sessioncap.Use(
    sessioncap.WithName(ctx.LocalName(sessioncap.Name)), // api.session / admin.session
    sessioncap.WithConfigLoader(func(*runtime.App) (sessioncap.Config, error) {
        cfg, err := apiconfig.Load(ctx.ConfigFile, ctx.Name, ctx.BaseConfig(), ctx.ConfigKey)
        if err != nil {
            return sessioncap.Config{}, err
        }
        return cfg.Session, nil
    }),
)
```

Router 构建时使用 session facade 读取当前服务的 store，并注入请求 context：

```go
r.Use(sessioncap.MiddlewareFromServiceContext(ctx))
```

runtime-managed 服务中该能力应由 provider 注册完成；直接构造 router 且未注册 session capability 时，middleware 会退化为 no-op。

handler 中通过标准 context 获取：

```go
store, ok := session.FromContext(c.Request.Context())
if !ok {
    // 当前请求没有注入 session store
}
```

`session.StoreFromContext` 与 `session.FromContext` 等价，返回 `(session.Store, bool)`。

## 登录创建会话

```go
sess := &session.Session{
    ID:          token,
    SubjectID:   identity.SubjectID,
    SubjectType: identity.SubjectType,
    Username:    identity.Username,
    RoleID:      identity.RoleID,
    Permissions: identity.Permissions,
    Extra:       identity.Extra,
}

store, _ := session.FromContext(c.Request.Context())
store.Set(c.Request.Context(), sess, session.SessionTTL)
session.SetCookie(c, sess.ID, session.SessionTTL)
```

如果使用 bootstrap 全局 session，可以继续使用 `session.GetStore()`。如果使用服务级 session，优先从 request context 获取 store。

## 保护路由

```go
r.Use(session.Require(store))    // 未登录返回 401
r.Use(session.Optional(store))   // 检测但不强制
```

handler 中获取 session：

```go
sess := session.GetSession(c)
if sess != nil {
    subjectID := sess.SubjectID
    subjectType := sess.SubjectType
}
```

`core/session` 只写入：

| key | 说明 |
|------|------|
| `session.current` | `*session.Session` |

## 登出

```go
func Logout(c *gin.Context) {
    sid, _ := c.Cookie(session.CookieName)
    session.GetStore().Delete(c.Request.Context(), sid)
    session.ClearCookie(c)
}
```

## 自定义存储

```go
store := session.NewRedisStore(redis.RDB)
r.Use(session.Require(store))

store := session.NewRedisStoreWithPrefix(redis.RDB, "mysvc:session:")
```

需要独立 Redis 客户端时见 [redis.md](redis.md)。

## Extra 存取

```go
sess.Extra["dept_id"] = int64(1)
deptID, _ := sess.GetExtraInt64("dept_id")

name := sess.GetExtraString("custom")
v, ok := sess.GetExtra("key")
```

`Extra` 是 `map[string]any`。如果存 struct，Redis JSON 序列化后会丢失 Go 类型，建议先序列化成 JSON 字符串再存。

## 常量

| 常量 | 值 | 说明 |
|------|-----|------|
| `CookieName` | `sid` | cookie 名 |
| `SessionTTL` | 30min | 会话有效期 |
| `RenewThreshold` | 5min | 距过期此时间内自动续期 |
| `HeaderExpireKey` | `X-Session-Expires-At` | 响应头 |
| `ContextSessionKey` | `session.current` | context key |
