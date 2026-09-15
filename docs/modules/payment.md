# Payment 模块

## 模块目标

`payment` 统一多支付平台、多支付产品形态下的支付生命周期。它不重写平台协议，而是在官方 SDK 或官方 HTTP API 外提供稳定的业务适配层。

核心目标：

- 统一支付、查询、关闭、退款、退款查询、回调模型
- 统一金额、币种、结算金额和汇率快照
- 统一前端动作，例如跳转、form、二维码、SDK 参数、client token
- 将微信、支付宝、Stripe、PayPal 等 SDK 类型限制在 `pkg/payment/*`
- 支持多商户、多 channel、运行期 reload 渠道映射
- 默认按请求创建 provider client，并在请求结束后 cleanup

模块不负责：

- 代替订单、库存、会员、财务、权益发放
- 存储支付单、退款单或回调幂等记录
- 管理数据库、Redis、配置中心
- 把所有平台私有字段塞进统一模型
- 主动引入非官方或过时 SDK

## 分层

```text
业务 handler
  -> core/payment
      统一模型 / Service / Registry / ChannelSelector / 状态和金额规则
    -> pkg/payment/{provider}
        provider adapter / client loader / action 校验 / capability
      -> pkg/payment/{provider}/{official-client}
          官方 SDK 或官方 HTTP API 封装
```

依赖方向：

```text
业务 -> core/payment -> pkg/payment/* -> 官方 SDK/API
```

`core/payment` 不依赖任何支付平台 SDK。

## Provider Build Tag

支付 provider 按 provider 维度编译。`core/payment` 只保留统一模型、registry、selector 和 facade，不 import 任何具体 provider 或官方 SDK。

| provider | package | build tag |
| --- | --- | --- |
| alipay | `pkg/payment/alipay`, `pkg/payment/alipay/openapi` | `sdkit_payment_alipay` |
| wechat | `pkg/payment/wechat`, `pkg/payment/wechat/apiv3` | `sdkit_payment_wechat` |
| stripe | `pkg/payment/stripe`, `pkg/payment/stripe/stripego` | `sdkit_payment_stripe` |
| paypal | `pkg/payment/paypal`, `pkg/payment/paypal/ordersapi` | `sdkit_payment_paypal` |
| airwallex | `pkg/payment/airwallex`, `pkg/payment/airwallex/httpapi` | `sdkit_payment_airwallex` |

`pkg/payment/aggregate`、`pkg/payment/channelrouter`、`pkg/payment/debuglog`、`pkg/payment/mock` 不绑定第三方 SDK，默认保留。

规则：

- 应用只 import 已启用 tag 对应的 provider。
- 构建时只打开当前二进制需要的 provider tag。
- 配置可以包含多个 channel，但二进制只能使用已编译的 provider。

## 包结构

```text
core/payment/
  types.go
  request.go
  event.go
  service.go
  default.go
  bind.go
  channel_selector.go
  registry.go
  pricing.go
  validation.go
  currency.go
  state.go
  errors.go
  facade/

pkg/payment/
  airwallex/
    httpapi/
  aggregate/
  alipay/
    openapi/
  channelrouter/
  debuglog/
  mock/
  paypal/
    ordersapi/
  stripe/
    stripego/
  wechat/
    apiv3/
```

## Provider 与 Channel

`Provider` 是服务商，`Channel` 是服务商下的产品形态。二者不能混成一个 `payment_type`。

当前 provider：

```go
ProviderWechat
ProviderAlipay
ProviderPayPal
ProviderStripe
ProviderAggregate
```

当前 channel：

```go
ChannelWechatApp
ChannelWechatMiniProgram
ChannelWechatH5
ChannelWechatNative
ChannelAlipayApp
ChannelAlipayWap
ChannelAlipayPage
ChannelPayPalOrder
ChannelStripeCheckout
ChannelStripeIntent
ChannelAggregateForm
```

聚合支付、学校、园区、政务、行业收费平台默认归入 `ProviderAggregate`。不要为每个学校或行业网关新增一级 provider。

