# Response 模块方案

## 作用

`core/response` 是 HTTP JSON 响应的统一出口。业务 handler 不直接调用 `c.JSON(...)`，而是使用本模块保持响应结构稳定。

统一结构：

```json
{
  "err_code": 200,
  "msg": "ok",
  "data": {}
}
```

成功响应的 `err_code` 固定为 `200`，与 `response.SuccessCode` 和 `core/errors.CodeOK` 保持一致。

## 对外接口

```go
func JSON(c *gin.Context, code int, data any)
func AbortJSON(c *gin.Context, code int, data any)
func Success(c *gin.Context, data interface{})
func Error(c *gin.Context, err error)
func Fail(c *gin.Context, err error)
func AbortError(c *gin.Context, err error)
func AbortFail(c *gin.Context, err error)
func AppError(err error) *errors.AppError
```

接口语义：

| 接口 | 说明 |
|------|------|
| `Success` | 返回 `err_code=200,msg=ok` |
| `Error` | 基于 `error` 自动映射响应 |
| `Fail` | `Error` 的 handler 语义别名，推荐业务 handler 使用 |
| `AbortError` | middleware 中返回错误并中断 Gin 链路 |
| `AbortFail` | `Fail` 后中断 Gin 链路 |
| `AppError` | 将任意 error 归一化为 `*errors.AppError` |
| `JSON` / `AbortJSON` | 底层 JSON 输出能力，主要供 middleware 或特殊场景使用 |

## Error 映射规则

`response.Error(c, err)` 的映射规则：

- 如果错误链中存在 `*errors.AppError`，使用其中的 `Code`、`Message` 和 `Data`。
- 如果是普通 error，统一映射为 `CodeInternal` / `INTERNAL_ERROR` / `服务器内部错误`。
- 业务 response 的 HTTP 状态保持 `200`，业务结果通过 `err_code` 表达。

JSON 编码失败时，`JSON` 会返回 HTTP `500`，响应体为：

```json
{
  "err_code": 500,
  "msg": "json marshal error",
  "data": null
}
```

## Handler 约束

新增或迁移 handler 时：

- 成功用 `response.Success(c, data)`。
- 错误用 `response.Fail(c, err)`，纯业务分支先构造 `core/errors.AppError`。
- validator 错误使用 `validator.BindJSON` / `BindQuery` / `BindForm` 返回的 error，并交给 `response.Fail`。
- 不要在 handler 内直接散落新的数字错误码；先在 `core/errors` 中增加常量。

## 更新记录

- 2026-05-15：新增 `response.Fail` 和 `response.AbortFail`，API handler 统一使用 `Fail` 表达业务失败。
- 2026-05-13：错误响应统一通过 `response.Error` + `core/errors.AppError` 输出。
- 2026-05-13：新增 response 模块文档，明确成功码为 `200`，补充基于 error 的统一响应入口。
