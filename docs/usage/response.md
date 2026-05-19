# Response 使用说明

`core/response` 已移出 sdkit core。应用层 HTTP 响应请在业务仓库维护。

sdkitgo 当前使用：

```go
import "sdkitgo/app/http/response"
```

成功响应：

```go
response.Success(c, data)
```

错误响应：

```go
response.Fail(c, err)
```

middleware 需要统一应用响应结构时，通过 `core/ginresponder` 注入应用 responder：

```go
recovery.Middleware(recovery.WithResponder(appmiddleware.ErrorResponder))
```

未注入 responder 时，sdkit core middleware 使用默认 fallback：

```go
c.JSON(status, gin.H{"error": message})
c.Abort()
```
