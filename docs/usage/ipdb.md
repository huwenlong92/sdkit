# IPDB 使用指南

`pkg/ipdb` 提供本地 IP 库文件查询能力，统一返回 `Record`。字段没有数据时保持空值，业务展示时直接跳过空字段。

## 下载 IP 库文件

`sdkit` 不内置 IP 库文件，业务需要自行下载并配置文件路径。

- ip2region：从 `https://github.com/lionsoul2014/ip2region` 获取 `ip2region.xdb`、`ip2region_v4.xdb` 或 `ip2region_v6.xdb`。
- MaxMind GeoLite2：从 `https://www.maxmind.com/en/geolite2/signup` 注册账号并下载 `GeoLite2-City.mmdb`、`GeoLite2-ASN.mmdb`。
- MaxMind GeoIP2 商业库：从 `https://www.maxmind.com/en/geoip2-databases` 获取对应 `.mmdb` 文件。

库文件建议放在业务自己的数据目录，例如 `/data/ipdb/`，不要提交到 SDK 仓库。不同数据源的授权、更新频率和字段完整度由业务侧负责。

## ip2region 查询

适合国内省市和运营商查询。

```go
locator, err := ipdb.New(ipdb.Config{
    Driver: ipdb.DriverIP2Region,
    Path:   "/data/ipdb/ip2region.xdb",
})
if err != nil {
    return err
}
defer locator.Close()

record, err := locator.Lookup(ctx, "114.114.114.114")
if err != nil {
    return err
}
```

`Mode` 可以控制加载策略：

```go
locator, err := ipdb.New(ipdb.Config{
    Driver: ipdb.DriverIP2Region,
    Path:   "/data/ipdb/ip2region.xdb",
    Mode:   ipdb.IP2RegionModeVectorIndex,
})
```

可选值：

- `ipdb.IP2RegionModeFile`：只读文件，内存占用低。
- `ipdb.IP2RegionModeVectorIndex`：预加载索引，默认值，查询 IO 更少。
- `ipdb.IP2RegionModeMemory`：加载完整 xdb 文件，查询更快，内存占用更高。

## MaxMind City 查询

适合全球国家、城市、经纬度查询。

```go
locator, err := ipdb.New(ipdb.Config{
    Driver:   ipdb.DriverMaxMindCity,
    Path:     "/data/ipdb/GeoLite2-City.mmdb",
    Language: "zh-CN",
})
```

## MaxMind ASN 查询

适合 ASN 和自治系统组织查询。

```go
locator, err := ipdb.New(ipdb.Config{
    Driver: ipdb.DriverMaxMindASN,
    Path:   "/data/ipdb/GeoLite2-ASN.mmdb",
})
```

## MaxMind City + ASN 合并查询

如果需要一次返回城市和 ASN 信息，使用 `DriverMaxMind` 并同时配置两个库文件。

```go
locator, err := ipdb.New(ipdb.Config{
    Driver:   ipdb.DriverMaxMind,
    CityPath: "/data/ipdb/GeoLite2-City.mmdb",
    ASNPath:  "/data/ipdb/GeoLite2-ASN.mmdb",
    Language: "zh-CN",
})
if err != nil {
    return err
}
defer locator.Close()

record, err := locator.Lookup(ctx, "8.8.8.8")
if err != nil {
    return err
}
```

## 展示字段

`Record` 使用 `json:",omitempty"`，没有的数据不会出现在 JSON 中。业务也可以用 `HasData` 判断是否查到了有效归属地或 ASN 信息。

```go
if record.HasData() {
    // 保存或展示 country/province/city/isp/asn 等非空字段
}
```

## 查询 Hook

如果需要在查询后记录日志、指标或投递事件，可以配置 `Hooks`。

```go
locator, err := ipdb.New(ipdb.Config{
    Driver: ipdb.DriverIP2Region,
    Path:   "/data/ipdb/ip2region.xdb",
    Hooks: []ipdb.Hook{
        ipdb.HookFunc(func(ctx context.Context, event ipdb.LookupEvent) {
            if event.Err != nil {
                return
            }
            // 记录日志、metrics，或把 event.Record 投递给业务自己的异步队列。
        }),
    },
})
```

Hook 是观察型扩展，不改变 `Lookup` 的返回结果。Hook 内部如果执行写库、发消息等操作，必须自行处理错误。强一致的业务写入建议在业务侧包装 `Locator`，不要依赖 Hook 隐式完成。

## 注意事项

- `Lookup` 会校验 IP 格式，非法 IP 返回 `ipdb.ErrInvalidIP`。
- `Lookup` 接收 `context.Context`，会在查询前后检查取消状态。
- 本包只负责 `IP -> Record`，不负责根据城市名反查 IP 段，也不负责自动下载或更新库文件。
- 后台按城市筛选业务数据时，应在业务入库时把 `country/province/city/isp/asn` 等字段冗余到业务表或日志表。