## Channel 选择

业务调用推荐只传业务 channel key：

```go
payment.CreatePayment(ctx, payment.CreatePaymentRequest{
	MerchantKey: "school_a_wechat_mini",
	OutTradeNo:  "pay_1001",
	Pricing:     payment.CNY(19900),
})
```

`ChannelSelector` 将它解析成：

```go
payment.ChannelSelection{
	Provider:    payment.ProviderWechat,
	Channel:     payment.ChannelWechatMiniProgram,
	MerchantKey: "school_a_wechat",
}
```

`MerchantKey` 在请求进入 provider adapter 前会被改写为真实商户配置 key，响应里也会写回真实商户配置 key。

`StaticChannelSelector.Reload` 用于运行期重新加载完整映射。reload 原子替换：新配置校验失败时保留旧映射。

全局能力场景使用：

```go
payment.ReloadChannels(bindings)
```

## Client 生命周期

provider adapter 默认使用动态 client 模式。

规则：

- `Config.ClientMode == ""` 等同 `ClientModeDynamic`
- 动态模式必须提供 `ClientLoader`
- 每次请求调用 `ClientLoader.LoadPaymentClient(ctx, merchantKey)`
- loader 返回本次请求 client 和 cleanup
- adapter 调用完成后负责 cleanup
- loader 返回 error 或 nil client 时，也会 cleanup 已创建资源
- cleanup 错误不覆盖支付主流程结果
- 静态 client 必须显式设置 `ClientModeStatic`

动态模式的目的：

- 不常驻不用的商户 client
- 平台密钥、证书、appid 变更后，下一次调用即可读取新配置
- registry 只注册 provider 能力，不作为商户 client 池
- 业务不需要自己实现 `ProviderAdapter`

静态模式只用于单商户、固定配置或测试。

## Capability

动态模式无法通过一个固定 client 自动推断可选能力，因此要显式声明：

```go
wechat.Config{
	ClientLoader:         loader,
	SupportsQuery:       true,
	SupportsClose:       true,
	SupportsRefund:      true,
	SupportsQueryRefund: true,
	SupportsNotify:      true,
}
```

静态模式下仍可通过 client 是否实现接口自动推断：

```go
type QueryClient interface {
	QueryPayment(ctx context.Context, req payment.QueryPaymentRequest) (*payment.QueryPaymentResponse, error)
}
```

新增 adapter 时必须保证 `Capabilities()` 与实际行为一致。service 会基于 capability 做 channel、currency、action、query/refund 能力校验。

## 金额与币种

金额使用最小货币单位，例如分、欧分、便士。

默认策略：

- `PayAmount` 必填
- `PayAmount.Currency` 为空时默认 `CNY`
- `SettleCurrency` 为空时不计算结算金额，不要求汇率
- `OrderAmount` 为空时默认等于 `PayAmount`
- 支付币种等于结算币种时，`SettleAmount` 默认等于 `PayAmount`
- 支付币种不等于结算币种时，必须提供 `ExchangeRate`
- 汇率使用 decimal 字符串，禁止 float
- provider adapter 必须校验平台支持的币种

provider 不应在通用逻辑里写死 `CNY`，除非平台本身只支持该币种，并返回明确错误。

## Action 规则

创建支付返回 `PaymentAction`，前端只根据 action 类型执行下一步。

Action 约束：

- `none`：无需额外字段
- `redirect_url`：必须有 `URL`
- `html_form`：必须有 `HTML`，或 `URL + Fields`
- `qr_code`：必须有 `URL` 或 `Token`
- `sdk_params`：必须有 `Params`
- `client_token`：必须有 `Token`

Channel 到 action 的当前映射：

| Channel | Action |
| --- | --- |
| `wechat_app` | `sdk_params` |
| `wechat_mini_program` | `sdk_params` |
| `wechat_h5` | `redirect_url` |
| `wechat_native` | `qr_code` |
| `alipay_app` | `sdk_params` |
| `alipay_wap` | `html_form` 或 `redirect_url` |
| `alipay_page` | `html_form` 或 `redirect_url` |
| `stripe_checkout` | `redirect_url` |
| `stripe_payment_intent` | `client_token` |
| `paypal_order` | `redirect_url` |
| `aggregate_form` | `html_form` |

