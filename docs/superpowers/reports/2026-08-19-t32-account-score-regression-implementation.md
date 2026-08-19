# T32 账号评分回归修复实现报告

## 状态

`READY_FOR_ROOT_REVIEW`

## 基线与候选

- 基线 `main` SHA：`dc51b37c9dbf73a87cccceab5815f129882812c5`
- 候选分支：`codex/t32-account-score-regression`
- 候选提交：`8b1248cb6`

## 根因与实现

- 原 runner `listPool` 只返回 `status=active && schedulable=true`，关闭调度但探测成功的账号从物理探测池消失。
- 原投影把 management paused 直接映射为 disabled，清空有新鲜探测证据账号的分数和排名。
- 现实现读取一次最近主动探测快照，仅当账号调度关闭且最近探测状态为 HTTP 400..599 的失败结果时跳过物理探测；其他 active 账号（含 `schedulable=false`）继续探测。
- 有新鲜 success 探测的 paused 账号按原成本/权重/成功率/延迟公式生成 normal、评分和排名；paused 仍保留管理/健康桶语义。
- paused 账号的 HTTP 4xx/5xx 变为 unavailable、无评分/排名；无记录、过期证据继续 stale/pending，无默认分数。
- 分组排名保留原可调度服务不可用 gate，仅允许 paused 行绕过 management gate，并继续要求主动探测评分状态和成本资格。

## 变更文件

- `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- `docs/superpowers/specs/2026-08-19-t32-account-score-regression-design.md`
- `docs/superpowers/plans/2026-08-19-t32-account-score-regression.md`
- `docs/superpowers/reports/2026-08-19-t32-account-score-regression-implementation.md`
- `docs/handoffs/2026-08-19-t32-account-score-regression-handoff.md`

## 验证

- `go test ./internal/service -run '^TestAccountMonitor' -count=1`：通过。
- `go test ./internal/service -run 'TestAccountMonitor(ProbeShouldStop|PausedProbeProjection|RunAllContinuesClosed)' -count=1`：通过。
- `go test ./internal/repository -count=1`：通过。
- `go vet ./internal/service ./internal/repository`：通过。
- `go test ./internal/service ./internal/repository ./internal/handler -count=1`：未通过；失败来自既有 OpenAI scheduler sticky 测试及 handler 错误中文化/stream 断言，与本次账号监控 diff 无关，未修改。
- `go test ./internal/service -count=1`：未通过；失败为既有 `OpenAIGatewayService_SelectAccountWithScheduler_*` 两项，与本次 service diff 无关。
- `git diff --check`：待最终提交前执行并记录。

## 迁移、配置与发布

- 数据库迁移：无。
- 配置变化：无。
- 评分公式/权重、计费、调度策略、数据库事实源：无变化。
- `downtime_required`：候选未运行根发布预检；按无迁移纯 service 变更预期为 `false`，最终以根发布预检为准。
- 未推送、未合并、未部署、未触碰生产、未修改全局队列/总账/发布证据。

## 回滚与剩余风险

- 回滚：根总控合并前删除/放弃候选提交即可恢复基线 SHA；无数据回滚。
- 剩余风险：真实生产账号状态/最近探测与本地 fixture 分布不同；根任务仍需在合并后运行直接回归、发布预检并做登录态排名专项验收。
- 组合 service/handler 全量套件存在与本任务无关的既有失败，根总控应按现有基线证据区分，不将其误归因于 T32。
