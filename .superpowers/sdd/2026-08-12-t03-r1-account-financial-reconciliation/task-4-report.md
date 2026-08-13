# T03-R1 Task 4 Report

状态：进行中（fix round 1 本地修复与定向验证已完成；仍待独立复审，且 fresh PostgreSQL harness 受环境阻断。未合并、未推送、未部署、未线上验证）。

基线：`e01a869040fadab4594e89d75e772a2345652e3c`。提交：见本线程最终回报。

## 范围

交付本地财务快照、异常隔离/核对、OAuth literal `oauth` 每日成本、今日营收/成本覆盖、余额快照和既有审计日志适配。没有上游 HTTP、延迟任务、重试、回填、估算、汇率、`usage_logs` 修改、普通用户 DTO、handler/API/frontend、schema/migration/generated Ent 或 Task 5+ 改动。`audit_log_service.go` 保持不变，财务审计通过其已有 `Record(*AuditLog)` 入队接口。

## TDD 证据

RED（真实失败）：

```text
go test ./internal/repository -run TestAccountFinancialRepository -count=1
go test ./internal/service -run 'Test(AccountFinancial|AccountFinancialAudit)' -count=1
```

在生产接口不存在时分别失败于 `undefined: NewAccountFinancialRepository`、`undefined: AccountFinancialSnapshot`、`undefined: UsageCostReviewInput` 等，修正 SQLite driver/foreign-key 测试夹具后仍保持为缺实现失败。

GREEN：

```text
go test ./internal/repository -run TestAccountFinancialRepository -count=1   # PASS
go test ./internal/service -run 'Test(AccountFinancial|AccountFinancialAudit)' -count=1   # PASS
go test ./internal/service -run 'TestAccountFinancial.*(Review|Override|OAuth|Beijing|24H|7D|31D|Balance)' -count=1   # PASS
go test ./internal/service ./internal/repository -run '^$'   # PASS (compile)
go vet ./internal/service ./internal/repository   # PASS
git diff --check   # PASS
```

## 公式 fixture 复算

服务测试使用固定 `Asia/Shanghai` 时钟。今日 fixture 的正常营收/成本为 `100/40`，覆盖基数为 `95/35`；随后正常 `20/8` 和已核对异常 `10/3` 继续累加，结果为营收 `125`、成本 `46`、利润 `79`、利润率 `79/125 = 0.632`。待核对 `confirmed_zero`/`unavailable` 只增加异常数和受影响营收，不进入四项金额。收入为零时 margin 为 `nil`。

24h 测试只取滚动时间范围并忽略每日覆盖；7d/31d 使用北京时间业务日窗口。用户余额测试确认 `SUM(users.balance) WHERE deleted_at IS NULL`，包含 disabled 用户，软删除用户排除，`frozen_balance` 不相加。

## 事务、并发和 cutoff

repository 的多语句报告读取使用只读 `REPEATABLE READ` Ent 事务，确保 setting、余额、账号、流水、证据、复核和日值来自同一数据库快照；覆盖写使用 `SERIALIZABLE` 事务冻结 evidence/review 两套 cutoff 并写入同一日值。唯一 `usage_log_id` 约束实现 review 幂等；重复 review 返回已有结果，不重复插入。`FreezeReviewFilter` 先冻结筛选结果中的最大异常 usage ID，`ReviewFiltered` 只处理 `ID <= max_usage_log_id` 且再次应用同一 account/time filters，返回 `matched/updated/skipped/cutoff/max_usage_log_id`。测试在冻结后插入更大 usage，确认新流水仍 pending。正式 PostgreSQL race 集成测试不在本任务范围，生产发布前由根任务专项门禁决定。

## Task 5 可消费接口

`NewAccountFinancialRepository(*ent.Client) service.AccountFinancialRepository`

`NewAccountFinancialService(repo AccountFinancialRepository, now func() time.Time) *AccountFinancialService`

`(*AccountFinancialService).GetReport(ctx context.Context, range AccountFinancialRange) (*AccountFinancialReport, error)`

`ReviewOne(ctx context.Context, UsageCostReviewInput) (*UsageCostReviewResult, error)`

`ReviewSelected(ctx context.Context, []UsageCostReviewInput) ([]UsageCostReviewResult, error)`

`ReviewFiltered(ctx context.Context, ReviewFilteredInput) (*ReviewFilteredResult, error)`

`SetOAuthDailyCost(ctx context.Context, OAuthDailyCostInput) (*FinancialMutationResult, error)`

`SetTodayOverride(ctx context.Context, TodayOverrideInput) (*FinancialMutationResult, error)`

`GetUsageEvidence(ctx context.Context, usageLogID int64) (*UsageFinancialEvidence, error)`

All report account rows and summary cards carry the repository snapshot's same `GeneratedAt`.

## 审计和风险

`AccountFinancialAudit` writes only redacted actor/request/account/day/old-new/cutoff/result fields to the existing audit queue; request bodies, credentials and raw upstream errors are omitted. Writes validate finite non-negative amounts, today's Beijing date, and literal OAuth type in the repository/service boundary.

