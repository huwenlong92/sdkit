# JSON

项目统一通过 `core/jsonx` 做 JSON 序列化，底层当前使用 Sonic。

不要在业务代码中直接使用：

- `encoding/json`
- `github.com/bytedance/sonic`
- `jsoniter`

使用：

```go
b, err := jsonx.Marshal(v)
err = jsonx.Unmarshal(b, &dst)
s, err := jsonx.MarshalString(v)
ok := jsonx.Valid(b)
```

## 基本使用

```go
type Profile struct {
    ID   int64  `json:"id"`
    Name string `json:"name"`
}

b, err := jsonx.Marshal(Profile{ID: 1, Name: "admin"})
if err != nil {
    return err
}

var profile Profile
if err := jsonx.Unmarshal(b, &profile); err != nil {
    return err
}
```

## Response

HTTP 响应统一使用应用层 response：

```go
response.Success(c, data)
response.Error(c, apperrors.NewCodeWithData(apperrors.CodeBusinessUnavailable, "参数错误", nil))
response.AbortJSON(c, 401, gin.H{"err_code": 4001, "msg": "用户未登录"})
```

不要在业务 handler 中直接使用 `c.JSON(...)`。

## Cache

缓存对象值使用 `core/cache` 的 JSON helper，内部统一走 `core/jsonx`：

```go
_ = cache.SetJSON(ctx, "profile:1", profile, time.Minute)

var profile Profile
ok, err := cache.GetJSON(ctx, "profile:1", &profile)
```

## Queue

任务 payload 使用结构体时，`core/queue` 会通过 `core/jsonx` 序列化：

```go
task := queue.NewTask("user:sync", UserSyncPayload{
    UserID: 1001,
})
_, err := queue.Enqueue(ctx, task)
```

payload 推荐使用结构体并写清 JSON tag。`[]byte` 和 `string` payload 会按队列模块规则处理，详见 `docs/modules/queue.md`。

## Session

Redis session 的 `Extra` 字段会通过 `core/jsonx` 编解码。复杂结构建议先序列化成 JSON 字符串再放入 `Extra`，读取时再反序列化：

```go
b, err := jsonx.Marshal(profile)
if err != nil {
    return err
}
sess.SetExtra("profile", string(b))

var dst Profile
if err := jsonx.Unmarshal([]byte(sess.GetExtraString("profile")), &dst); err != nil {
    return err
}
```

## Access Log

`core/accesslog` 会通过 `core/jsonx` 处理请求头、JSON 请求体摘要和 multipart/form 字段记录。敏感头和敏感字段仍由 accesslog 自己过滤，不要在业务侧重复记录原始敏感数据。

## Request Bind

Gin request bind 暂时保留，不由 `core/jsonx` 接管：

- GET 使用 `ShouldBindQuery`。
- POST JSON 使用 `ShouldBindBodyWith(&req, binding.JSON)`。

应用层 validator 中识别 `json.UnmarshalTypeError` 是为了把 Gin bind 的类型错误转换成统一响应，不代表业务代码可以直接使用 `encoding/json`。

## 迁移检查

新增或修改 JSON 相关代码时检查：

- 业务 JSON 编解码是否只 import `github.com/huwenlong92/sdkit/core/jsonx`。
- HTTP 响应是否使用应用层 response。
- 对象缓存是否使用 `cache.Set` / `cache.Get` / `cache.GetJSON`。
- 队列 payload 是否通过 `queue.NewTask` 和 `queue.Enqueue`。
- 是否避免把密码、token、cookie 等敏感字段写入日志。
