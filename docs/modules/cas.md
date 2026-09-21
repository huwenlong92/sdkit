# CAS 模块

位置：`core/cas`。按 Utes 本次明确要求归入 core，只有通用协议能力；不吸收应用模型、CAS 与学工号映射、账号创建或授权规则。

## API

| API | 职责 |
| --- | --- |
| `Config.Normalized/Validate` | 默认值、协议版本、URL 配置校验 |
| `NewClient/NewClientWithHTTPClient` | 构造独立客户端，无全局实例 |
| `ServiceURL/LoginURL` | 回调 service 与登录跳转地址 |
| `ValidateTicket` | CAS v2/v3 XML 校验与通用属性解析 |
| `LogoutURL` | 生成服务器退出地址，不销毁应用 Session |
| `SafeRedirect` | 检查站内回跳路径，包括编码后的双斜线、反斜线与控制字符 |
| `Principal.Metadata` | 属性副本；不隐式写库或日志 |

## ucenter 审计（2026-09-05）

参考仓库 `ucenter/pkg/umsdk`，仅按协议提炼，不复制其硬编码配置、密钥或学校业务分类。

| 实现 | 观察到的能力 | 处理 |
| --- | --- | --- |
| cqu | `/p3/serviceValidate`、登录/退出、uid/cn | 归纳为 CAS v3 与可配置主体属性 |
| nuaa / shzu | `/authserver/serviceValidate`、登录/退出 | 用 ServerURL 路径前缀与 CAS v2 配置表达；不复制各校分类解析 |
| shu / sjtu | OAuth / OAuth2 授权与令牌交换 | 不是 CAS ticket 协议，不并入本模块 |
| udata | `/authorize/info` 自定义 JSON 接口 | 不伪装为标准 CAS，不并入本模块 |
| ecupl | 文件只有 package 声明 | 无可复用实现，华政仍需真实联调 |

旧实现存在响应日志输出、context 未接到 HTTP 请求，以及 shzu 的 Logout 返回 login 地址等问题；新模块不沿用这些行为。

## 安全与未支持范围

- 默认使用协议 `<user>`；只有显式配置才以其他属性为主体。校验失败不返回原始服务端失败文本。
- 要求 CAS serviceResponse 根元素和命名空间，拒绝同时出现成功与失败；拒绝空主体、超大响应及非 2xx。
- 不支持 proxy ticket、PGT、CAS REST 密码交换、SAML、OAuth/OIDC 或单点退出回调（SLO）。生成 logout 地址不等于 SLO。
- 不实现浏览器 state 管理、Cookie 策略、应用 Session 或权限复核，这些需要消费方负责。

## 本次验证

根 `tests/core/cas` 覆盖 v2/v3 路径、service 一致性、主体字段选择、重复属性、副本隔离、失败/错误 XML、响应体上限、3xx 拒绝、超时、context 取消、URL 配置和回跳检查。

使用说明见 [CAS 使用](../usage/cas.md)。
