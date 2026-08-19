# T32 账号评分回归修复交接

## 状态

`READY_FOR_ROOT_REVIEW`

## 交接摘要

T32 修复了账号监控投影将 paused 账号错误清空分数，以及 runner 排除关闭调度但探测成功账号的问题。runner 现在只在“调度关闭 + 最近原生主动探测失败且 HTTP 400..599”时停止；paused 成功探测保持 normal、评分和排名；无证据/过期继续 pending/stale；真实请求指标未参与评分。

## 基线与候选

- 基线 `main`：`dc51b37c9dbf73a87cccceab5815f129882812c5`
- 候选分支：`codex/t32-account-score-regression`
- 候选提交 SHA：`8b1248cb6`

## 变更文件

- `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- `docs/superpowers/specs/2026-08-19-t32-account-score-regression-design.md`
- `docs/superpowers/plans/2026-08-19-t32-account-score-regression.md`
- `docs/superpowers/reports/2026-08-19-t32-account-score-regression-implementation.md`
- `docs/handoffs/2026-08-19-t32-account-score-regression-handoff.md`

## 直接相关测试

- 通过：`go test ./internal/service -run '^TestAccountMonitor' -count=1`
- 通过：T32 focused stop/projection/runner 三项回归。
- 通过：`go test ./internal/repository -count=1`
- 通过：`go vet ./internal/service ./internal/repository`
- `go test ./internal/service -count=1` 与 service+repository+handler 组合套件包含既有 scheduler sticky、错误中文化和 stream 断言失败；未修改这些范围。
- `git diff --check`：通过。

## 未验证项

- 未做生产登录态、真实账号探测调用次数或线上排名验收。
- 未执行根发布预检、推送、合并、部署或线上回滚演练。

## 迁移/配置/停机

- 迁移：无。
- 配置：无。
- `downtime_required`：候选预期 `false`，最终以根发布预检为准。

## 回滚与风险

- 根合并前回滚到基线 `dc51b37c9dbf73a87cccceab5815f129882812c5`。
- 根合并后按发布链保留旧镜像/旧 SHA 回退；无数据回滚。
- 风险仅在生产状态组合和真实探测时序尚未验证；候选未改变评分公式、权重、计费、调度策略或数据库事实源。

## 根任务动作

根总控审查候选提交和本交接，合并前在目标 `main` 上运行直接账号监控回归与发布预检；本任务不自行合并、推送、部署或修改全局队列/总账。
