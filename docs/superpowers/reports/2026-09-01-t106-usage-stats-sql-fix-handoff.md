# T106 用户用量汇总 SQL 修复交接

- Task: T106
- Branch: `codex/t106-usage-stats-sql-fix`
- Baseline: `b2d504797fa2fdb869f0db35f7325dca2dfa6664`
- Status: `READY_FOR_ROOT_REVIEW`
- Deployment: 未执行
- Push: 未执行

## Changes

- 恢复 `GetStatsWithFilters` 的单次 scoped 聚合 SQL：唯一一次读取 `usage_logs`，然后从 `scoped` 通过 `GROUPING SETS` 返回总计、入口 endpoint、上游 endpoint 和 endpoint path。
- 补回与现有 Go scanner 一致的 13 列结果形状，消除 `%!s(MISSING)` 和后续扫描列数错误。
- CTE 复用 `effectiveAccountCostSQL("")`，保持 `account_cost` 优先、历史行回退 `account_stats_cost/total_cost * account_rate_multiplier` 的原生有效账号成本口径。
- 保留所有既有 filters、参数顺序、`usageLogNormalQueryFilter`、Token/消费聚合和 endpoint 排序语义。
- 新增 SQL 结构与结果映射回归测试，并更新三条已过时的 stats sqlmock 夹具到单查询契约。

## TDD Evidence

RED:

```text
go test ./internal/repository -run TestUsageLogRepositoryGetStatsWithFiltersUsesSingleScopedAggregate -count=1
internal/repository/usage_log_repo_stats.go:1004:3: fmt.Sprintf format %s reads arg #2, but call has 1 arg
FAIL
```

GREEN:

```text
go test ./internal/repository -run 'TestUsageLogRepositoryGetStatsWithFilters|TestUsageLog_GetStatsWithFilters' -count=1
ok github.com/Wei-Shaw/sub2api/internal/repository

go test ./internal/repository -count=1
ok github.com/Wei-Shaw/sub2api/internal/repository

go test -tags=unit ./internal/repository -run 'TestUsageLogRepositoryGetStatsWithFiltersUsesSingleScopedAggregate|TestEffectiveAccountCostSQL' -count=1
ok github.com/Wei-Shaw/sub2api/internal/repository
```

Formatting and diff:

```text
gofmt -d internal/repository/usage_log_repo_stats.go internal/repository/usage_log_repo_request_type_test.go internal/repository/usage_log_repo_stats_query_test.go
# no output

git diff --check
# no output
```

## Unverified

- `go test -tags=integration ./internal/repository -run TestUsageLog_GetStatsWithFilters_AggregatesAndEndpoints -count=1` could not start because this worktree host has no rootless Docker; testcontainers panicked with `rootless Docker not found` before the test ran.
- No production API, database, logs, credentials, deployment or online verification was accessed from this task.

## Root Controller Gates

1. Refresh/review the candidate against the then-current clean `main`; the initial baseline predates later root-ledger/deployment commits and `origin/main` consistency must be rechecked.
2. Merge only through the root controller, then run the same focused repository tests from clean root `main`.
3. Push `main` before any deployment and ensure root `HEAD` commit/tree equals `origin/main`.
4. Obtain one of the explicit production authorization phrases defined by the acceptance-station constraints before main-site deployment; if preflight reports `downtime_required=true`, stop for explicit downtime authorization.
5. Deploy only through the reviewed local/host chain, never GitHub Actions, and perform online `/api/v1/usage/stats` verification after promotion.
