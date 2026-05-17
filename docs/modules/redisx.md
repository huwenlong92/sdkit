# Redisx Driver 模块

## 作用

`pkg/redisx` 是 Redis driver/sdk 层，负责封装 go-redis client 的基础创建逻辑。

它负责：

- Redis client 构造
- Redis 配置结构
- key prefix helper
- hook 注入点
- maintenance notifications 握手设置

它不负责：

- 全局默认实例
- bootstrap 初始化
- capability provider
- logger / tracing / metrics 语义
- cache/session/queue/eventbus 等业务能力解释

## 依赖方向

```txt
core/redis
  ↓
pkg/redisx
  ↓
github.com/redis/go-redis/v9
```

`pkg/redisx` 禁止反向依赖 `core/*`。

## 对外接口

```go
redisx.New(cfg, opts...)
redisx.WithHooks(hooks...)
client.Ping(ctx)
client.Close()
client.Key(parts...)
```

框架内需要日志和 tracing 时，由 `core/redis` 实现 hook 并通过 `redisx.WithHooks` 注入。

## 更新记录

- 2026-05-13：Redis driver 层定位为 `pkg/redisx`；全局实例和观测治理统一由 `core/redis` 管理。
