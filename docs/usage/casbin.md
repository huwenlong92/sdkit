# Casbin 接口鉴权

基于 Casbin RBAC 的接口权限控制。

## 模型

`configs/rbac_model.conf`：

```
r = sub, obj, act            # 请求：角色, API路径, HTTP方法
p = sub, obj, act            # 策略：角色, 路径模式, 方法
e = some(where (p.eft == allow))
m = r.sub == p.sub && keyMatch2(r.obj, p.obj) && keyMatch2(r.act, p.act)
```

- `keyMatch2` 支持 `*` 通配符，如 `/users/*` 匹配 `/users/1`
- 超级管理员角色 `admin` 拥有 `*` `*` 通配策略，可访问所有接口

## 策略存储

策略保存在 `casbin_rule` 表（自动建表）：

| ptype | v0（角色code） | v1（路径） | v2（方法） |
|-------|---------------|-----------|-----------|
| p | admin | * | * |
| p | user | /admin/v1/users | GET |

## API 管理

**接口列表：**

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/admin/v1/apis` | 接口列表（分页） |
| GET | `/admin/v1/apis/:id` | 接口详情 |
| POST | `/admin/v1/apis` | 注册接口 |
| PUT | `/admin/v1/apis/:id` | 更新接口 |
| DELETE | `/admin/v1/apis/:id` | 删除接口 |

路由启动时自动扫描所有已注册路径并写入 `system_api` 表。

**角色授权：**

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/admin/v1/roles/:id/apis` | 获取角色的 API 权限 IDs |
| PUT | `/admin/v1/roles/:id/apis` | 更新角色的 API 权限 |

## Casbin 中间件

```go
import adminmiddleware "sdkitgo/app/admin/middleware"

// 在 authgin.Required(authenticator) 之后使用
authorized.Use(authgin.Required(authenticator))
authz := authorized.Group("")
authz.Use(adminmiddleware.Casbin())
```

中间件流程：

1. core/auth 写入 `auth_role_id`
2. 服务 middleware 查询自己的角色表，得到 `role.Code`
3. 服务 middleware 归一化路径，例如 admin 去掉 `/admin/v1`
4. `core/gin/casbin` middleware 调用 `core/casbin` manager 的 `Enforce(roleCode, path, method)`
5. 无权限返回 403

`core/casbin` 只提供通用 manager；Gin middleware 位于 `core/gin/casbin`。角色解析、路径前缀处理等服务相关逻辑放在 `app/admin/middleware/casbin.go` 或各服务自己的 middleware 中。

## 初始化

`sdkitgo serve` 和 `bootstrap.Init` 会在 `DB: true` 时自动注册 `core/casbin/facade` capability。默认配置：

```go
casbinfacade.Config{
    ModelPath:       "configs/rbac_model.conf",
    SuperRole:       "admin",
    AutoCreateTable: true,
}
```

facade 默认作为内部 capability 注册；只有需要对外展示 capability 时才显式使用 `WithExternal()`。

自定义 runtime app 中推荐使用 facade capability：

```go
import casbinfacade "github.com/huwenlong92/sdkit/core/casbin/facade"

app.RegisterCapabilities(
    casbinfacade.Use(
        casbinfacade.WithConfig(casbinfacade.Config{
            ModelPath:       "configs/rbac_model.conf",
            SuperRole:       "admin",
            AutoCreateTable: true,
        }),
    ),
)
```

需要等配置加载后再构造配置时，使用 `WithConfigLoader`：

```go
casbinfacade.Use(
    casbinfacade.WithConfigLoader(func(app *runtime.App) (casbinfacade.Config, error) {
        return casbinfacade.Config{
            ModelPath:       "configs/rbac_model.conf",
            SuperRole:       "admin",
            AutoCreateTable: true,
        }, nil
    }),
)
```

兼容入口仍保留，适合非 runtime 场景：

```go
casbin.InitContext(ctx, database.Default, casbin.Config{
    ModelPath:       "configs/rbac_model.conf",
    SuperRole:       "admin",
    AutoCreateTable: true,
})
```

## 自定义策略

admin 角色默认有 `*` 通配权限。其他角色的权限通过角色授权接口管理：

```json
PUT /admin/v1/roles/:id/apis
{"api_ids": [1, 2, 3]}
```

策略变更后自动重新加载。
