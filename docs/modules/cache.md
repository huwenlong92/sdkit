# Cache 模块方案

## 目标

`core/cache` 提供项目统一缓存 capability，业务代码不要直接散落 Redis `Get` / `Set` 或手写 JSON 序列化。

目标能力：

- 统一缓存抽象
- 支持内存和 Redis 后端
- Redis 不可用时降级为内存缓存
- 对象缓存统一通过 `core/jsonx` 做 JSON 编解码
- cache miss 使用统一错误 `ErrNotFound`
- 支持泛型 `Remember` 模式
- 使用 `singleflight` 降低同一 key 并发回源

## 模块边界

`core/cache` 负责：

- 创建和维护默认缓存实例
- 暴露 `core/cache.Cache` 字符串缓存接口
- 提供对象缓存 JSON helper
- 提供 `Remember` 回源写回能力
- 按配置和 Redis client 选择 Redis 或内存 backend

`core/cache` 不负责：

- 初始化 Redis 基础设施
- 持有 Redis 全局 client 生命周期
- 定义业务缓存 key 的完整命名体系
- 决定业务数据 TTL 策略
- 吞掉回源错误
- 承担 `core/redis` 的 Redis 基础设施能力
- 承担队列、会话、限流等模块的专用存储封装

## 当前目录

```txt
core/cache/
  binding.go
  cache.go
  errors.go
  facade/
    config.go
    use.go
  json.go
  operations.go
  remember.go

pkg/cache/
  cache.go
  memory/
    store.go
  redis/
    store.go
```

## Runtime Capability

`core/cache` 是缓存实现包，Runtime Capability 接入层统一放在 `core/cache/facade`：

```go
import cachecap "github.com/huwenlong92/sdkit/core/cache/facade"

runtimeApp.RegisterCapabilities(
    cachecap.Use(cachecap.WithConfig(cfg.Cache)),
)
```

bootstrap 使用 `cachecap.WithConfigLoader(...)`，确保配置能力先初始化，再由 `cachecap.Use` 读取最终配置。cache capability 依赖 Redis 时只声明可选依赖，Redis 不启用时自动使用内存缓存。
根包不实现 runtime `Use`；根包只保留缓存实现、默认缓存入口和 `Bind(app, cache)` 容器绑定能力，避免与 facade 重复实现 capability 注册。根包的 `KeyCache`、`From(app)`、`Bind(app, cache)` 统一放在 `binding.go`；`Use(...)`、`WithConfig(...)`、生命周期关闭只允许放在 `facade/use.go`。缓存实例创建统一走 `core/cache.NewFromConfig`，facade 不再维护 Redis/memory 分支。

`cachecap.Use()` 默认按框架底座能力处理，metadata `Internal=true`。需要在启动信息或 CLI 中对外展示 cache capability 时，调用方必须显式传入 `cachecap.WithExternal()`。Cache facade 不默认读取 `core/config.V`；配置由 `WithConfig` / `WithConfigLoader` 显式提供。未传配置时使用 `core/cache.NewFromConfig(nil, rdb)`，即默认 prefix 和可用 Redis，否则内存缓存。

## 核心接口

底层缓存接口位于 `pkg/cache`，当前缓存值是字符串：

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

业务侧统一使用 `core/cache.Cache`，不要直接依赖 `pkg/cache` 或具体 backend 包。

常用字符串缓存可以直接使用包级默认入口：

```go
func Get(ctx context.Context, key string) (string, error)
func Set(ctx context.Context, key, value string, ttl time.Duration) error
func Del(ctx context.Context, keys ...string) error
func Exists(ctx context.Context, keys ...string) (int64, error)
func Incr(ctx context.Context, key string) (int64, error)
func TTL(ctx context.Context, key string) (time.Duration, error)
func Expire(ctx context.Context, key string, ttl time.Duration) error
func Gets(ctx context.Context, keys []string) (map[string]string, []string)
func Sets(ctx context.Context, values map[string]string, ttl time.Duration) error
func Delete(ctx context.Context, keys []string) error
```

需要指定 cache 实例时使用对应 `With` 方法：

