# payment Airwallex 普通支付适配器

状态：公共 adapter 与离线验证已完成；真实渠道和消费方业务接入待后续。

日期：2026-09-11

## 范围与决定

用户确认先完成 `sdkit/core/payment` → `pkg/payment/airwallex` 的公共能力。普通支付、查询、关闭、退款、退款查询与验签通知沿用 ProviderAdapter；不加入 Connected Accounts、Hosted Flow、Funds Split、Transfers 或消费方业务状态机。

GitNexus 对 ProviderAdapter 的 upstream 检查检出 34 个直接依赖文件，风险 CRITICAL；本轮不修改其方法或现有 provider，仅新增 provider/channel 常量文件和独立实现。项目内其他邮件、短信等未提交工作保留。

## 交付

- `core/payment/airwallex.go`：ProviderAirwallex、ChannelAirwallexHPP。
- `pkg/payment/airwallex`：动态/静态客户端 adapter，受信任的回调商户 key。
- `pkg/payment/airwallex/httpapi`：官方 API、token、精确金额、统一状态、SDKParams、验签。
- `tests/pkg/payment/airwallex`：注入 HTTP transport 的离线 contract 与 core facade 调用测试，不使用真实凭据或公网。
- `docs/usage/payment.md`、`docs/modules/payment.md`：配置、使用、边界和限制。

## 验收记录

已在隔离源码副本通过：

- `go test -race -count=1 -tags sdkit_payment_airwallex ./tests/pkg/payment/airwallex/... ./tests/core/payment/...`
- 同时启用 airwallex/alipay/wechat/stripe/paypal 五种 tag 的全部 payment 测试，跳过 `^TestSandbox` 真实账户用例，14 个测试包通过。
- 无 provider tag 的 payment 编译与 core/mock/aggregate/channelrouter 回归。
- `go vet -tags sdkit_payment_airwallex ./pkg/payment/airwallex/... ./tests/pkg/payment/airwallex/...`

离线测试覆盖统一 facade 完整调用链、动态 loader/cleanup、配置与路由验证、准确 JSON 金额、0/2/3 位货币精度、int64 边界、并发认证/到期刷新/账户隔离、401 失效且不自动重放、相同 payload 幂等重试、禁止重定向、回调原文验签/时效/账户/事件状态匹配、未知与重复事件和响应错误脱敏。

本轮不连接 Sandbox 或生产、不创建真实支付、不修改消费方数据库、运行配置或部署。真实浏览器收银台、商户权限、Webhook API 版本与真实退款到账仍须在环境资料确认后联调。
