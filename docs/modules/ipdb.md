# IPDB 工具设计

`pkg/ipdb` 提供本地 IP 库文件查询能力，用统一接口屏蔽 ip2region xdb 和 MaxMind mmdb 的差异。

## 包结构

```txt
pkg/ipdb/
  composite.go
  errors.go
  hook.go
  ip2region.go
  ipdb.go
  maxmind.go
  record.go
```

测试位于：

```txt
tests/pkg/ipdb/
```

## 对外 API

核心接口：

```go
type Locator interface {
    Lookup(ctx context.Context, ip string) (*Record, error)
    Close() error
}
```

创建入口：

```go
locator, err := ipdb.New(ipdb.Config{
    Driver: ipdb.DriverIP2Region,
    Path:   "/data/ipdb/ip2region.xdb",
})
```

支持的 driver：

- `ipdb.DriverIP2Region`：读取 ip2region xdb 文件。
- `ipdb.DriverMaxMindCity`：读取 MaxMind City mmdb 文件。
- `ipdb.DriverMaxMindASN`：读取 MaxMind ASN mmdb 文件。
- `ipdb.DriverMaxMind`：合并 MaxMind City 与 ASN 查询结果。

## Hook

`Config.Hooks` 支持查询后观察：

```go
type Hook interface {
    AfterLookup(ctx context.Context, event LookupEvent)
}
```

Hook 会收到原始 IP、查询结果和错误，适合日志、metrics、审计事件、非关键异步投递。Hook 不返回错误，不改变 `Lookup` 结果；Hook 内执行的外部副作用必须自行处理错误。必须影响业务结果的逻辑应由业务包装 `Locator` 实现。

## Record 统一规则

`Record` 统一国内外字段：

- 通用：`IP`、`Version`、`Source`
- 地理：`Continent`、`Country`、`Region`、`Province`、`City`、`District`
- 编码：`ContinentCode`、`CountryCode`、`RegionCode`
- 网络：`ISP`、`ASN`、`ASNumber`、`ASOrganization`、`Network`
- 位置：`Latitude`、`Longitude`、`PostalCode`、`TimeZone`

不同数据源字段不一致时，缺失字段保持零值，并通过 JSON `omitempty` 隐藏。

ip2region 通常提供 `Country/Province/City/ISP`，不提供经纬度和 ASN。MaxMind City 通常提供国家、城市、经纬度和网络段，不提供运营商和 ASN。MaxMind ASN 提供 ASN 和自治系统组织。

## 并发与生命周期

- ip2region Go binding 标注 Searcher 非线程安全，`pkg/ipdb` 在 provider 内加锁保护查询和关闭。
- MaxMind reader 支持并发读取，`pkg/ipdb` 只用读写锁保护关闭状态。
- `Close` 可重复调用。
- `Lookup` 不启动 goroutine。
- `Lookup` 在查询前后检查 `context.Context`，底层库不支持中断正在进行的本地文件读取。

## 边界

`pkg/ipdb` 负责：

- 按路径加载本地 IP 库文件
- 校验 IP 格式
- 查询 IP 归属地、ASN 等信息
- 合并 MaxMind City 与 ASN 结果
- 提供查询后观察 Hook
- 返回统一 `Record`

`pkg/ipdb` 不负责：

- 自动下载 IP 库文件
- 自动更新 IP 库文件
- 根据城市名反查 IP 段
- 写入业务表或日志表
- 接入 bootstrap、全局默认实例或配置中心

如果后续需要框架级统一初始化，可在 `core/ipdb` 中包装 `pkg/ipdb`，提供 `Init`、`Default`、`Close` 和配置映射。

## IP 库文件来源

- ip2region：`https://github.com/lionsoul2014/ip2region`
- MaxMind GeoLite2：`https://www.maxmind.com/en/geolite2/signup`
- MaxMind GeoIP2：`https://www.maxmind.com/en/geoip2-databases`

库文件授权和更新策略由业务侧负责，SDK 不内置任何数据文件。

## 更新记录

- 2026-05-25：新增 `pkg/ipdb`，支持 ip2region、MaxMind City、MaxMind ASN 和 City+ASN 合并查询。
