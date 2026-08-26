# hostprobe 使用

```go
probe := hostprobe.New(hostprobe.Config{
	CacheTTL:    2 * time.Second,
	CPUInterval: 200 * time.Millisecond,
	Disks: []hostprobe.DiskSpec{
		{Name: "工作目录", Path: "."},
		{Name: "搬运暂存区", Path: "/var/lib/sdingest/stage"},
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
- 页面应展示 `Issues`，不要因为单个磁盘目录暂时不可用而隐藏全部主机指标。
