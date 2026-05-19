# JSONX 模块方案

## 作用

`core/jsonx` 是项目内唯一的 JSON 编解码入口。业务代码和 core 模块需要序列化或反序列化 JSON 时，统一依赖 `core/jsonx`，不要直接依赖 `encoding/json`、`github.com/bytedance/sonic` 或 `jsoniter`。

当前底层实现使用 Sonic。后续如果需要切换 JSON 实现，只应调整 `core/jsonx`，避免业务侧跟随改动。

## 初始化

`core/jsonx` 无需初始化。

直接 import 后使用：

```go
import "github.com/huwenlong92/sdkit/core/jsonx"
```

## 配置项

暂无配置项。

如果后续需要暴露 HTML escape、数字精度等选项，应先评估是否会影响现有响应、缓存和队列 payload 的稳定性。

## 对外接口

```go
func Marshal(v any) ([]byte, error)
func Unmarshal(data []byte, v any) error
func MarshalString(v any) (string, error)
func Valid(data []byte) bool
```

接口语义与常见 JSON 库保持一致：

- `Marshal` 返回 JSON 字节。
- `Unmarshal` 将 JSON 字节写入目标对象。
- `MarshalString` 返回 JSON 字符串。
- `Valid` 判断字节是否是合法 JSON。

## 模块集成

已经通过 `core/jsonx` 处理 JSON 的模块：

| 模块 | 场景 |
|------|------|
| 应用层 response | HTTP JSON 响应 |
| `core/cache` | 对象缓存 helper |
| `core/queue` | 任务 payload 编解码 |
| `core/session` | Redis session extra 字段 |
| `core/accesslog` | 请求头、请求体摘要和表单字段记录 |
| `crontab` | 任务 payload 解析 |
| `worker` | 失败任务 payload 处理 |

应用层 validator 中识别 `encoding/json.UnmarshalTypeError` 是为了把 Gin bind 的类型错误转换成统一响应，不承担业务 JSON 编解码职责。

## 中间件

无。

HTTP 响应由应用层 response 模块统一封装，业务 handler 不直接调用 `c.JSON(...)`。

## Hook

无。

## 使用示例

```go
type Profile struct {
    ID   int64  `json:"id"`
    Name string `json:"name"`
}

b, err := jsonx.Marshal(Profile{ID: 1, Name: "admin"})
if err != nil {
    return err
}

var dst Profile
if err := jsonx.Unmarshal(b, &dst); err != nil {
    return err
}
```

缓存对象使用 `core/cache` helper，不在业务侧手写 JSON：

```go
_ = cache.Set(ctx, cache.Default(), cache.UserKey(uid), profile, ttl)

var cached Profile
ok, err := cache.GetJSON(ctx, cache.Default(), cache.UserKey(uid), &cached)
```

响应使用应用层 response：

```go
response.Success(c, data)
response.Error(c, apperrors.NewCodeWithData(apperrors.CodeBusinessUnavailable, "参数错误", nil))
```

## 注意事项

- 新代码不要直接 import `encoding/json`、`github.com/bytedance/sonic` 或 `jsoniter` 做业务 JSON 编解码。
- 新 handler 不要直接调用 `c.JSON(...)` 返回业务响应。
- Gin request bind 继续使用 `ShouldBindQuery` 和 `ShouldBindBodyWith(&req, binding.JSON)`，本模块不替换 bind 行为。
- JSON tag 是跨 response、cache、queue、session 的稳定契约，修改 tag 需要评估数据契约影响。
- `[]byte` 和 `string` payload 在 `core/queue` 中有特殊处理，详见 `docs/modules/queue.md`。

## 已知限制

- 当前没有自定义 decoder 选项。
- 当前没有提供流式 encoder/decoder。
- 当前不改写 Gin 默认 JSON binding。
- Sonic 的行为差异如果影响边界类型，需要先在 `core/jsonx` 增加集中测试，再决定是否调整行为。

## 更新记录

- 2026-05-10：补齐 `core/jsonx` 模块方案，明确统一入口、集成模块和 request bind 边界。

## Breaking Changes

暂无。
