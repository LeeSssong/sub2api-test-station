# T69 账号监控证据与评分回退交接

- 状态：`READY_FOR_ROOT_REVIEW`
- 原始基线：`main@c70f11193`
- 刷新后根 `main`：`main@031b58e4c`
- 候选提交：`b6e85e5ab9fb5c3f631391f376ed4473d742e72a`
- 候选 worktree：`.worktrees/t69-account-monitor-evidence-fallback`
- 候选分支：`codex/t69-account-monitor-evidence-fallback`
- 尚未合并、推送、部署或线上验收；候选未执行任何生产动作
- `downtime_required`：待根发布预检确认

## 原始目标与根因

账号监控在当前模型检测出现 `evidence_insufficient` 时，卡片可能丢失最近一次最终结果；当前窗口没有评分时，暂停或不可调度账号也可能显示空评分。质量样本此前还会因没有主动探测而丢弃真实调用。根因位于原生账号监控窗口证据与评分投影：真实请求样本被主动探测样本门控，且历史最终评分回退错误地复用 `schedulable` 条件。

## 实现范围

- `accountMonitorWindowEvidence` 保留真实调用样本，即使当前窗口没有主动探测；来源明确区分 `hybrid`、`monitor_probe`、`real_request`。
- 仅有真实调用、没有新鲜主动探测时，质量证据可见但仍为当前评分/排名 `pending`，真实调用不替代主动探测新鲜度门控。
- `historical_final` 评分回退不再受 `schedulable=false` 限制，因此暂停/不可调度账号可展示历史有效评分，但不会恢复实际调度资格。
- 当前 `evidence_insufficient` 继续如实表达当前证据不足；历史最终 `normal/abnormal` 只作为详情回退和展示来源，不伪装成当前检测结果。

未新增事实源、API、数据库迁移、配置项、生产写入或调度资格变更。

## 变更文件

- `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- 本交接文档

## 直接相关验证（2026-08-27 刷新后复核）

已重新执行并通过：

```bash
go test ./internal/service -run 'TestAccountMonitor' -count=1
go test ./internal/repository -run 'TestAccountMonitor' -count=1
go test ./internal/handler -run 'Test.*AccountMonitor|TestAccountMonitor' -count=1
go build ./cmd/server
gofmt -w internal/service/account_monitor_service.go internal/service/account_monitor_service_test.go
git diff --check
```

通过证据：service 与 repository 测试均返回 `ok`；handler 定向命令返回 `ok`（当前包无匹配测试，显示 `[no tests to run]`）；`go build ./cmd/server` 退出码为 0；gofmt 后工作区无代码变化；`git diff --check` 无输出。

## 发布、回滚与剩余风险

- 候选已刷新到 `main@031b58e4c`；根总控合并前仍需确认目标 `main` 未漂移，并确认候选差异仅包含本任务范围代码/测试与本交接文档。当前根已包含 T80/T81 组合候选；本次发布需以合并后完整 `main` 的直接门禁和发布证据为准。
- 推送、发布预检、蓝绿部署和线上账号监控专项验收只能从合并后已验证的 `main` 执行；本候选不自行合并、推送或部署。
- 发布预检若返回 `downtime_required=false`，按既有无停机发布链继续；若为 `true`，在停机动作前暂停等待授权。
- 线上专项验收需确认：账号 #131 当前仍显示 `evidence_insufficient` 且详情可见最近一次最终 `normal`；账号 #266/#276 在 `schedulable=false` 时可显示历史评分但不进入调度；真实调用-only、probe-only 与 hybrid 样本来源及排名门控符合约定。
- 回滚恢复到合并前 `main`，或恢复到本候选最终提交对应的上一稳定版本；若合并/发布/验收失败，保留本 worktree、分支和失败证据继续修复。
