# ID 使用指南

`pkg/id` 用于生成不依赖 sdkit runtime 的 KSUID。业务前缀由调用方定义。

## 生成 ID

```go
value, err := id.New()
if err != nil {
    return err
}
```

## 生成带业务前缀的 ID

```go
jobID, err := id.NewPrefixed("job_")
if err != nil {
    return err
}
```

`pkg/id` 不维护 `job_`、`user_` 等业务命名空间，也不负责数据库 Hook。