新增 channel 时必须先明确 action 类型，并补 contract test。

## 状态规则

支付状态：

```text
pending -> processing/requires_action/authorized/succeeded/failed/closed
succeeded -> refunding/partial_refunded/refunded
```

退款状态：

```text
pending -> processing -> succeeded/failed/closed
```

业务侧保存状态时应以状态机为准。查询或回调发现平台已支付时，不能因为本地过期时间已到就覆盖为失败。

## Debug

`pkg/payment/debuglog` 提供平台请求级 debug 事件；`pkg/payment/channelrouter` 提供路由级 debug 事件。

默认不输出完整 payload。只有显式设置 full payload 时，才记录请求和响应。

注意：

- full payload 可能包含 openid、邮箱、client secret、approval URL
- 不要输出平台密钥、私钥、证书
- debug logger 不绑定 zap/logrus 等具体日志库

## 新增 Provider 规则

新增 provider 前先确认：

- 是否有官方 Go SDK
- 没有官方 SDK 时，是否有官方 HTTP API
- 不引入非官方、过时、长期无人维护的 SDK
- provider 是否真的是一级服务商，而不是聚合网关内的一个商户

新增 provider 包结构：

```text
pkg/payment/{provider}/
  types.go      // Client 接口、Config、ClientLoader、ClientMode
  adapter.go    // ProviderAdapter 实现
  capabilities.go 或 adapter.go 内部方法

pkg/payment/{provider}/{official-api}/
  client.go     // 官方 SDK/API 封装
  config.go
  operations.go
```

必须满足：

- adapter 实现 `payment.ProviderAdapter`
- `Name()` 返回稳定 provider 常量
- `Capabilities()` 返回真实支持能力
- 所有方法透传 `context.Context`
- 所有错误返回 error，禁止 panic
- 动态模式默认启用，静态模式必须显式开关
- 每次动态创建 client 后必须 cleanup
- 创建支付必须校验 channel/action 匹配
- 查询、退款、回调等可选能力不支持时返回 `ErrUnsupportedCapability`
- 响应必须补齐 provider、channel、merchant_key、支付引用、状态、金额快照
- 平台原始响应可放 `Raw`，不要覆盖统一字段语义

测试要求：

- contract test 覆盖 channel/action
- capability test 覆盖 query/refund/notify 能力
- dynamic loader test 覆盖每次调用创建 client 和 cleanup
- error test 覆盖 loader error、nil client、unsupported channel/action
- sandbox test 默认 skip，通过环境变量开启

文档要求：

- 更新 `docs/usage/payment.md` 的 provider 参数、初始化、调用示例
- 更新 `docs/modules/payment.md` 的 provider/channel/action 规则

## 新增 Channel 规则

新增 channel 时必须确认：

- 所属 provider
- 前端 action 类型
- 必传请求参数
- 支持币种
- 是否支持过期时间
- 是否支持关闭、查询、退款、退款查询、回调
- 回调如何定位 `merchant_key`
- 平台交易号、退款号字段如何映射到统一响应

新增 channel 后需要：

- 在 `core/payment/types.go` 增加常量
- 在 provider adapter 校验 channel/action
- 在 provider client 实现请求构造和响应映射
- 补 contract test
- 更新使用文档

## 配置边界

`core/payment/facade.Config` 只保存 channel 映射：

```go
type Config struct {
	Channels []payment.ChannelBinding
}
```

平台配置不进入 core facade：

- 微信 appid/mchid/证书/APIv3Key
- 支付宝 appid/私钥/公钥/网关
- Stripe secret key
- PayPal client id/secret

这些配置由业务的 `ClientLoader` 按真实 `merchant_key` 加载。

