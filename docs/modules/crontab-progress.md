# crontab 状态记录

记录时间：2026-05-17

## 已完成

- `crontab` 根包继续保持服务入口职责，具体 store、lock 等 adapter 放到 `infra`。
- 根包 `crontab/router.go` 承载模板显式注册；模板查询和 DB 任务管理已收敛到 `core/crontab.Service`。
- 根包 `crontab/provider.go` / `register.go` 已按 worker 服务方式接入骨架注册。
- crontab middleware runtime 已删除，runtime governance 统一由 `core/crontab.Registry.Dispatch` 接管。
- `crontab/demo_*.go` 已收敛为 `corecron.Template` 声明和示例执行实现，根包 `router.go` 统一注册。
- 模板采用显式 router 注册，不再通过 `init()` 隐式注册。
- `cron run <entry_id>` 已改为执行 DB 任务实例，不再把模板当运行对象。
- `cron run-template <template_key>` 保留为模板能力调试入口。
- 同一任务实例并发执行时返回冲突，错误为 `ErrJobRunning`，CLI/Admin API 提示“该任务正在执行中”。
- 任务锁粒度已改为任务实例，默认锁 key：`crontab:entry:<entry_id>`。
- 后台 Admin API 已补齐：
  - `GET /crontab/runtime`
  - `POST /crontab/run`
  - `POST /crontab/start`
  - `POST /crontab/stop`
  - `POST /crontab/reload`
- `POST /crontab/run` 执行 DB 任务，支持 `id` 或 `entry_id`。
- Admin 进程内增加了 crontab 控制器，用于保存由 API 启动的 scheduler 实例。
- DB 动态任务覆盖边界已收紧并写入文档。
- crontab 测试已移动到 `crontab/tests`，并按模块放到子目录。
- 已补真实 DB/Redis 集成测试，覆盖任务锁冲突和 Admin run API 锁冲突。
- Jaeger/OTel 短命令退出前增加 tracing shutdown flush，使用者平时不需要关心。
- Template-Driven Runtime freeze 已完成，Template 是唯一 runtime definition source。
- failure callback 已收口到 `corecron.UseFailureHandler(...)`。
- crontab operations facade 已接入 Admin，Admin 不再直接维护模板 catalog，也不直接操作 `models.SystemCrontab`。

## 已定规则

- 模板是能力定义，任务才是运行对象。
- 同一个模板可以被多条 DB 任务引用。
- 默认互斥粒度是任务实例，不是模板。
- DB 动态任务只配置任务实例字段：
  - `template_key/name`
  - `label`
  - `spec`
  - `payload`
  - `enabled`
- DB 动态任务不覆盖执行能力和执行策略：
  - `handler`
  - `mode`
  - `payload_schema`
  - `payload_format`
  - `validate_payload`
  - `timeout`
  - `skip_if_running`
  - `distributed`
  - `lock_ttl`
  - `queue`
  - `task_type`
- `queue/task_type` 是早期 queue 模式预留字段，当前 local 模板不暴露、不使用。
- 分布式锁默认由全局 `crontab.lock` 配置控制。
- `crontab/demo_*.go` 同文件放模板定义和示例实现；模板 router、Registry 构建归根包 `crontab`，模板查询和 DB 任务管理通过 `core/crontab.Service` 暴露，外部适配归 `crontab/infra`。
- `crontab` 服务接入走 `Provider()`，`command/serve` 只负责 blank import。

## 未完成

- `cron stop` 还不是 daemon stop。
  - 当前只输出停止说明。
  - 没有 pidfile、后台 daemon 管理、systemd/supervisor/docker 控制封装。
- `cron reload` 还不能通知已经运行中的 cron 进程 reload。
  - 当前 CLI reload 只在本次命令进程内加载并校验任务。
  - 长驻进程仍靠 `reload_interval` 自动 reload。
- 后台 UI 尚未调整。
- DB 存储模型中仍存在非覆盖字段：
  - `queue`
  - `task_type`
  - `timeout_sec`
  - `timeout_seconds`
  - `skip_if_running`
  - `distributed`
  - `lock_ttl_seconds`
  这些字段当前作为执行策略快照或运行展示字段存在，后台动态任务不通过 DB 覆盖它们。
- 存量 DB 数据治理未处理。
  - 如果库里存在未注册模板名或不可用任务记录，仍可能显示但无法运行。
  - 后续可补迁移或清理脚本。

## 已验证

- `go test ./core/crontab ./crontab/... ./app/admin/handler/system ./app/admin/handler/system/tests ./app/admin` 通过。
- 真实 DB/Redis `TestIntegrationRunEntryReportsRealRedisLockConflict` 通过。
- 真实 DB/Redis `TestIntegrationRunCrontabReportsRealRedisLockConflict` 通过。

## 下次建议顺序

1. 先决定是否需要做真正的 daemon stop / remote reload。
2. 如需做 daemon 控制，先选控制方式：pidfile、Unix socket、HTTP 管理端口，或完全交给外部服务管理器。
3. 最后再处理 DB 字段收敛和存量任务数据治理。
