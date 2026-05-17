# 自定义验证规则

## 初始化

HTTP runtime host 应通过 runtime capability 初始化 validator：

```go
app.RegisterCapabilities(
    validatorfacade.Use(),
)
```

`validatorfacade.Use()` 只注册 Gin binding validator 的 JSON 字段名、中文翻译和自定义规则，不持有外部资源，关闭阶段是 no-op。`validator.Init()` 内部幂等，可以被测试或旧入口重复调用；新增 runtime 接线优先使用 capability，并让 HTTP 服务声明依赖 `validator`。

## 快速使用

handler 中统一使用 helper，不再手写 `ShouldBind...` 后再调用 `HandlerValidatorError`：

```go
func CreateUser(c *gin.Context) {
    var req struct {
        Phone    string `json:"phone" binding:"required,mobile"`
        Username string `json:"username" binding:"required,username"`
    }
    if err := validator.BindJSON(c, &req); err != nil {
        response.Fail(c, err)
        return
    }

    response.Success(c, nil)
}
```

当前提供：

```go
validator.BindJSON(c, &req)
validator.BindQuery(c, &req)
validator.BindForm(c, &req)
```

这些 helper 会直接返回统一的 `*errors.AppError`，调用方交给 `response.Fail` 即可。

在 handler 的 binding tag 中用规则名即可：

```go
func CreateUser(c *gin.Context) {
    var req struct {
        Phone    string `json:"phone"    binding:"required,mobile"`
        Username string `json:"username" binding:"required,username"`
        Password string `json:"password" binding:"required,password"`
        IDCard   string `json:"idcard"   binding:"idcard"`
        Landline string `json:"landline" binding:"phone"`
        Date     string `json:"date"     binding:"date"`
        Nickname string `json:"nickname" binding:"noemoji"`
    }
    if err := validator.BindJSON(c, &req); err != nil {
        response.Fail(c, err)
        return
    }
}
```

## 规则列表

| 规则 | 说明 | 示例 |
|------|------|------|
| `mobile` | 中国大陆手机号 | `binding:"required,mobile"` |
| `username` | 5-16位字母/数字/下划线 | `binding:"required,username"` |
| `password` | 6-20位，包含字母和数字 | `binding:"required,password"` |
| `idcard` | 中国大陆18位身份证号 | `binding:"idcard"` |
| `phone` | 固定电话（含区号） | `binding:"phone"` |
| `date` | 日期格式 YYYY-MM-DD | `binding:"date"` |
| `noemoji` | 禁止包含 emoji | `binding:"noemoji"` |
| `between` | 数值区间（含边界） | `binding:"between=1,100"` |
| `in` | 必须是枚举值中的一个（数字/字符串都支持） | `binding:"in=1,2,3"` 或 `binding:"in=apple,banana,orange"` |
| `required` | 必填（内置） | `binding:"required"` |
| `min`/`max` | 长度/数值范围（内置） | `binding:"min=6,max=20"` |
| `email` | 邮箱（内置） | `binding:"email"` |
| `url` | URL（内置） | `binding:"url"` |
| `ip` | IP 地址（内置） | `binding:"ip"` |
| `oneof` | 枚举值（内置） | `binding:"oneof=1 2 3"` |

错误消息自动显示 JSON 字段名 + 中文描述，如 `"password需要6-20位，包含字母和数字"`。

## 新增规则

在 `core/validator/custom/` 下新建文件，用 `init()` 注册 `custom.Rule`：

```go
// core/validator/custom/xxx.go
package custom

import "github.com/go-playground/validator/v10"

func init() {
    Register(Rule{
        Tag:       "xxx",
        Validate:  func(fl validator.FieldLevel) bool { return /* true=通过 */ },
        Translate: "{0}格式不正确",
    })
}
```

- `Tag` — binding 中使用的规则名
- `Validate` — 验证函数，返回 true 表示通过
- `Translate` — 错误消息模板，`{0}` 会被替换为 JSON 字段名

新增文件后无需修改任何代码，`init()` 自动注册。
