# Errors 模块方案

## 作用

`core/errors` 是项目业务错误模型和错误码的集中入口。handler、middleware 和 core 模块需要向 HTTP response 映射错误时，优先返回或包装 `*errors.AppError`，再交给 `core/response` 输出统一响应结构。

模块目标：

- 统一 `err_code`、`sub_code` 和展示消息的来源。
- 保留 Go 标准错误链语义，支持 `errors.Is` / `errors.As`。
- 让 validator、业务错误和未知错误可以走同一条 response 映射路径。

## 错误模型

```go
type AppError struct {
    Code    int
    SubCode string
    Message string
    Data    interface{}
    Err     error
}
```

字段语义：

| 字段 | 说明 |
|------|------|
| `Code` | 对外响应的 `err_code` |
| `SubCode` | 稳定的字符串错误标识，用于日志、前端分支和测试断言 |
| `Message` | 对外响应的 `msg` |
| `Data` | 可选响应数据 |
| `Err` | 被包装的原始错误，通过 `Unwrap()` 暴露 |

`AppError` 实现了 `Unwrap()` 和 `Is()`：

- `errors.As(err, &appErr)` 可识别错误链中的 `AppError`。
- `errors.Is(err, apperrors.ErrInternalServer)` 可按 `Code + SubCode` 匹配项目预置错误。
- 原始错误放在 `Err` 中，便于上层保留根因。

## 错误码

通用错误码：

| 常量 | 值 | 说明 |
|------|----|------|
| `CodeOK` | `200` | 成功响应 |
| `CodeBadRequest` | `400` | 参数错误 |
| `CodeUnauthorized` | `401` | 未认证 |
| `CodeForbidden` | `403` | 无权限 |
| `CodeNotFound` | `404` | 资源不存在 |
| `CodeConflict` | `409` | 资源冲突 |
| `CodeInternal` | `500` | 服务内部错误 |

项目业务错误码：

| 常量 | 值 | 说明 |
|------|----|------|
| `CodeBusinessError` | `3000` | 通用业务失败 |
| `CodeBusinessUnavailable` | `3001` | 业务对象不可用或禁用 |
| `CodeBusinessDuplicated` | `3002` | 业务对象重复 |
| `CodeBusinessProtected` | `3003` | 受保护对象不可操作 |
| `CodeBusinessConflict` | `3004` | 业务状态冲突 |
| `CodeAuthRequired` | `4001` | 登录态或令牌缺失 |
| `CodeOperationFailed` | `5001` | 异步、队列、示例能力等操作失败 |
| `CodeQueueTaskConflict` | `4091` | 队列任务重复 |

新增 handler 不要直接写 magic number。先复用现有常量；确实需要新增错误码时，先在 `core/errors` 中命名，再使用。

## 对外接口

```go
func New(code int, subCode, message string) *AppError
func NewWithData(code int, subCode, message string, data interface{}) *AppError
func NewCode(code int, message string) *AppError
func NewCodeWithData(code int, message string, data interface{}) *AppError
func Wrap(err error, code int, subCode, message string) *AppError
func WrapWithData(err error, code int, subCode, message string, data interface{}) *AppError
func SubCodeForCode(code int) string
```

使用规则：

- 有原始错误时用 `Wrap` / `WrapWithData`，不要丢失根因。
- 纯业务分支错误可用 `New` / `NewWithData`。
- 需要从数字错误码构造 `AppError` 时可用 `NewCode` / `NewCodeWithData`，由 `SubCodeForCode` 补齐稳定 `sub_code`。
- 对外消息必须可展示，不要把敏感信息写入 `Message`。
- 低层模块可以返回普通 error；HTTP 边界再决定是否包装成 `AppError`。

## 模块集成

| 模块 | 集成方式 |
|------|----------|
| `core/response` | 通过 `response.Error(c, err)` 将 `AppError` 映射为统一响应 |
| `core/validator` | 将 Gin bind 和 validator 错误转换为 `CodeBadRequest` 的 `AppError` |
| `app/admin/handler/system/auth.go` | 示例迁移：登录、用户信息错误通过 `AppError` 返回 |
| `app/api/handler/security_demo.go` | 示例迁移：安全 demo 错误通过 `AppError` 返回 |

## 更新记录

- 2026-05-13：新增 `NewCode/NewCodeWithData/SubCodeForCode`，用于通过数字 code 构造稳定 `AppError`。
- 2026-05-13：新增统一错误模型文档，明确 `AppError`、错误码常量和 response 映射边界。
