# Security 模块设计

`core/security` 是安全与风控基础能力中心。当前风控主线为 `core/security/risk`：core 只提供事件、规则、名单、频率、命中决策、Store、Counter、Logger 和 Gin 适配；业务表、SQL、后台配置和响应结构由应用层实现。

加解密、签名、密码哈希、随机 token、请求指纹等纯工具位于 `pkg/security`。

不负责登录态、JWT、Session、RBAC、菜单权限和普通访问日志；这些继续由 `core/auth`、`core/gin/session`、`core/casbin`、`core/accesslog` 等模块维护。

## 包结构

- `pkg/security/crypto`：AES-GCM、HMAC-SHA256、SHA256、随机字节和 OpenAPI 签名 payload。
- `pkg/security/password`：bcrypt 密码哈希与校验。
- `pkg/security/token`：随机 token、nonce、数字验证码。
- `pkg/security/fingerprint`：从 HTTP 请求提取 IP、UA、DeviceID，并生成请求指纹。
- `core/security/captcha`：验证码 Provider 接口、Manager、内存 Provider、base64 图片验证码 Provider、client token Provider、noop Provider。
- `core/security/captcha/store`：captcha challenge 短期 TTL 存储接口，提供 memory 和 Redis 实现。
- `core/security/risk`：可配置风控引擎、Store 接口、Counter、Logger 和决策模型。
- `core/security/risk/cache`：风控运行时配置的短 TTL Store 包装器，缓存场景、名单和频率规则查询。
- `core/gin/security/risk`：Gin 到 `risk.Event` 的适配、middleware 和统一拦截响应。
- `core/gin/security/middleware`：通用安全响应头。

## Redis 与 DB 分工

Redis 或 cache 用于短期状态、计数和配置短 TTL 缓存：

- `security:risk:freq:*`
- `security:risk:config:*`
- captcha challenge key 由应用侧按服务和场景定义

DB 用于长期事件日志、命中记录、场景配置、事件定义、名单规则和频率规则。表模型由应用层定义，core 不持有业务数据库结构。

## 核心接口

`risk.Engine` 面向后台可配置风控，核心只依赖应用实现的 `risk.Store`：

```go
type Store interface {
    LoadScene(ctx context.Context, service, scene string) (*Scene, error)
    MatchList(ctx context.Context, event Event, listType string) (*ListRule, error)
    ListFrequencyRules(ctx context.Context, event Event) ([]FrequencyRule, error)
    CountEvents(ctx context.Context, query EventCountQuery) (int64, error)
    SaveDecision(ctx context.Context, event Event, decision *Decision) error
}
```

`ListRule` 和 `FrequencyRule` 可以携带响应配置：`ResponseCode`、`ResponseMessage`、`ResponseAction`、`ResponsePayload`、`HTTPStatus`。这些字段不参与规则是否命中，只在命中后传递给 `HitDecision` 和最终 `Decision`，由 Gin 适配优先用于拦截响应。

多个规则同时命中时，Engine 按 `action` 严重程度选择主规则；`action` 相同取 `score` 更高的规则；再相同保留先命中规则。主规则的响应配置会写入最终 `Decision`。

`risk.Counter` 用于把频率统计从事件表 `COUNT` 下沉到短期计数能力：

```go
type Counter interface {
    Incr(ctx context.Context, key CounterKey) (int64, error)
}
```

`NewRedisCounter` 是生产推荐实现，内部使用 Redis Lua + ZSET 做滑动窗口计数。一次 `Incr` 会在同一个脚本中完成：

1. 清理窗口外的 ZSET 成员。
2. 写入当前事件。
3. 设置 ZSET 与序列 key 的 TTL。
4. 返回窗口内当前次数。

Redis key 使用 `security:risk:freq:` 前缀，业务维度会 hash 到 key 中，维度包括 service、scene、event、target_type、target_value、window_seconds。ZSET key 和序列 key 使用同一个 Redis Cluster hash tag，保证脚本在集群模式下仍访问同一个 slot。

