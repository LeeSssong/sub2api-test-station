# T104 Monitor V4 持久化快照恢复检查点

日期：2026-08-31（Asia/Shanghai），CONTEXT_RECOVERY checkpoint（本轮）

## 任务目标

- 将 Monitor V4 的分组统计从页面/调度切换时的即时全窗口 SQL，改为约每 5 分钟定时计算并持久化；页面和调度切换只读取最近一次成功快照。
- 保持已确认的指标口径：最终用户可见逻辑请求计数；5 分钟桶有真实请求时只用真实请求，当前桶最后一分钟仍无真实请求时用同桶一次主动探测兜底；成功率为选中成功逻辑请求数除以选中总逻辑请求数；TTFT/完整耗时只从成功样本计算，保留当前 P95 文案和前后 5% 截尾平均实现；缓存命中率沿用 T102 的成功真实请求 Sub 原生 Token 口径。

## 范围与硬边界

- 本任务只处理持久化统计读取路径和实际成功率统计问题；不恢复或新增 admission、slow-session、账号级额外并发控制。
- 不修改根 `main`、`docs/project/project-progress.md` 或 `docs/project/native-sub-task-package-queue.md`；不合并、推送、部署、停机、迁移、重启、切槽或触碰生产。
- T103 已废弃；已进入根 `main` 的 native-only guard 仅作为永久发布门禁，不属于 T104 实现范围。

## Git 与工作区事实

- 根目录：`/Users/gongtengxinwen/Documents/sub2api搭建`
- 根分支：`main`
- 根基线（当前事实）：`main@5e6ccee143f07ee34017c25e75979b74b6bcfc77`，tree `42dda8e317725a710340b5624bbda887cd1f6a50`
- 根 `main` 与 `origin/main`：commit/tree 一致，工作区干净；根最近提交为 `docs: register T105 OAuth rate-limit recovery`。根目录另有用户/总控正在维护的全局文档现场，未触碰。
- 本任务候选：`/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t104-monitor-v4-persisted-snapshot`
- 分支：`codex/t104-monitor-v4-persisted-snapshot`
- 候选 HEAD：`5e6ccee143f07ee34017c25e75979b74b6bcfc77`，tree `42dda8e317725a710340b5624bbda887cd1f6a50`
- 候选工作区：仅有未跟踪本恢复文件；候选已安全快进到根 `main`，`git rev-list --left-right --count HEAD...main` 为 `0 0`；尚无 T104 独有代码提交。
- 其他 worktree、未提交内容和保护窗口均保持原状，未执行 reset/clean/checkout/删除。

## 已读取与已确认

- 已完整读取根 `AGENTS.md`、`docs/project/native-sub-incremental-delivery-constraints.md`、`docs/project/native-sub-task-package-queue.md`、`docs/project/acceptance-station-global-constraints.md`。
- 已读取现有 T85、T97、T99、T101 规格/计划/报告/交接及当前代码目录；当前分支携带 T99 的规格/计划/报告/交接，但 T104 尚无独立实现规格/计划/报告。T99 文档仅作为已部署字段口径证据，不视为 T104 规格或实现。
- 现有调用链：`routes/user.go` 注册 `/monitor-v4` -> `monitor_v4_handler.go` -> `MonitorV4Service.Snapshot` -> `ProjectMonitorV4GroupsForGroups`；该路径每次 HTTP 请求计算窗口并扫描事实表。
- 现有运行器 `account_monitor_runner.go` 负责账号监控轮次、主动探测和桶终态 watchdog；没有 Monitor V4 快照仓储或 5 分钟快照刷新 ticker。
- 现有原始事实源仍为 `usage_logs`、`ops_error_logs`、`account_monitor_results`、`account_monitor_bucket_terminals`；快照只能作为派生缓存，不能替代这些事实源。
- 无发布、合并、部署、迁移、重启或切槽命令正在运行；未发现 Git 发布锁。根/候选及已登记 worktree 均保留，未执行 reset、clean、checkout、删除或覆盖。

## 未确认与证据缺口

- T104 尚未登记到全局队列/总账；本线程无权修改根总控文档，登记状态待根总控确认。
- 候选已在本轮安全快进到根 `main`，尚未开始 T104 运行时代码实现或直接测试；后续任何实现提交都必须留在此候选，且在 READY_FOR_ROOT_REVIEW 前保留该基线证据。
- T104 专属规格已写入 `docs/superpowers/specs/2026-08-31-t104-monitor-v4-persisted-snapshot-design.md` 并完成自审，待根总控依据用户既有确认书面批准；批准前不写实施计划、测试或运行时代码。
- 快照方案、worker 生命周期、窗口边界和可见分组权限策略已在规格中固定；迁移编号 `232` 仍需根总控整合时核对是否与其他候选冲突。
- Task 4 当前指针：候选 `codex/t104-monitor-v4-persisted-snapshot`，基线 `5e6ccee143f07ee34017c25e75979b74b6bcfc77`，已在 `monitor_v4_handler_test.go`、`api.spec.ts` 和 `HybridPerformanceView.spec.ts` 增加直接合同覆盖；验证报告为 `docs/superpowers/reports/2026-08-31-t104-monitor-v4-persisted-snapshot-verification.md`。
- Task 4 focused backend verification：repository/service 通过；handler 因既有 `handler_wiring_test.go` 参数数量错误及 `openAIAccountScheduleModel` 未定义编译阻塞。Frontend Vitest 因候选无 `node_modules` 阻塞，离线 pnpm 安装又被既有 lockfile/override mismatch 拒绝。`git diff --check` 与 native-only guard 通过。
- 当前边界：`READY_FOR_ROOT_REVIEW`。本线程不合并、不推送、不部署、不迁移、不修改根总账；根总控下一动作是审阅报告与候选 diff，并决定是否授权合并。无运行时、迁移、配置或生产数据变化。

## 恢复后的唯一第一步

等待根总控书面批准已自审的 T104 规格；获批后调用 `writing-plans`，再按计划先写失败测试、实现快照仓储/刷新循环/读取路径和缺失探测分母修正。

## 停止条件与恢复方式

- 发现根 `main`、其他 worktree 或生产状态发生未授权变化，立即停止并更新本文件。
- 任一实现/测试阶段再次中断，先在本文件追加当前 HEAD、changed files、测试命令/结果、未验证项和下一唯一动作，再暂停。
- 回滚仅通过保留候选分支/提交并由根总控在干净 `main` 上决定；本线程不执行回退或发布。
