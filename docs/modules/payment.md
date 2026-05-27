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
- `SettleCurrency` 为空时默认 `CNY`
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

## 回调约束

回调验签必须使用正确商户配置。常见做法：

- 回调 URL 中带 `merchant_key`
- query 中带 `merchant_key`
- 根据平台回调体里的 appid/mchid 反查 merchant_key

adapter 不能使用随机默认商户验签。无法定位商户时应返回明确错误。

## Braintree

Braintree 当前没有官方 Go server SDK。本模块不引入非官方 `braintree-go`。后续如需接入，优先使用 PayPal/Braintree 官方 HTTP API，并按上述 provider 规则实现。

## 更新记录

- 新增统一 `payment.CreatePayment` 等全局入口。
- 新增 `ChannelSelector` 和 `payment.ReloadChannels`。
- 新增微信、支付宝、Stripe、PayPal adapter。
- 默认改为动态 client 模式，静态 client 需要显式 `ClientModeStatic`。