风险：本地 SQLite tests do not prove PostgreSQL isolation/race behavior; the explicit `REPEATABLE READ`/`SERIALIZABLE` paths should receive a fresh PostgreSQL concurrent transaction test in root review. 7/31 daily semantics are represented by Beijing-day projection and require Task 5 API integration to expose per-day completeness. No deployment evidence exists in this worktree; `downtime_required` is therefore not assessed here and remains a root deployment gate.

## Fix round 1 takeover evidence (2026-08-13)

接管基线为 `5a8d830b2de8063e2b99876e13e044b0e1930cdb`。没有 reset、丢弃或重做原 implementer 的共享工作区改动；保留并复审了新 PostgreSQL 集成用例和 `SUB2API_TEST_POSTGRES_TMPFS=1` opt-in harness。默认未设置该变量时 Postgres container options 保持原状；只有显式设置为 `1` 才会附加 tmpfs。

本轮关闭独立复审列出的 3 Critical + 5 Important：

- 读取和写入统一使用 production canonical key `t03_r1_account_financial`；SQLite regression 证明 `enabled_at` 与余额快照会被读取。
- `CreateReview` 和 `ReviewFiltered` 在 `SERIALIZABLE` 事务中重查 canonical activation、post-enable、literal non-`oauth`、无既有 review，以及不存在 `confirmed` evidence；confirmed、OAuth 和 pre-enable usage 返回 `ErrFinancialReviewNotEligible`，missing/unavailable pending usage 可核对且重复核对幂等。
- 所有五个 mutation entrypoints 需要审计依赖并 fail closed；逐笔、选中、筛选、OAuth 日成本和日覆盖均记录脱敏 actor/request/account/day/old/new/cutoff/result。筛选/选中核对按每条实际结果记录真实 old/new；未产生写入的失败也记录失败结果。
- 今日窗口显式从北京时间 00:00 开始；不再使用 `Time.Truncate(24h)`。
- OAuth 已填写账号日现在贡献当天营收和成本；7/31 同样保留每日覆盖，覆盖后新增的同日官方营收继续累加；缺成本日仍整体排除。
- `ListExceptions` 支持页码、search、evidence/review status，保留 persisted `reason_code` 与结构化账单 trace，并可按 `pending` 或 `reviewed` 查询。
- 今日覆盖按 revenue/cost 维度分别返回真实 old/new、`MutationKind` 及两个 cutoff；cost-only 覆盖不会错误返回 revenue 值。
- `ReviewFiltered` 在单个 `SERIALIZABLE` transaction 内读取筛选集合、冻结 zero cutoff、按 cutoff 与所有 eligibility/filter 条件复查、创建 review、计算 matched/updated/skipped；任一行失败 rollback，无部分写入。PostgreSQL integration test 覆盖注入失败 rollback、newer ID exclusion、同一 account/day 唯一日值与 cost old/new。

本轮首先运行原始与新增聚焦矩阵，第一次发现 `TestAccountFinancialServiceAuditsEveryMutation` 预期 5 条审计而 stub 的 `ReviewFiltered` 没有返回逐条 review result，实际得到 4；只修正测试 stub 使其模拟真实 repository 返回的 review results，随后整套矩阵通过：

```text
cd upstream/sub2api/backend
go test ./internal/repository -run TestAccountFinancialRepository -count=1
ok github.com/Wei-Shaw/sub2api/internal/repository 3.614s

go test ./internal/service -run 'Test(AccountFinancial|AccountFinancialAudit)' -count=1
ok github.com/Wei-Shaw/sub2api/internal/service 0.701s

go test ./internal/service -run 'TestAccountFinancial.*(Review|Override|OAuth|Beijing|24H|7D|31D|Balance)' -count=1
ok github.com/Wei-Shaw/sub2api/internal/service 0.694s

go test ./internal/service -run 'TestAccountFinancial(OAuthOverrideKeepsLaterRevenueForTodayAndSevenDays|ListExceptionsFiltersReviewedRows)|TestAccountFinancialServiceReviewAuditUsesTruthfulOldAndNewValues' -count=1
ok github.com/Wei-Shaw/sub2api/internal/service 0.724s

go test ./internal/service ./internal/repository -run '^$'
ok github.com/Wei-Shaw/sub2api/internal/service 0.931s [no tests to run]
ok github.com/Wei-Shaw/sub2api/internal/repository 1.546s [no tests to run]

go vet ./internal/service ./internal/repository
PASS (exit 0)

git diff --check
PASS (exit 0)
```

Fresh PostgreSQL verification was attempted through the repository harness, with the only new behavior explicitly enabled:

```text
SUB2API_TEST_POSTGRES_TMPFS=1 SUB2API_TEST_POSTGRES_IMAGE=postgres:15-alpine \
  go test -tags integration ./internal/repository -run TestAccountFinancialRepositoryPostgresTransactions -count=1 -v
```

It was blocked before `TestAccountFinancialRepositoryPostgresTransactions` or migrations ran. Exact environment error: `panic: rootless Docker not found` from `github.com/testcontainers/testcontainers-go/internal/core.MustExtractDockerHost` during repository `TestMain`; command exited 1. No container, database, migration or production state was changed. This leaves fresh PostgreSQL transaction/concurrency execution unverified despite the checked-in integration coverage.
