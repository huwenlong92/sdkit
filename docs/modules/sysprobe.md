# sysprobe 模块

## 职责

`core/sysprobe` 提供主机资源与可执行工具依赖的统一快照，`core/sysprobe/facade` 提供 runtime capability、容器绑定和实例生命周期。调用方可以独立构造探针，也可以通过 runtime 注册。

主机采样包括主机信息、CPU、内存、指定目录所在磁盘，以及网卡累计流量和相邻采样间速率。依赖检查包括可执行文件定位、版本命令、版本范围、检查超时、缓存和必需工具的就绪判断。

## 配置和快照

- `HostConfig`：主机采样缓存、CPU 采样间隔、磁盘目录、网络 proc 根路径和接口白名单。
- `Config`：组合 `Host`、`NetworkCapacity`、依赖缓存 TTL、检查超时和 `Dependencies`。
- `DependencySpec`：工具标识、路径、参数、调用方提供的环境变量、可选探针目录、版本范围和是否必需。
- `HostSnapshot`：主机、CPU、内存、磁盘、网络和采集 Issues。
- `Snapshot`：组合主机快照、网络容量、依赖状态、检查时间和 `Ready`。
- `DependencyStatus.Code`：稳定状态码，调用方可据此生成本地化提示。

## 对外 API

```go
func New(config Config, options ...Option) *Service
func (s *Service) Snapshot(ctx context.Context) (Snapshot, error)
func (s *Service) Close() error
func WithRunner(runner Runner) Option
func WithHostSampler(host HostSampler) Option

func NewHost(config HostConfig) *HostProbe
func (p *HostProbe) Snapshot(ctx context.Context) (HostSnapshot, error)
```

facade 提供 `Use`、`UseConfigFile`、`WithName`、`WithConfig`、`WithConfigLoader`、`WithDefault`、`From`、`FromDefault` 和 `FromServiceContext`。能力名为 `sysprobe`，Group 为 `system`，Scope 为 `service-local`。

## 缓存与故障边界

- 主机默认缓存 2 秒，CPU 默认采样 200 毫秒；CPU 采样间隔最多 1 秒，磁盘和接口各最多 16 项。
- 工具检查默认缓存 30 秒，单个版本命令默认超时 2 秒，配置超时超过 10 秒时回到默认值；同时最多执行 8 个检查。
- 必需工具缺失或版本不兼容时 `Ready=false`；可选工具失败仍保留其状态，不阻塞依赖就绪。
- 主机部分采集失败写入 `HostSnapshot.Issues`；主机采集器 panic 降级为 issue。主机采集器返回 error、context 取消或超时则返回错误。
- 父 context 取消的工具检查不写入缓存。依赖状态和配置切片均做隔离，调用方修改返回值不影响缓存。
- 只启动版本命令，不启动常驻采样 goroutine。等待依赖缓存刷新时响应 context 取消；`Close` 可重复调用，并取消活动快照，关闭后新 `Snapshot` 返回 `ErrUnavailable`。
- 同一 capability 重复注册复用现有实例；Shutdown 只关闭自身实例，下一次注册重新加载配置并创建实例。

网络速率至少需要两个有效的非缓存样本；首个样本 `RateReady=false`。容器需要采集宿主机出口时，由部署配置提供只读 proc 网络挂载及接口白名单。

## 默认实例和多服务

默认不绑定进程默认实例。显式 `WithDefault` 后，仅在默认实例为空时绑定当前实例，后续服务不会覆盖已有默认实例。Shutdown 使用实例匹配清理默认引用，不会关闭其他服务的探针；原默认实例关闭后，不自动选择另一个默认实例。

多服务应通过 `From(app, name)` 或 `FromServiceContext` 取得自己容器中的实例。只有确实需要进程默认入口的调用方使用 `FromDefault`。

## 项目边界

core 不维护具体项目的工具清单、环境变量名、帮助主题、页面文案、数据库或远程节点上报。项目通过配置加载器提供所需工具及环境变量，并在 HTTP 响应中处理帮助入口和本地化。

`PrepareDir` 仅在工具存在时创建调用方指定的目录；不会推导任何供应商环境变量。环境变量通过 `DependencySpec.Env` 显式传入，命令执行复用 `pkg/execx`。

主机指标和工具检查是在当前进程所在节点执行。Worker 节点的采样与 Admin 的聚合、上报和过期判断属于独立集成工作。

## 命名迁移

2026-10-08：`core/hostprobe` 正式整合并改名为 `core/sysprobe`，旧导入路径不再保留。原 `Config`、`Snapshot`、`Probe`、`New` 的主机采样 API 分别迁移为 `HostConfig`、`HostSnapshot`、`HostProbe`、`NewHost`；组合探针使用新的 `Config`、`Snapshot`、`Service`、`New`。

真实消费方已选择 SDIngest 验证，项目只保留配置适配和产品响应处理。
