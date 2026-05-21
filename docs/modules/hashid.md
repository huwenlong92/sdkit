# HashID 工具设计

`pkg/hashid` 提供基于 Hashids 的公开 ID 编码工具，用于把数据库自增 ID 转成不连续的短字符串。

HashID 只用于 ID 混淆，不提供加密、鉴权或防越权能力。接口鉴权仍必须依赖 auth、RBAC 或业务权限校验。

## 包结构

```txt
pkg/hashid/
  alphabet.go
  errors.go
  hashid.go
```

## 边界

`pkg/hashid` 负责：

- 按配置创建 Encoder
- 编码和解码多个整数
- 编码和解码带类型标记的 ID
- 提供大小写字母表常量

`pkg/hashid` 不负责：

- 定义业务 ID 类型，例如 UserID、OrderID
- 读取配置文件
- 做权限判断
- 隐藏敏感数据

业务 ID 类型常量应放在使用方自己的 domain、internal 或 config 包内。

## 更新记录

- 2026-05-21：新增 `pkg/hashid`，从业务包能力沉淀为配置化公共工具。
