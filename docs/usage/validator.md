# Validator 使用说明

`core/validator` 已移出 sdkit core。应用层需要自行维护 Gin bind helper、中文翻译和自定义规则。

sdkitgo 当前使用：

```go
import "sdkitgo/app/http/validator"
```

runtime capability 当前使用：

```go
import validatorcap "sdkitgo/app/infra/capabilities/validator"

app.RegisterCapabilities(
    validatorcap.Use(),
)
```

handler 中统一使用应用层 helper：

```go
if err := validator.BindJSON(c, &req); err != nil {
    response.Fail(c, err)
    return
}
```

新增自定义规则时，在业务仓库新增文件，例如：

```go
// app/http/validator/custom/order_no.go
package custom

import "github.com/go-playground/validator/v10"

func init() {
    Register(Rule{
        Tag:       "order_no",
        Validate:  func(fl validator.FieldLevel) bool { return true },
        Translate: "{0}订单号格式不正确",
    })
}
```

sdkit core 不再接收业务校验 tag。
