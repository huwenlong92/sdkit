# Response 使用说明

示例中 `apperrors` 表示：

```go
import apperrors "github.com/huwenlong92/sdkit/core/errors"
```

## 成功响应

```go
response.Success(c, gin.H{
    "id":   user.ID,
    "name": user.Name,
})
```

响应：

```json
{
  "err_code": 200,
  "msg": "ok",
  "data": {
    "id": 1,
    "name": "admin"
  }
}
```

## 返回业务错误

```go
err := apperrors.New(
    apperrors.CodeBusinessError,
    apperrors.SubCodeUserNotFound,
    "用户不存在",
)
response.Fail(c, err)
```

带原始错误时保留错误链：

```go
if err := db.First(&user, id).Error; err != nil {
    response.Fail(c, apperrors.Wrap(
        err,
        apperrors.CodeBusinessError,
        apperrors.SubCodeUserNotFound,
        "用户不存在",
    ))
    return
}
```

需要返回额外数据：

```go
response.Fail(c, apperrors.WrapWithData(
    err,
    apperrors.CodeConflict,
    apperrors.SubCodeConflict,
    "资源冲突",
    gin.H{"id": id},
))
```

## 参数校验错误

handler 中统一使用 validator helper：

```go
var req struct {
    Username string `json:"username" binding:"required"`
}
if err := validator.BindJSON(c, &req); err != nil {
    response.Fail(c, err)
    return
}
```

`validator.BindJSON` / `BindQuery` / `BindForm` 会将错误转换为 `CodeBadRequest` 的 `AppError`，再通过 `response.Fail` 输出。

## 直接按错误码返回

纯业务分支没有底层 error 时，先构造 `AppError`，再交给 `response.Error`：

```go
response.Fail(c, apperrors.NewCodeWithData(
    apperrors.CodeInternal,
    "查询失败",
    nil,
))
```

这样 `errors.As` / `errors.Is`、validator 映射和错误码常量可以保持一致。
