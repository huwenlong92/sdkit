# HashID 使用指南

## 初始化

```go
encoder, err := hashid.New(hashid.Config{
    Salt:      "service-specific-salt",
    Alphabet:  hashid.LowerAlphabet,
    MinLength: 8,
})
if err != nil {
    return err
}
```

## 普通编码

```go
raw, err := encoder.Encode(123, 10)
values, err := encoder.Decode(raw)
```

## 带类型编码

业务侧自行定义类型常量：

```go
const UserIDType = 10000

publicID, err := encoder.EncodeTyped(12345, UserIDType)
dbID, err := encoder.DecodeTyped(publicID, UserIDType)
```

类型不匹配时返回 `hashid.ErrTypeMismatch`。

## 注意事项

- salt 必须由业务配置提供，不能硬编码公共默认值。
- HashID 不是加密，不能替代鉴权和权限校验。
- 不要把手机号、身份证、token 等敏感原文放进 HashID。
