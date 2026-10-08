# sysprobe 使用

## 显式构造

```go
import (
    "context"
    "time"

    "github.com/huwenlong92/sdkit/core/sysprobe"
)

probe := sysprobe.New(sysprobe.Config{
    Host: sysprobe.HostConfig{
        CacheTTL: 2 * time.Second,
        CPUInterval: 200 * time.Millisecond,
        Disks: []sysprobe.DiskSpec{{Name: "workdir", Path: "/var/lib/example/work"}},
        Network: sysprobe.NetworkSpec{ProcRoot: "/host/proc", Interfaces: []string{"eth0"}},
    },
    NetworkCapacity: sysprobe.NetworkCapacityConfig{SendLimitMbps: 10},
    DependencyTTL: 30 * time.Second,
    CheckTimeout: 2 * time.Second,
    Dependencies: []sysprobe.DependencySpec{{
        Key: "example-tool", Name: "Example Tool", BinaryPath: "/opt/example/bin/tool",
        Args: []string{"--version"}, MinVersion: "1.2.0", MaxVersion: "1.2.9", Required: true,
    }},
})
defer probe.Close()

snapshot, err := probe.Snapshot(context.Background())
if err != nil {
    return err
}
```

需要只采集主机指标时使用 `sysprobe.NewHost(sysprobe.HostConfig{...})`，返回 `HostSnapshot`。相同探针实例应在服务生命周期内复用，以便命中缓存并计算网络速率。

## runtime capability

```go
import "github.com/huwenlong92/sdkit/core/sysprobe/facade"

capability := facade.Use(
    facade.WithName(ctx.LocalName(facade.Name)),
    facade.WithConfigLoader(func(app *runtime.App) (facade.Config, error) {
        return loadProbeConfig(ctx.ConfigFile)
    }),
)
```

服务 provider 显式注册 capability；业务通过 `facade.FromServiceContext(serviceContext)` 或 `facade.From(app, capability.Name())` 获取实例。需要进程默认入口时显式加入 `facade.WithDefault()`，再通过 `facade.FromDefault()` 获取。

配置完全采用通用结构时，可以直接用 `facade.UseConfigFile(configPath)`。它要求入口配置存在顶层 `sysprobe`，按 `mapstructure` 标签加载：

```yaml
sysprobe:
  host:
    cache_ttl: 2s
    cpu_interval: 200ms
    disks:
      - name: workdir
        path: /var/lib/example/work
    network:
      proc_root: /host/proc
      interfaces: [eth0]
  network_capacity:
    receive_limit_mbps: 100
    send_limit_mbps: 10
  dependency_ttl: 30s
  check_timeout: 2s
  dependencies:
    - key: example-tool
      name: Example Tool
      binary_path: /opt/example/bin/tool
      args: [--version]
      min_version: "1.2.0"
      max_version: "1.2.9"
      required: true
```

项目已经维护独立的工具配置时，通过 `WithConfigLoader` 转成 `facade.Config`，不重复维护工具路径和版本。

## 环境变量和版本

工具检查仅执行服务端配置的命令。`Env` 传入工具需要的环境变量；`PrepareDir` 可选，用于提前创建工具检查所需的隔离目录。core 不推断工具供应商、配置目录变量或命令参数。

版本检查提取三段数值版本，支持 `v1.2.3`、`n1.2.3-build` 等前缀。最小和最大版本边界均包含边界值；这不是完整的预发布版本排序。无法识别三段数值版本时返回 `version_unknown`，即使没有配置版本边界也不会标为兼容。

`Dependencies` 返回 `Code`、安装和兼容状态、版本、路径及检查消息。需要中文提示或产品帮助主题时，由调用方按状态码和工具 Key 映射。

## 容器与多节点

容器展示宿主机出口流量时，可把宿主机 `/proc/1/net` 只读挂载到 `/host/proc/net`，并配置 `host.network.proc_root: /host/proc`。接口应填写实际出口网卡，避免聚合 Docker bridge、veth 和 loopback 流量。

core 只探测当前节点。要查看独立 Worker 主机的负载，需要 Worker 本机采样、节点标识和定期上报，再由管理入口汇总并判断数据是否过期。
