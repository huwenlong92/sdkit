# hostprobe 模块

## 作用

`core/hostprobe` 提供跨平台、轻量、只读的主机资源快照，用于后台运行看板和服务健康检查。

当前采集范围：

- 主机名、操作系统、架构、内核版本和运行时长；
- CPU 逻辑核心数和使用率；
- 内存总量、已用量、可用量和使用率；
- 调用方显式配置目录所在文件系统的容量与使用率；
- 全机累计网络收发量、包数量以及相邻有效采样间的收发速率。

## 边界

- 只采集主机资源，不维护业务依赖清单，不判断 BaiduPCS-Go、FFmpeg 等业务工具是否必需。
- 不落库、不启动常驻 goroutine、不主动上报远端监控平台。
- 不枚举所有挂载点，只探测调用方配置的目录，避免泄露无关主机布局。
- 单个指标采集失败时写入 `Snapshot.Issues`，其余指标继续返回；context 取消或超时会直接返回错误。
- `Probe` 内部串行采样并按 `CacheTTL` 复用快照，避免并发刷新页面时重复执行 CPU 采样。

## 对外 API

```go
type Probe struct { /* internal state */ }

func New(config Config) *Probe
func (p *Probe) Snapshot(ctx context.Context) (Snapshot, error)
```

网络实时速率需要两个有效样本。首次采样的 `Network.RateReady` 为 `false`；后续非缓存采样根据累计计数差值计算每秒速率。

## 更新记录

- 新增主机、CPU、内存、指定磁盘和网络快照。
- 新增短时缓存与相邻样本网络速率计算。
