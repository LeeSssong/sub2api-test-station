# T99 Monitor V4 分组缓存 P95 交接

日期：2026-08-31
状态：`READY_FOR_ROOT_REVIEW`（仅本地候选；未合并、未推送、未部署）

## Delivered

- 新增 `cache_read_tokens_p95` 与 `cache_read_tokens_sample_count` 两个 Monitor V4 分组字段。
- 口径为所选窗口内最终成功真实请求的 `usage_logs.cache_read_tokens` P95；0 值参与，失败请求和主动探测排除。
- 无成功真实请求时返回 `null/0`，卡片显示 `--`。
- 现有成功率、TTFT、总耗时、5 分钟真实请求优先与主动探测兜底逻辑保持不变。
- 现有分组卡片从两列指标扩展为三列，仅新增“缓存 P95”，未增加新页面或辅助字段。

## Candidate

- 分支：`codex/t99-monitor-v4-group-cache-p95`
- 初始基线：`main@e9db36d4b5cf789ac85bbabdfb82aa2c4beb7479`
- 实现提交：`2e9a4b76c`
- 已无冲突刷新到 `main@5d77271b32990076b8b0344a3f1909c62192abc6`
- 刷新合并提交：`c2ec433ae`
- 最终交接文档提交只改变规格、报告与交接；根审时以该分支 `HEAD` 为候选 SHA。

## Verification

刷新后已通过：Go 1.27.0 repository/service/handler 定向测试、完整 `cmd/server` 编译、前端 11/11、类型检查、production build、gofmt 和 diff check。完整命令、基线阻断与未验证项见：

- `docs/superpowers/reports/2026-08-31-t99-monitor-v4-group-cache-p95-verification.md`

独立只读审查未发现 P0/P1；审查建议的 service/handler 字段透传测试已补齐。无 PostgreSQL 真实数据集分位数集成测试，repository 使用 sqlmock 锁定 SQL、列序与扫描映射。

## Release Boundary

- 无数据库迁移、配置变化、依赖变化、账号/分组写入或生产数据写入。
- `downtime_required=unverified until root preflight`；按变更形状预期为 false，但不能替代发布预检。
- 当前用户的“继续”不属于主站两条明确授权语义，禁止从本候选自行推送或部署。
- 根线程应先确认本候选仍基于最新 `main`，再发送精确 `AUTHORIZE_MERGE_TO_MAIN`；合并后只运行直接相关回归、构建和发布预检。

## Rollback

候选尚未上线，无运行时回滚。未来若发布后需撤回，在根 `main` 反转 T99 实现提交并走既有受控发布链；没有数据库或配置回滚步骤。
