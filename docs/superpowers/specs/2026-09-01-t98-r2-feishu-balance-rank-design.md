# T98-R2 飞书余额新鲜度与排名投影修复设计

## 问题证据

生产只读数据表明，CX 告警使用 `2026-09-01 11:56:40+08` 的 `1.49166957 USD`，SHUAI 告警使用 `2026-09-01 11:56:35+08` 的 `0.065142 USD`。余额快照刷新位于账号主动探测成功路径之后；当真实流量覆盖账号所属全部分组时，主动探测提前跳过，余额刷新也被跳过，导致充值后仍沿用旧快照。

通知读取 `AccountMonitorService.List()`。该路径生成 `GroupRank`，但分组行会清空 `SchedulerRank`；通知只复制 `SchedulerRank`，所以卡片全部显示“未排名”。

## 目标与边界

- 同一规范化 BaseURL 仍以 `observed_at` 最新的有效 API Key 可用余额为准，不求和、不取最大值。
- 余额刷新不再依赖主动探测是否执行；账号监控周期触发时，活跃 API Key 仍刷新余额。
- 飞书通知读取能生成原生 `SchedulerRank` 的 24 小时监控投影。
- 保持余额 API、快照结构、阈值、去重节奏、凭据登记簿和卡片格式不变。
- 不新增迁移、配置、余额事实源、生产数据写入或真实飞书发送。

## 方案

在现有 `AccountMonitorService.runAll` 中把 `refreshAuxiliary(... RefreshBalance:true)` 从“主动探测成功后”提升为每个应处理账号的独立辅助刷新；主动探测跳过时仍执行余额刷新。通知读取改为 `ListWindow(ctx, "24h")`，复用原生分组调度投影，继续由 `SchedulerRank` 提供排名。

## 测试与验收

- 回归测试证明真实流量跳过主动探测时仍调用余额刷新。
- 回归测试证明多个账号快照按最新有效 `observed_at` 选值。
- 回归测试证明通知投影复制每个账号所有所属分组的 `SchedulerRank`。
- 运行 T98 service/notify 定向测试、必要 Go build、gofmt 和 `git diff --check`。

## 发布边界

仅提交候选 worktree，完成后进入 `READY_FOR_ROOT_REVIEW`。本候选不得自行合并、推送、部署或发送真实飞书消息。