Payment facade 不读取 `core/config.V`，也不假设业务项目的 `payment` 配置结构。启动层必须通过 `WithConfig` / `WithConfigLoader` 显式传入 channel 映射；如果配置来自数据库、远端配置或项目 YAML，由业务 adapter 负责读取并映射到 `paymentfacade.Config`。

## 回调约束

回调验签必须使用正确商户配置。常见做法：

- 回调 URL 中带 `merchant_key`
- query 中带 `merchant_key`
- 根据平台回调体里的 appid/mchid 反查 merchant_key

adapter 不能使用随机默认商户验签。无法定位商户时应返回明确错误。

## Braintree

Braintree 当前没有官方 Go server SDK。本模块不引入非官方 `braintree-go`。后续如需接入，优先使用 PayPal/Braintree 官方 HTTP API，并按上述 provider 规则实现。

## 更新记录

- 2026-05-28：Payment facade 移除 `core/config.V` 隐式配置读取；channel 映射必须由业务侧显式注入。
- 新增统一 `payment.CreatePayment` 等全局入口。
- 新增 `ChannelSelector` 和 `payment.ReloadChannels`。
- 新增微信、支付宝、Stripe、PayPal adapter。
- 默认改为动态 client 模式，静态 client 需要显式 `ClientModeStatic`。


## Airwallex 普通支付适配器（2026-09-11）

2026-09-15：HTTP client 增加显式 AuthenticationMode。空值保持账号头认证；default 模式支持单账号 Scoped Key 默认认证，仍要求 AccountID 并保留 webhook 跨账号拒绝。模式仅影响认证请求头，不修改 token 缓存、操作重试或支付事实映射。

- 只增加 `ProviderAirwallex` / `ChannelAirwallexHPP` 常量，不扩充既有 ProviderAdapter 或 facade.Config；无 tag 时具体实现不参与编译。
- adapter 沿用动态 ClientLoader / 显式静态 Client 模式，普通支付客户端实现全套六个方法；capability 与方法对应。回调账户选择使用服务器配置 NotifyMerchantKey，不接受请求自报商户 key。
- httpapi 复用 pkg/request，强制显式环境及账户、官方固定域名、禁止重定向、30 秒默认 HTTP 超时、1 MiB 响应上限。每实例维护 token 和到期时间；并发刷新合并，等待者可取消；不做全局跨账户 token 缓存。
- v4 UUID 幂等键由业务在操作前持久化；不自动重新生成，不自动重试 POST。401 失效缓存，错误不输出凭据或上游原始 body。
- 金额使用整数与精确有理数转换，拒绝多余精度、负值和 int64 溢出。查询的付款金额不伪装成真实结算金额；退款先核对原支付币种。
- HPP 返回 SDKParams（需官方 Airwallex.js）；创建接口本身不等于付款成功。REQUIRES_CAPTURE 映射 authorized，不等于 captured/succeeded。
- 验签先于 JSON 解析；使用配置中的签名密钥、原始毫秒时间戳和原始 body，验证账户和事件/状态一致性。未知事件/状态不升级为成功；原始 payload 不回显。
- 支付/退款事件解析是无状态的；可靠入库、去重、乱序事件、订单关联、费率、分账、结算和银行到账事实由消费方拥有。
- 本轮不提供开户、渠道分账、出款、独立 capture 或指定支付过期时间。官方当前未提供 Go 服务端 SDK，HTTP 层为官方 REST 协议封装。

