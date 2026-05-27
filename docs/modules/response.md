# Response 模块迁移说明

`core/response` 已从 sdkit core 中移除。HTTP JSON 响应 envelope 属于应用层协议，不再放在 core 模块内。

## 当前边界

- `core/errors` 继续负责业务错误模型和错误码。
- `core/gin/responder` 只提供 Gin middleware 的错误响应注入点和默认 fallback。
- sdkit core middleware 不再直接输出 `err_code/msg/data` 应用协议。
- sdkitgo 应用层统一使用 `app/http/response` 输出业务响应。

## Core 默认行为

未注入 responder 时，core Gin middleware 使用默认 fallback：

```go
c.JSON(status, gin.H{"error": message})
c.Abort()
```

这个默认行为只保证 core 独立可用。业务项目需要统一响应结构时，应在应用层注入 responder。

## 应用层接入

sdkitgo 通过 `app/middleware.ErrorResponder` 注入 `app/http/response`：

```go
recovery.Middleware(recovery.WithResponder(appmiddleware.ErrorResponder))
casbin.Middleware(casbin.WithResponder(appmiddleware.ErrorResponder))
```

## 更新记录

- 2026-05-19：移除 `core/response`，新增 `core/gin/responder`，core middleware 改为 responder 注入。
- 2026-05-15：历史版本新增 `response.Fail` 和 `response.AbortFail`。
- 2026-05-13：历史版本新增统一 response 模块。