`NewCacheCounter` 保留为测试或单进程场景的可选实现，语义是固定窗口；生产多实例频率规则不应使用内存 cache 计数。若 counter 不可用或返回错误，Engine 会回退到 Store 的 `CountEvents`，保持可用性。

运行时配置可通过 `risk/cache.New` 包一层短 TTL 缓存，缓存内容包括场景、黑白名单匹配结果和频率规则。该缓存只缓存 Store 查询结果，不引入业务配置结构。

事件日志和命中记录可通过 `risk.Logger` 异步写入。core 定义 `DecisionWriter`，应用层实现批量落库：

```go
type DecisionWriter interface {
    WriteDecisionBatch(ctx context.Context, records []DecisionRecord) error
}
```

Engine 在配置 `WithLogger(logger)` 后会优先把事件决策推入 logger 队列；推入成功后不等待 DB 写入，降低请求路径延迟。若没有 logger 或队列已满，Engine 会同步调用 Store 的 `SaveDecision`，保证高压时仍尽量保留审计日志。异步写入失败只能记录日志，不能影响已经返回的请求。

## Captcha

`captcha.Provider` 用于接入图片验证码、滑块验证码、点选验证码、Turnstile、reCAPTCHA 或业务自研 Provider。Provider 负责生成 challenge 并校验答案，Manager 按 `Kind` 路由到具体 Provider。

内置 Provider：

- `Base64Provider`：数字图片验证码。
- `SliderProvider`：后端生成缺口背景图和滑块图，校验偏移量、耗时、尝试次数和 TTL。
- `ClickProvider`：后端生成点选图和目标序列，校验点位顺序、容错、尝试次数和 TTL。
- `ClientSliderProvider` / `ClientClickProvider`：临时一次性客户端通过型 Provider，仅用于过渡。

`captcha/store.Store` 只抽象 challenge 需要的 `Set`、`Get`、`Delete`，测试默认使用 memory，生产可使用 `store.NewRedisStore(redisClient)`。

## 与其他模块关系

- `core/auth` 保持独立，security 只产出风险结果，不签发登录态或 JWT。
- `core/ratelimit` 保持独立；风控频率规则是安全决策，不替代通用限流模块。
- `core/accesslog` 继续记录普通访问日志；security 只处理安全事件和风控决策。
- `core/gin/security/risk` 只做 Gin 到 `risk.Event` 的适配、middleware 和统一拦截响应；不依赖应用层 response 包。

## 更新记录

- 2026-05-29：新增正式 `SliderProvider` 和 `ClickProvider`，支持后端生成图片、Redis/memory 短期状态、坐标强校验、TTL 和尝试次数限制。
- 2026-05-29：captcha challenge 存储收敛到 `core/security/captcha/store` 子包；base64 图片验证码通过 `captcha.NewBase64Store` 适配该存储，临时的 slider/click 一次性客户端校验由 `captcha.NewClientSliderProvider` 和 `captcha.NewClientClickProvider` 提供。
- 2026-05-29：删除旧 checker/manager、内置 checkers、audit、blacklist 和旧 Gin 风控/签名 middleware；可配置风控正式使用 `risk` 包名。
- 2026-05-29：`risk` 的名单规则和频率规则新增响应配置字段，支持规则级错误码、提示文案、前台动作、动作参数和 HTTP 状态；最终响应配置跟随主命中规则。
- 2026-05-29：配置查询缓存从 `risk` 根包移动到 `core/security/risk/cache`。
- 2026-05-28：`risk` 新增通用 Counter 和短 TTL 配置缓存，支持应用侧把频率统计切到 Redis 滑动窗口，并缓存场景、名单和频率规则配置。
- 2026-05-28：新增 `risk` 事件/规则/名单/频率评估引擎抽象和 Gin 适配，应用层通过 Store 实现持久化。
- 2026-05-21：纯安全工具下沉到 `pkg/security`；captcha Provider 改为生成 challenge + 校验答案的多类型接口，预留图片验证码和滑块验证码扩展。
