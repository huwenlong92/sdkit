# CORS 使用文档

`core/gin/cors` 用于 Gin HTTP 服务的跨域处理。

## 默认使用

```go
import "github.com/huwenlong92/sdkit/core/gin/cors"

r.Use(cors.Middleware())
```

默认允许 `Content-Type`、`Authorization`、`X-Request-ID`、`X-Track-ID` 请求头，并暴露 `X-Session-Expires-At`、`X-Request-ID`、`X-Track-ID` 响应头。

## 自定义配置

```go
r.Use(cors.Middleware(
    cors.WithOrigins("https://example.com"),
    cors.WithMethods("GET", "POST"),
    cors.WithHeaders("Content-Type", "Authorization", "X-Request-ID", "X-Track-ID"),
    cors.WithExposeHeaders("X-Session-Expires-At", "X-Request-ID", "X-Track-ID"),
    cors.WithMaxAge("3600"),
))
```

需要跨域携带 cookie 时开启 credentials：

```go
r.Use(cors.Middleware(
    cors.WithCredentials(true),
))
```

当 `Origins` 为默认 `*` 且开启 credentials 时，中间件会回显当前请求的 `Origin`，避免浏览器拒绝 `Access-Control-Allow-Origin: *` 携带凭据的响应。

## 注意事项

- 如果前端需要读取 response header，必须通过 `WithExposeHeaders` 暴露。
- `WithHeaders` 会替换默认 allow headers；自定义时不要漏掉业务需要透传的 request header。
- 携带 cookie 的跨域请求必须开启 `WithCredentials(true)`，前端也要设置 `withCredentials`。
- OPTIONS 预检请求会直接返回 `204`。
