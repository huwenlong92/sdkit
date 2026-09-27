# hostprobe 使用

```go
probe := hostprobe.New(hostprobe.Config{
	CacheTTL:    2 * time.Second,
	CPUInterval: 200 * time.Millisecond,
	Disks: []hostprobe.DiskSpec{
		{Name: "工作目录", Path: "."},
		{Name: "搬运暂存区", Path: "/var/lib/sdingest/stage"},
	},
	Network: hostprobe.NetworkSpec{
		ProcRoot:   "/host/proc",
		Interfaces: []string{"eth0"},
	},
})

snapshot, err := probe.Snapshot(ctx)
if err != nil {
	return err
}
```

## 配置建议

- 后台轮询周期建议为 5～10 秒，`CacheTTL` 建议为 2～5 秒。
- `CPUInterval` 建议保持在 100～300 毫秒；间隔越长，单次请求等待越久。
- 磁盘路径必须由服务端配置，不能直接接受外部请求参数。
- 相同 `Probe` 实例应在服务生命周期内复用，网络速率才能使用稳定的相邻样本计算。
- 容器读取宿主机出口流量时，可把宿主机 `/proc/1/net` 只读挂载到 `/host/proc/net`，并把 `Network.ProcRoot` 配置为 `/host/proc`；直接挂载 `/proc` 会让 `/proc/net` 继续跟随容器自身的 network namespace。
- `Network.Interfaces` 应显式填写公网出口网卡，例如 `eth0`，不要把 Docker bridge、veth 和 loopback 一并聚合。
- 页面应展示 `Issues`，不要因为单个磁盘目录暂时不可用而隐藏全部主机指标。
