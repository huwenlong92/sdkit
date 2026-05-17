# Scheduler 使用说明

当前框架的定时调度统一使用 `core/crontab` 和项目侧 `crontab` 包。

业务侧新增定时任务时，按 [crontab.md](crontab.md) 注册模板、handler 和 DB 动态任务。`core/crontab.Scheduler` 是 crontab 模块内部的调度器接口，用于接入 `robfigcron` 等 driver，不作为独立业务模块使用。

相关文档：

- 使用方式：[crontab.md](crontab.md)
- 模块设计：[../modules/crontab.md](../modules/crontab.md)
