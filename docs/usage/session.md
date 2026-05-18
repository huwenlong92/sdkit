# Session 会话管理

## 概述

`core/session` 是 `gin-contrib/sessions` 的薄封装，只处理 HTTP 请求里的 session：

- 初始化 cookie / redis store
- 注册 Gin session middleware
- 从 `*gin.Context` 读取当前 session
- 提供 `Set` / `Get` / `Delete` / `Clear` 快捷函数
- 在写入、删除、清空后执行调用方传入的 `Hook`

Session 不再是 runtime capability，也不再维护自研 store。WebSocket / SSE 可以在 HTTP 握手阶段读取同一套 Gin session；握手结束后的业务状态应由 realtime 自己维护。

## 配置

```yaml
session:
  type: redis      # cookie / memory / redis，空值默认 cookie；memory 兼容为 cookie store
  key: sid         # 浏览器 cookie 名
  secret: change-me
  path: /
  max_age: 1800
  secure: false
  http_only: true
  redis:
    network: tcp
    address: 127.0.0.1:6379
    password: ""
    db: 0
    pool_size: 10
    key_prefix: "session:"
```

`secret` 必填。cookie / memory 模式使用 `gin-contrib/sessions/cookie`；redis 模式使用 `gin-contrib/sessions/redis`。

如果 session 里保存 struct，需要注册 gob 类型：

```go
session.Register(User{})
```

## 注册中间件

```go
middleware, err := session.Middleware(cfg.Session)
if err != nil {
    return err
}

r := gin.New()
r.Use(middleware)
```

也可以只创建 store，自己接 `gin-contrib/sessions`：

```go
store, err := session.NewStore(cfg.Session)
if err != nil {
    return err
}
r.Use(sessions.Sessions(cfg.Session.CookieKey(), store))
```

## 写入和读取

`LoginIdentityCode` 这类 key 只是 session 内部字段名，不是 cookie 里的 sid。它用于标识“这个 session 里哪一项是登录用户”。

```go
const LoginIdentityCode = "session_user"

type User struct {
    ID      string
    IsLogin string
}

err := session.Set(c, LoginIdentityCode, user, session.Options{
    Path:     "/",
    MaxAge:   30 * 60,
    HttpOnly: true,
})
```

读取：

```go
value := session.Get(c, LoginIdentityCode)
user, ok := value.(User)
```

直接使用原生能力：

```go
s := session.Default(c)
s.Set(LoginIdentityCode, user)
err := s.Save()
```

## Hook

`Hook` 用于把登录、登出或其它 session 操作后的额外动作留给业务层，例如写单点登录缓存、写辅助 cookie、打审计日志。

```go
err := session.Set(c, LoginIdentityCode, user, opts,
    func(c *gin.Context, s session.Session) error {
        key := fmt.Sprintf("%s_single", user.ID)
        if err := redis.Client.Set(c.Request.Context(), key, user.IsLogin, ttl).Err(); err != nil {
            return err
        }
        c.SetCookie("isLogin", user.IsLogin, opts.MaxAge, opts.Path, opts.Domain, opts.Secure, false)
        return nil
    },
)
```

Hook 不属于 session 核心逻辑；核心只保证按顺序调用并返回第一个错误。

## 删除和清空

```go
_ = session.Delete(c, LoginIdentityCode, session.Options{Path: "/", MaxAge: -1})
_ = session.Clear(c, session.Options{Path: "/", MaxAge: -1})
```

## 对外 API

| API | 说明 |
| --- | --- |
| `Register(value any)` | 注册 gob 类型 |
| `NewStore(cfg Config)` | 创建 cookie / redis store |
| `Middleware(cfg Config)` | 创建 Gin middleware |
| `Default(c)` | 返回 `gin-contrib/sessions.Default(c)` |
| `Current(c)` | 返回当前 Gin session 和是否存在 |
| `Set(c, key, value, opts, hooks...)` | 写入并保存 |
| `Get(c, key)` | 读取字段 |
| `Delete(c, key, opts, hooks...)` | 删除字段并保存 |
| `Clear(c, opts, hooks...)` | 清空并保存 |
