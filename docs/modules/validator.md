# Validator 模块说明

## 职责

`core/validator` 负责 Gin 参数绑定错误和 go-playground/validator 错误的统一转换。

它负责：

- 初始化中文翻译。
- 注册项目自定义校验规则。
- 将绑定和校验错误转换为 `core/errors.AppError`。
- 提供 API DX helper：`BindJSON`、`BindQuery`、`BindForm`。

## Handler 约束

API handler 中优先使用：

```go
if err := validator.BindJSON(c, &req); err != nil {
    response.Fail(c, err)
    return
}
```

不要在新 handler 中继续手写 `ShouldBind...` + `HandlerValidatorError`。旧接口迁移时保持同样规则。

## 更新记录

- 2026-05-15：新增 `BindJSON`、`BindQuery`、`BindForm`，handler 可直接返回统一错误给 `response.Fail`。
