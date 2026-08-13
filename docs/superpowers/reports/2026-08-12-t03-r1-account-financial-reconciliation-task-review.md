# T03-R1 Task 8 集成验证与任务审查证据

日期：2026-08-13（Asia/Shanghai）
候选：`codex/t03-r1-upstream-cost-persistence@32f8ec12572ad9d2e50eab054788d4ec0bf05454`
候选 tree：`ab19d7807763e1fcd03d0d0cd3f8f49d335f9fbb`
merge base：`19492c57da24270eb2b3e9b5d9727c2865aebb9e`
保留 stash：`stash@{0}`（未恢复、未删除）。

## 范围

本轮仅执行 Task 8 离线集成验证、负面门禁、冻结历史 RED 证据和 release-preflight 文档。没有修改应用实现、没有调用上游账单、没有访问生产、没有合并、推送、部署、GitHub Actions 或启动 T05。

## RED/负面门禁

- `git status --porcelain=v1` 在验证开始时无输出；分支、HEAD 和 merge base 与上方一致。
- `git diff --name-only 19492c57..HEAD | rg '^\\.github/workflows/'` 无输出：PASS。
- 相对 merge base 的 migration SQL delta 仅为新 `222_account_financial_reconciliation.sql`；既有迁移 SQL 未修改：PASS。
- 222 SHA-256：`47f786d6b2b020d0211a17d4ccd2bc6bb3774a315f483fdc0ac45657c9ee738e`。独立语句扫描无 `UPDATE`、`DELETE`、`DROP`、`TRUNCATE` 或 `ALTER TABLE usage_logs`：PASS；外键 `ON DELETE CASCADE` 不是迁移数据改写。
- scoped `ent/schema` 与 `internal/repository` 不含旧 direct `usage_logs` 成本字段：PASS。新 guard fixture 中的字段名称仅用于断言其不存在。
- **BLOCKED**：`rg -n 'GetByUsageID\\(' upstream/sub2api/frontend/src upstream/sub2api/backend/internal/handler/admin` 命中 `internal/handler/admin/usage_handler.go:75`。该处保留 `SubUpstreamCostService.GetByUsageID` 的兼容 fallback，仅在 `accountFinancialService == nil` 时触发，仍与“管理员读取零 read-time upstream HTTP”门禁冲突。

精确 brief guard 若扫描整个历史 migrations 目录的 `ALTER TABLE usage_logs`，会命中既有官方迁移；这不是候选新增越界，故以 changed-path/scoped guard 判断本候选。

## 冻结历史 RED

未建临时 worktree、未改历史。只读 `git show ce5691527a54cb2e7f8b3dabf624eb65e93fc177:<path>` 证明：

- `ent/schema/usage_log.go` 含 `upstream_actual_cost`、`upstream_cost_status`、`upstream_cost_reason`、`upstream_cost_recorded_at`；
- `internal/repository/usage_log_repo_insert.go` 在 INSERT 中使用这些字段；
- `migrations/221_usage_log_upstream_cost_persistence.sql` 含 `ALTER TABLE usage_logs` 和上述字段。

这是预期 RED：旧冻结候选违反当前“独立证据表、官方 usage_logs 不扩字段”合同。当前候选无 221 迁移，当前 UsageLog schema/insert 路径无这些字段。

## GREEN 集成矩阵

以下均为新鲜执行，退出码 `0`：

- `go test ./migrations -run 'Test(AccountFinancialReconciliationMigration|T03R1LegacyUsageLogFieldsAreAbsent)' -count=1`
- `go test ./internal/repository -run 'Test(AccountFinancial|UsageCostEvidence|MigrationsSchema|UsageLog)' -count=1`
- `go test ./internal/service -run 'Test(UsageCostEvidenceRegistrar|SubUpstreamCost|AccountFinancial|AccountFinancialAudit)' -count=1`
- `go test ./internal/handler/admin -run 'Test(AccountFinancial|AdminUsage.*(Evidence|UpstreamCost|Exception))' -count=1`
- `go test ./internal/handler -run 'TestUsageRecord.*(Evidence|Fallback)|TestGateway.*Usage|TestUsage.*Detail' -count=1`
- `go vet ./internal/service ./internal/repository ./internal/handler ./internal/handler/admin`
- `make build`
- `pnpm test:run` 指定 6 个 financial/usage 文件（6 files / 53 tests passed）
- `pnpm typecheck`
- `pnpm build`
- `git diff --check`

前端非阻断警告：pnpm `overrides` 迁移提示、Node localStorage experimental、Browserslist 数据陈旧、Vite dynamic/static import 与 chunk-size 提示。本轮未改依赖或拆包策略。

## 迁移、配置与后续门禁

- 迁移 222 为 expand-only 新增表/索引；Ent 相关 tests 已通过。
- 配置 delta：无 compose、`.env`、release、Caddy 或运行时配置路径。
- 无上游 HTTP、生产数据库写入、凭据/API Key/原始响应输出。
- 当前没有全分支 APPROVE。本报告不产生 `READY_FOR_ROOT_REVIEW`；独立终审必须处理上述 handler fallback。
