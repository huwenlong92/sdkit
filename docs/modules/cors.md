# CORS 模块方案

## 作用

`core/cors` 提供 Gin CORS 中间件，统一处理跨域请求的 allow headers、expose headers、methods、origins 和 max age。

## 对外接口

```go
type Config struct {
    Origins       []string
    Methods       []string
    Headers       []string
    ExposeHeaders []string
    MaxAge        string
}

func Middleware(opts ...Option) gin.HandlerFunc
func WithOrigins(origins ...string) Option
func WithMethods(methods ...string) Option
func WithHeaders(headers ...string) Option
func WithExposeHeaders(headers ...string) Option
func WithMaxAge(seconds string) Option
```

## 默认配置

- origins：`*`
- methods：`GET, POST, OPTIONS`
- allow headers：`Content-Type, Authorization, X-Request-ID, X-Track-ID`
- expose headers：`X-Session-Expires-At, X-Request-ID, X-Track-ID`
- max age：`86400`

## 内部约束

- OPTIONS 请求直接返回 `204` 并终止后续 handler。
- tracking/request headers 必须同时允许请求透传和响应读取。
- 不在 CORS 模块内处理鉴权或来源白名单配置加载。

## 更新记录

- 2026-05-12：默认 allow/expose headers 切换到 `X-Track-ID`，新增 `WithExposeHeaders`。
