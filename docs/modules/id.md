# ID 工具设计

`pkg/id` 提供基于 KSUID 的通用标识生成原语。它可以脱离 sdkit runtime 独立使用，因此属于 `pkg`，不属于 `core` 或安全凭证模块。

## API

```go
func New() (string, error)
func NewPrefixed(prefix string) (string, error)
```

## 边界

`pkg/id` 负责：

- 生成全局唯一、按生成时间大致有序的 KSUID 字符串
- 在调用方提供的前缀后拼接 KSUID
- 返回随机源错误，不通过 panic 隐藏失败

`pkg/id` 不负责：

- 定义业务实体及其前缀
- 注册 GORM Hook
- 生成密码、Token 或应用密钥
- 读取配置或参与 runtime 生命周期

## 与其他 ID 能力的区别

- `pkg/hashid` 将已有整数编码为公开短 ID，并可解码回整数。
- `core/requestid` 和 `core/tracking` 生成请求链路相关标识。
- `pkg/id` 生成独立于数据库主键和请求链路的资源标识。

## 更新记录

- 2026-09-28：新增 `pkg/id`，统一通用 KSUID 生成能力。
