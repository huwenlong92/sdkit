# Cache 缓存

`core/cache` 提供统一缓存入口，支持内存和 Redis 后端。Redis 基础设施由 `core/redis` 统一初始化，详见 [redis.md](redis.md)。

方案文档见 [../modules/cache.md](../modules/cache.md)。

## 初始化

`core/cache` 保留缓存实现和业务 API；Runtime 接入门面放在 `core/cache/facade`：

```go
import cachecap "github.com/huwenlong92/sdkit/core/cache/facade"

app := runtime.New()
app.RegisterCapabilities(
    cachecap.Use(cachecap.WithConfig(appCfg.Cache)),
)
```

通常由 `bootstrap` 初始化。`bootstrap` 会先按需初始化 `core/redis`，再通过 `core/cache/facade` 注册 cache 能力。cache 会自动绑定已存在的 Redis；Redis 不可用时降级为内存缓存。
根包 `core/cache` 不直接提供 runtime `Use`，业务侧继续使用 `cache.Default()`、`cache.From(app)` 或对象缓存 helper。根包的 `Key/From/Bind` 约定统一放在 `binding.go`；真正的 runtime `Use` 只在 `core/cache/facade/use.go`。

```go
cache.Init(&cache.Config{
    Prefix: "cache:",
})
```

`cache.Init` 不再负责连接 Redis。需要 Redis 时，应在入口层先初始化 `core/redis`。

## 使用默认缓存

底层 `cache.Cache` 存取字符串，适合计数器、简单字符串和批量字符串读写：

```go
c := cache.Default()

if err := c.Set(ctx, "key", "value", time.Minute); err != nil {
    return err
}

value, err := c.Get(ctx, "key")
if err != nil {
    return err
}

if err := c.Del(ctx, "key"); err != nil {
    return err
}
```

底层 `Get` 对不存在的 key 返回空字符串和 nil error。对象缓存需要区分 miss 时，使用下面的 JSON helper。

## 对象缓存

业务对象使用 `core/cache` 的 JSON helper，内部统一走 `core/jsonx`：

```go
key := cache.UserKey(uid)

if err := cache.Set(ctx, cache.Default(), key, user, time.Minute*10); err != nil {
    return err
}

var cached User
err := cache.Get(ctx, cache.Default(), key, &cached)
if errors.Is(err, cache.ErrNotFound) {
    // cache miss
    return nil
}
if err != nil {
    return err
}
```

偏好 `ok` 风格时可使用：

```go
ok, err := cache.GetJSON(ctx, cache.Default(), key, &cached)
if err != nil {
    return err
}
if !ok {
    // cache miss
}
```

## Remember

`Remember` 统一处理读取、miss 回源、写回和 `singleflight` 防击穿：

```go
user, err := cache.Remember(ctx, cache.Default(), cache.UserKey(uid), time.Minute*10, func() (User, error) {
    return repo.GetUserByID(ctx, uid)
})
if err != nil {
    return err
}
```

`Remember` 的回源函数只在缓存 miss 时执行。同一个缓存实例和 key 的并发 miss 会合并为一次回源；回源失败时错误会返回给调用方，不会写入缓存。

## Key

使用 `cache.Key(...)` 或约定 helper 生成业务 key：

```go
cache.Key("tenant", tenantID, "user", uid)
cache.UserKey(uid)
cache.SessionKey(sid)
```

`cache.Key` 会去掉片段首尾多余的 `:`：

```go
key := cache.Key("tenant:", tenantID, ":profile")
// tenant:1001:profile
```

业务模块可以在自己的包内封装更具体的 key helper，调用点不要手写散落的 Redis key 字符串。

## 自定义实例

未传 Redis 时使用内存缓存：

```go
c := cache.New(cache.WithPrefix("local:"))
defer c.Close()
```

传入独立 Redis client 时使用 Redis 后端：

```go
c := cache.New(
    cache.WithRedis(redis.RDB),
    cache.WithPrefix("admin:cache:"),
)
defer c.Close()
```

## 批量读写

底层接口支持批量字符串读写：

```go
err := c.Sets(ctx, map[string]string{
    "a": "1",
    "b": "2",
}, time.Minute)
if err != nil {
    return err
}

values, missing := c.Gets(ctx, []string{"a", "b", "c"})
```

`Gets` 返回已命中的 map 和未命中的 key 列表。

## 计数器和 TTL

```go
n, err := c.Incr(ctx, "counter")
if err != nil {
    return err
}

ttl, err := c.TTL(ctx, "counter")
if err != nil {
    return err
}
```

TTL 语义对齐 Redis：

- `-2` 表示 key 不存在
- `-1` 表示 key 永不过期
- 大于 0 表示剩余过期时间

## 接口

```go
type Cache interface {
    Get(ctx context.Context, key string) (string, error)
    Set(ctx context.Context, key, value string, ttl time.Duration) error
    Del(ctx context.Context, keys ...string) error
    Exists(ctx context.Context, keys ...string) (int64, error)
    Incr(ctx context.Context, key string) (int64, error)
    TTL(ctx context.Context, key string) (time.Duration, error)
    Gets(ctx context.Context, keys []string) (map[string]string, []string)
    Sets(ctx context.Context, values map[string]string, ttl time.Duration) error
    Delete(ctx context.Context, keys []string) error
    Close() error
}
```

## 注意事项

- 对象缓存优先使用 `cache.Set` / `cache.Get` / `cache.Remember`。
- 业务代码不要直接处理 `redis.Nil`。
- 业务代码不要直接使用 `sonic` 做缓存 JSON 编解码。
- `Remember` 的回源函数必须能安全重复执行，不能依赖副作用只发生一次。
- 内存缓存只适合本地、测试和 Redis 不可用时降级，不提供跨进程一致性。
- Redis 初始化、`redis.Default` / `redis.RDB`、独立客户端、key 前缀和 pipeline 使用方式统一放在 [redis.md](redis.md)。
