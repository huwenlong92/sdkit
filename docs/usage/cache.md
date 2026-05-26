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
根包 `core/cache` 不直接提供 runtime `Use`，业务侧继续使用 `cache.Default()`、`cache.From(app)` 或对象缓存 helper。根包的 `Key/From/Bind` 约定统一放在 `binding.go`；真正的 runtime `Use` 只在 `core/cache/facade/use.go`。缓存实例创建逻辑统一由 `cache.NewFromConfig` 负责，`facade.Use` 只负责 runtime 注册和绑定。

`cachecap.Use()` 默认是内部底座能力。只有需要把 cache capability 展示给外部启动信息或 CLI 时，才传入 `cachecap.WithExternal()`。未传 `WithConfig` / `WithConfigLoader` 时不会从 `core/config.V` 隐式读取配置，而是使用默认 prefix 和可用 Redis；Redis 不存在时使用内存缓存。

```go
cache.Init(&cache.Config{
    Prefix: "cache:",
})
```

`cache.Init` 不再负责连接 Redis。需要 Redis 时，应在入口层先初始化 `core/redis`，`cache.Init` 会复用已有的 `core/redis.RDB`。

## 使用默认缓存

底层 `cache.Cache` 存取字符串，适合计数器、简单字符串和批量字符串读写：

```go
if err := cache.Set(ctx, "key", "value", time.Minute); err != nil {
    return err
}

value, err := cache.Get(ctx, "key")
if err != nil {
    return err
}

if err := cache.Del(ctx, "key"); err != nil {
    return err
}
```

底层 `Get` 对不存在的 key 返回空字符串和 nil error。对象缓存需要区分 miss 时，使用下面的 JSON helper。

## 对象缓存

业务对象使用 `core/cache` 的 JSON helper，内部统一走 `core/jsonx`：

```go
key := userCacheKey(uid)

if err := cache.SetJSON(ctx, key, user, time.Minute*10); err != nil {
    return err
}

var cached User
ok, err := cache.GetJSON(ctx, key, &cached)
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
func userCacheKey(uid int64) string {
    return fmt.Sprintf("user:%d", uid)
}

func GetUser(ctx context.Context, uid int64) (User, error) {
    return cache.Remember(ctx, userCacheKey(uid), time.Minute*10, func() (User, error) {
        return repo.GetUserByID(ctx, uid)
    })
}
```

指定实例时使用 `RememberWith`：

```go
user, err := cache.RememberWith(ctx, c, userCacheKey(uid), time.Minute*10, func() (User, error) {
    return repo.GetUserByID(ctx, uid)
})
if err != nil {
    return err
}
```

`Remember` 的回源函数只在缓存 miss 时执行。同一个缓存实例和 key 的并发 miss 会合并为一次回源；回源失败时错误会返回给调用方，不会写入缓存。

适合读多写少、允许短时间缓存的数据，例如用户资料、配置、字典、权限菜单。不要把有副作用的操作放进回源函数，例如扣款、发短信、写状态。

## Key

`core/cache` 不提供 key helper。业务 key 命名由应用或业务模块自己定义。

推荐在业务包里封装清楚：

```go
func userCacheKey(uid int64) string {
    return fmt.Sprintf("user:%d", uid)
}
```

简单场景也可以直接写明确的 key 字符串。core 不关心 user、session、tenant 等业务命名。

## 自定义实例

未传 Redis 时使用内存缓存：

```go
c := cache.New(cache.WithPrefix("local:"))
defer c.Close()

if err := cache.SetWith(ctx, c, "key", "value", time.Minute); err != nil {
    return err
}
```

传入独立 Redis client 时使用 Redis 后端：

```go
c := cache.New(
    cache.WithRedis(redis.RDB),
    cache.WithPrefix("admin:cache:"),
)
defer c.Close()
```

入口层已经同时持有 cache 配置和 Redis client 时，可以直接复用配置工厂：

```go
c := cache.NewFromConfig(&cfg.Cache, redis.RDB)
defer c.Close()
```

## 批量读写

底层接口支持批量字符串读写：

```go
err := cache.Sets(ctx, map[string]string{
    "a": "1",
    "b": "2",
}, time.Minute)
if err != nil {
    return err
}

values, missing := cache.Gets(ctx, []string{"a", "b", "c"})
```

`Gets` 返回已命中的 map 和未命中的 key 列表。

## 计数器和 TTL

```go
n, err := cache.Incr(ctx, "counter")
if err != nil {
    return err
}

ttl, err := cache.TTL(ctx, "counter")
if err != nil {
    return err
}

if err := cache.Expire(ctx, "counter", time.Minute); err != nil {
    return err
}
```

TTL 语义对齐 Redis：

- `-2` 表示 key 不存在
- `-1` 表示 key 永不过期
- 大于 0 表示剩余过期时间

`Expire(ctx, key, ttl)` 用于修改已有 key 的过期时间；`ttl <= 0` 表示清掉过期时间，变成不过期。删除 key 使用 `Del` 或 `Delete`。

## 接口

```go
type Cache interface {
    Get(ctx context.Context, key string) (string, error)
    Set(ctx context.Context, key, value string, ttl time.Duration) error
    Del(ctx context.Context, keys ...string) error
    Exists(ctx context.Context, keys ...string) (int64, error)
    Incr(ctx context.Context, key string) (int64, error)
    TTL(ctx context.Context, key string) (time.Duration, error)
    Expire(ctx context.Context, key string, ttl time.Duration) error
    Gets(ctx context.Context, keys []string) (map[string]string, []string)
    Sets(ctx context.Context, values map[string]string, ttl time.Duration) error
    Delete(ctx context.Context, keys []string) error
    Close() error
}
```

包级方法默认使用已经初始化的 default cache；需要指定实例时使用对应 `With` 方法：

```go
cache.GetWith(ctx, c, key)
cache.SetWith(ctx, c, key, value, ttl)
cache.DelWith(ctx, c, key)
cache.ExistsWith(ctx, c, key)
cache.IncrWith(ctx, c, key)
cache.TTLWith(ctx, c, key)
cache.ExpireWith(ctx, c, key, ttl)
cache.GetsWith(ctx, c, keys)
cache.SetsWith(ctx, c, values, ttl)
cache.DeleteWith(ctx, c, keys)
```

## 注意事项

- 字符串缓存优先使用 `cache.Set` / `cache.Get` / `cache.Del` 等包级方法。
- 对象缓存优先使用 `cache.SetJSON` / `cache.GetJSON` / `cache.Remember`。
- 业务代码不要直接处理 `redis.Nil`。
- 业务代码不要直接使用 `sonic` 做缓存 JSON 编解码。
- `Remember` 的回源函数必须能安全重复执行，不能依赖副作用只发生一次。
- 内存缓存只适合本地、测试和 Redis 不可用时降级，不提供跨进程一致性。
- Redis 初始化、`redis.Default` / `redis.RDB`、独立客户端、key 前缀和 pipeline 使用方式统一放在 [redis.md](redis.md)。