```go
func GetWith(ctx context.Context, c Cache, key string) (string, error)
func SetWith(ctx context.Context, c Cache, key, value string, ttl time.Duration) error
func DelWith(ctx context.Context, c Cache, keys ...string) error
func ExistsWith(ctx context.Context, c Cache, keys ...string) (int64, error)
func IncrWith(ctx context.Context, c Cache, key string) (int64, error)
func TTLWith(ctx context.Context, c Cache, key string) (time.Duration, error)
func ExpireWith(ctx context.Context, c Cache, key string, ttl time.Duration) error
func GetsWith(ctx context.Context, c Cache, keys []string) (map[string]string, []string)
func SetsWith(ctx context.Context, c Cache, values map[string]string, ttl time.Duration) error
func DeleteWith(ctx context.Context, c Cache, keys []string) error
```

对象缓存使用 `core/cache` 包提供的 JSON helper：

```go
func SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error
func SetJSONWith(ctx context.Context, c Cache, key string, value any, ttl time.Duration) error
func GetJSON(ctx context.Context, key string, dst any) (bool, error)
func GetJSONWith(ctx context.Context, c Cache, key string, dst any) (bool, error)
func DeleteJSON(ctx context.Context, key string) error
func DeleteJSONWith(ctx context.Context, c Cache, key string) error
func Remember[T any](ctx context.Context, key string, ttl time.Duration, fn func() (T, error)) (T, error)
func RememberWith[T any](ctx context.Context, c Cache, key string, ttl time.Duration, fn func() (T, error)) (T, error)
```

## 初始化方案

`bootstrap` 初始化 Redis 后，会调用 cache 初始化：

```go
cache.Init(&cache.Config{
    Prefix: "cache:",
})
```

`cache.Init` 的行为：

1. 复用已经初始化的 `core/redis.RDB`
2. Redis 可用时使用 `pkg/cache/redis`
3. Redis 不可用时使用 `pkg/cache/memory`
4. 默认 key 前缀为 `cache:`

`cache.Init` 和 `core/cache/facade.Use` 都复用同一个工厂：

```go
func NewFromConfig(cacheCfg *Config, client *redis.Client) Cache
```

`core/cache` 负责根据配置和 Redis client 创建实例；`facade.Use` 只负责从 runtime container 取 Redis、绑定 cache 到 container，以及处理 capability 生命周期。

全局默认实例通过 `cache.Default()` 获取。关闭入口为 `cache.Close()`。

## Redis 后端

Redis 后端只封装缓存语义：

- 所有 key 通过 `Prefix + key` 写入 Redis
- `redis.Nil` 在底层 `Get` 中转换为空字符串和 nil error
- `MGet` 用于批量读取
- `Pipeline` 用于批量写入
- `Expire` 用于修改已有 key 过期时间，`ttl <= 0` 时通过 `Persist` 清掉过期时间
- `Close` 不关闭外部传入的 Redis client，Redis 生命周期由 `core/redis` 管理

业务代码需要 Redis 基础能力时优先走 `core/redis`，不要在业务侧创建 `redis.NewClient(...)`。

## 内存后端

内存后端用于本地、测试和 Redis 不可用时降级：

- 使用 `sync.RWMutex` 保护 map
- 支持 TTL
- 支持 `Expire` 修改已有 key 过期时间，`ttl <= 0` 清掉过期时间
- 后台 cleanup goroutine 每分钟清理过期 key
- `Close` 会通知 cleanup goroutine 退出

内存后端只适合单进程内缓存，不提供跨进程一致性。

## JSON 方案

对象缓存统一使用：

```txt
core/cache
  -> core/jsonx
  -> sonic
```

业务代码不要为了缓存对象直接调用 `jsonx.Marshal`、`sonic.Marshal` 或 `redis.Set`。字符串缓存使用 `cache.Set` / `cache.Get`；对象写入使用 `cache.SetJSON`，对象读取使用 `cache.GetJSON`。

## Cache Miss

类型化 helper 的 miss 统一返回：

```go
var ErrNotFound = errors.New("cache not found")
```

判断方式：

```go
if errors.Is(err, cache.ErrNotFound) {
    // cache miss
}
```

也可以使用：

```go
cache.IsNotFound(err)
```

底层 `Cache.Get` 面向字符串缓存，miss 返回空字符串和 nil error。需要区分 miss 的对象缓存场景，应使用类型化 helper。

