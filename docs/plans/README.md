# sdkit 实施计划索引

## 当前计划

| 计划 | 状态 | 范围 |
| --- | --- | --- |
| [payment Airwallex 普通支付适配器](./2026-09-11-payment-Airwallex普通支付适配器.md) | 公共能力已完成 | 既有 core/payment 契约、Airwallex adapter、官方 HTTP API 和离线验证完成；消费方页面/数据库与真实联调另行处理。 |
| [payment Airwallex 支付与分账旧方案](./2026-09-02-payment-Airwallex支付与分账能力实施计划.md) | 历史/已替代 | 不执行旧 Connected Accounts、Hosted Flow、Funds Split 设计；由普通支付方案替代。 |
| [SDIngest 传输复用与 RemoteFS 边界优化实施计划](./2026-08-25-sdingest%E4%BC%A0%E8%BE%93%E5%A4%8D%E7%94%A8%E4%B8%8Eremotefs%E8%BE%B9%E7%95%8C%E4%BC%98%E5%8C%96%E5%AE%9E%E6%96%BD%E8%AE%A1%E5%88%92.md) | 待执行 | `pkg/sdingest` 复用 `pkg/request`，修正 RemoteFS 下载、Session、安全与公共契约，并协调迁移 SDIngest。 |
| [RemoteFS 与媒体探测基础能力实施计划](./2026-08-22-remotefs%E4%B8%8E%E5%AA%92%E4%BD%93%E6%8E%A2%E6%B5%8B%E5%9F%BA%E7%A1%80%E8%83%BD%E5%8A%9B%E5%AE%9E%E6%96%BD%E8%AE%A1%E5%88%92.md) | 已完成 | `remotefs` 通用契约、local/baidupan driver、多账号会话隔离、media/ffprobe capability 的首版实现。 |

## 维护约定

- 计划只记录 sdkit 可复用基础能力，不写消费方数据库、任务状态机和页面需求。
- 模块实现完成后，把稳定的设计和用法分别沉淀到 `docs/modules/` 与 `docs/usage/`；计划保留实施和验收记录。
- 计划状态变化、新增、删除或重命名时同步更新本索引。