调用方式、配置字段和回调限制见 [Payment 使用 / Airwallex 普通支付](../usage/payment.md#airwallex-普通支付2026-09-11)。


## 2026-09-11：支付主流程与可选能力

支付金额/币种/状态是主流程。支付币种为空默认 CNY；未指定结算币种时不隐式换汇。显式设置结算币种及 ExchangeRate 的计算能力继续保留。ProviderSettlement 是下单、查询及回调共有的可选渠道事实，和调用方计算的 Pricing 分开；缺失不等于零，不要求补齐，也不额外请求结算报告。

CreatePayout/QueryPayout 根据可选 PayoutProvider 接口分派，普通支付 adapter 不实现时返回 ErrUnsupportedCapability。打款请求使用收款人 ID、最小单位金额、币种、持久化 RequestID；业务方负责授权及幂等存储。Airwallex 使用 Transfers 协议，SENT 仅表示已发出，不能当作收款成功。

同一种 provider 可有多个 merchant_key；一个 adapter 通过 ClientLoader 选择商户实例。ChannelBinding.disabled_for_new 阻止新下单/打款，查询、退款和回调继续使用原绑定。NotifyRequest.MerchantKey 由服务端回调路由选择，adapter 据此加载验签配置；签名校验后还须匹配原支付商户。不得因超时自动换商户重试。

Airwallex 目前官方服务端 SDK 仅列出 Node.js Beta，Go 协议实现复用 sdkit/pkg/request。参考：https://www.airwallex.com/docs/developer-tools/sdks/server-side-sdks-%28beta%29 。普通 PaymentIntent 文档未提供逐笔完整结算金额/汇率；它们通过单独的 Settlement Records API 提供。本轮不主动查询该 API，不伪造或从支付金额推导 ProviderSettlement。

运行时可调用 payment.RegisterProvider(adapter) 注册新类型，payment.ReloadChannels(bindings) 原子替换商户绑定；重复注册同名类型返回 ErrAdapterAlreadyExists。动态商户凭据通过 adapter 的 ClientLoader 提供，core 不持有业务配置和密钥。请求显式指定 Provider/Channel/MerchantKey；回调也支持服务端指定 MerchantKey。Airwallex 打款使用 ChannelAirwallexTransfer，同币种出款；接收人管理及自动换汇不在此操作范围。PAID 是渠道处理成功状态，之后仍可能失败，业务方不得将其视为永不可变的状态。

Transfers HTTP 请求固定使用 x-api-version 2024-09-27，与 transfer_amount/transfer_currency 和 Transfers 资源协议一致；普通支付请求维持原版本行为。


### 下单地址参数（2026-09-11）

`CreatePaymentRequest.ReturnURL` 和 `NotifyURL` 是单笔支付参数。支持的 adapter 使用请求值，未传时使用该商户可选默认值；同一次渠道幂等重试应保留首次参数。ReturnURL 只决定客户端返回页面，不是付款成功凭据。

Airwallex 支持请求级 ReturnURL。它的 PaymentIntent API 不接受逐笔 notify_url，通知地址通过 Webhook 订阅预先注册。`httpapi.Config.NotifyURL` 记录已在 Airwallex 注册的地址，不会创建或修改订阅；请求 NotifyURL 为空或等于该地址时正常下单，其他值在网络请求前返回 ErrUnsupportedCapability。微信/支付宝沿用已有请求 NotifyURL 优先于 client 默认值的行为。

参考：[Airwallex Webhook 订阅](https://www.airwallex.com/docs/developer-tools/webhooks/webhooks-overview)、[PaymentIntent API](https://www.airwallex.com/docs/api/payments/payment_intents/create)。


### Provider expiry (2026-09-11)

`CreatePaymentRequest.ExpireAt` requests a provider expiry only when `SupportsExpireAt` is supported. Create/query responses and `PaymentEvent` optionally return the provider-confirmed `ExpireAt`; nil means unknown. `PaymentExpired` / `EventPaymentExpired` represent provider-confirmed expiration. Business deadlines stay with the caller and are not proof that a provider cannot collect. A verified late success may supersede expiration; stale intermediate results cannot reopen an expired payment. `CreatePaymentResponse.PaidAt` optionally carries the provider payment timestamp.

Airwallex token 到期时间兼容 RFC3339 与 ISO-8601 无冒号时区偏移（如 `+0000`），保留严格到期校验和提前刷新。


业务方每次明确重新支付应创建新的支付单号和 request_id。相同 HTTP 命令重放保持原 request_id；已知 ProviderTradeID 时可读取既有渠道单返回付款动作。Airwallex 不再在 duplicate_request 后自动检索或重建支付单；渠道错误原样交由调用方处理。