## Remember 方案

`Remember` 统一处理：

1. 先读缓存
2. miss 时进入 `singleflight`
3. `singleflight` 内再次读缓存
4. 仍 miss 时执行回源函数
5. 回源成功后写回缓存
6. 返回类型化结果

同一个缓存实例和 key 的并发 miss 只执行一次回源函数。`singleflight` key 包含缓存实例指针和业务 key，避免不同缓存实例之间互相影响。

`Remember` 不吞掉回源错误，也不缓存错误结果。

## Key 方案

`core/cache` 不提供业务 key helper，也不定义 user、session、tenant 等命名规则。

原因：

- key 命名属于业务边界，core 不应该知道业务实体。
- 不同应用、租户、模块的 key 前缀策略可能不同。
- core 只负责缓存读写、TTL、过期和对象序列化。

业务侧需要 key helper 时，在应用或业务模块里定义：

```go
func userCacheKey(uid int64) string {
    return fmt.Sprintf("user:%d", uid)
}
```

简单场景可以直接使用明确的 key 字符串。需要统一规范时，应在应用层建立 `cachekey` 包或在对应业务包内封装。

## 使用约束

- context 必须从调用链透传。
- error 必须返回给上层处理。
- cache key 由业务侧定义，core 不提供 `UserKey`、`SessionKey` 这类 helper。
- 字符串缓存优先使用 `cache.Set` / `cache.Get` / `cache.Del` 等包级方法。
- 修改已有 key 过期时间使用 `cache.Expire`；删除 key 使用 `cache.Del` 或 `cache.Delete`。
- 对象缓存优先使用 `cache.SetJSON` / `cache.GetJSON` / `cache.Remember`。
- 业务代码不要直接处理 `redis.Nil`。
- 业务代码不要直接使用 `sonic` 做缓存 JSON 编解码。
- `Remember` 的回源函数必须可重入，不能依赖“必定只执行一次”的副作用。
- TTL 由业务按数据更新频率和一致性要求决定。
- 内存缓存只作为本地和降级能力，不作为分布式一致性方案。

## 已知限制

- 底层 `Cache.Get` 使用空字符串表示 miss，因此字符串值为空的场景不能只依赖底层接口区分是否存在。
- 当前没有提供按 pattern 删除能力。
- 当前没有提供分布式锁能力。
- 当前没有提供 Redis Cluster/Sentinel 专用配置。

## 更新记录

- 2026-05-26：Cache runtime facade 默认作为 internal 底座能力，新增 `WithExternal()` 显式对外展示；移除 `core/config.V` 隐式读取，无配置时使用内存缓存默认配置。
- 2026-05-19：删除 `core/cache/key.go`，cache core 不再提供 `Key/UserKey/SessionKey`，业务 key helper 由应用层自行定义。
- 2026-05-19：补齐 `core/cache` 包级字符串缓存方法 `Get/Set/Del/Exists/Incr/TTL/Expire/Gets/Sets/Delete` 及对应 `With` 方法；对象缓存 helper 改为 `SetJSON/GetJSON/DeleteJSON/Remember`。
- 2026-05-19：新增 `core/cache.NewFromConfig`，`cache.Init` 和 `core/cache/facade.Use` 统一复用根包工厂，移除 facade 内部重复的 Redis/memory 创建分支。
- 2026-05-19：删除 `core/cache/facade` 中重复转发的 `Default/From/Init/Close/New/WithRedis/WithPrefix`，facade 只保留 runtime capability 接入和必要类型别名。
- 2026-05-16：`core/cache/facade` 作为唯一 Runtime Capability 接入层；根包移除重复的 `Use/UseOption`，保留 `Bind/From/Default/New` 等缓存本体 API；根包运行时绑定原语统一放在 `binding.go`。
- 2026-05-13：缓存接口和 backend driver 拆到 `pkg/cache`，`core/cache` 不再初始化 Redis 基础设施。
- 2026-05-10：补齐 `core/cache` 模块方案，明确底层字符串接口、类型化 JSON helper、Remember、key 管理和 Redis/内存后端边界。

## Breaking Changes

- 不再提供 `core/cache/store` 子包。
- `cache.Init` 不再接收 `RedisConfig`，Redis 生命周期统一由 `core/redis` 管理。
