# Validator 模块迁移说明

`core/validator` 已从 sdkit core 中移除。Gin 参数绑定、中文翻译和业务自定义校验规则属于应用 HTTP 边界，不再由 core 维护。

## 当前边界

- `core/errors` 继续提供 `AppError` 和错误码。
- 应用层负责 Gin binding validator 初始化、字段名翻译、自定义 tag 注册和 bind helper。
- sdkitgo 当前实现位置：`app/http/validator`。
- sdkitgo runtime capability 位置：`app/infra/capabilities/validator`。

## 迁移原因

业务自定义规则如 `tenant_code`、`order_no`、`slug`、`scene` 等不应通过修改 sdkit core 增加。迁出后，业务仓库可以直接在应用层新增规则文件，并通过应用自己的 runtime capability 初始化。

## 更新记录

- 2026-05-19：移除 `core/validator` 和 `core/validator/facade`，迁移到 sdkitgo 应用层。
- 2026-05-17：历史版本新增 `core/validator/facade.Use()` runtime capability。
- 2026-05-15：历史版本新增 `BindJSON`、`BindQuery`、`BindForm`。
