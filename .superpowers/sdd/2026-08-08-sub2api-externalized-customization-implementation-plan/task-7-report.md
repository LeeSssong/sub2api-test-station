# Task 7 Report

## 变更摘要

- 为 `request.completed` 与 `account.health_changed` 增加稳定、幂等的 UUIDv4-shaped event id。
- usage log repository 在 Ent 外层事务中复用 tx；无外层事务时用本地 SQL 事务将 usage_logs 与 externalization_outbox 同提交。
- account monitor 增加带 outbox 的生产构造器，健康事实仅在状态变化时写入事件；旧构造器保留测试/兼容行为。
- relay-ops projections 兼容实际响应模型、`observed_at`/`error_category` 与 `cost_usd` 最小事实。

## 规格映射

1. 最小 payload：请求事件只含 request/account/model/token/cost/latency/currency；健康事件只含 account/status/error/observed/probe 事实。
2. 稳定幂等：request identity 使用 request_id；health identity 使用 account/status/error/observed_at，映射为确定性 UUIDv4 形状。
3. 事务可见性：Append 接收 caller tx；Create 无 tx 时开启本地 tx，Append 失败回滚 usage insert；Ent tx 路径复用同一 tx client。
4. 异步故障隔离：核心没有同步 relay-ops 网络调用，消费者投递仍由 outbox claim/worker 处理。
5. 协议兼容：relay projections 接受新增字段并回退 `actual_cost` 到 `cost_usd`，忽略未知字段。

## RED

`go test ./internal/integration ./internal/handler ./internal/service -run 'Test(RequestCompleted|HealthChanged)' -v`

基线预期失败：`undefined: HealthChanged`，且无稳定 event identity 实现。

## GREEN / 全量 / race / vet

- `go test ./internal/integration ./internal/handler ./internal/service -run 'Test(RequestCompleted|HealthChanged)' -v` PASS
- backend `go test ./...` PASS
- backend `go test -race ./internal/integration ./internal/handler ./internal/service` PASS（无 race 报告）
- backend `go vet ./internal/integration ./internal/events ./internal/repository` PASS
- relay-ops `go test ./...` PASS
- `git diff --check` PASS

## 事务、幂等与敏感字段证据

- `events.Outbox.Append` 只接受 caller transaction，并以 `event_id` 主键 `ON CONFLICT DO NOTHING`。
- usage Create 的 outbox append 与 usage_logs insert 位于相同 SQL transaction；Append 错误返回并回滚。
- 事件构造器通过合同 Validate，payload 不包含 prompt、credential、cookie、authorization 或完整响应体；金额使用 decimal 字符串。

## 自审

路由、计费选择与实时转发决策未修改；变更限制在事件构造、持久化适配器与读模型兼容。

## 风险

带 outbox 的健康写入在读取历史状态失败时按“状态变化”继续发事件，可能产生一次重复健康事件；消费者 event_id 幂等可抑制重复副作用。旧测试构造器不启用外置 outbox，保留现有行为。

## Commit

待提交：`feat: emit minimal request and health events`

## Fix Round 1

- `CreateBestEffort` now bypasses the asynchronous batcher whenever the externalization outbox is configured and uses the same `createAtomic` transaction as `Create`; no post-flush or async补发 path is used.
- OpenAI usage snapshots now set `ActualResponseModel` before persistence, so request events do not depend on the later best-effort audit UPDATE.
- Health history query distinguishes `sql.ErrNoRows` from real database errors; real errors rollback and suppress event creation.
- Health identity uses canonical JSON encoding before hashing, covering delimiter collision regression.

Fix-round tests: `go test ./internal/integration ./internal/repository -run 'Test(HealthChangedEventIdentity|AccountMonitorRepositoryHealthHistoryError|UsageLogRequestEventIncludesActualResponseModel|RequestCompleted)' -v` PASS.

## Fix Round 2

- Health identity canonical JSON now contains only account_id, status, error_category and observed_at; probe_version remains payload-only. The collision regression also proves a probe-version-only change retains the same event id.
- Added production-configured `CreateBestEffort` transaction-boundary tests for the repository integration harness's isolated temporary PostgreSQL Testcontainers instance: one commits both usage_logs and externalization_outbox for the shared repository entry point, and one injects an outbox INSERT failure via a temporary trigger and proves neither row remains. `CreateBestEffort` continues to bypass batching when the outbox is configured, so this is one SQL transaction rather than a flush-then-append design.
- Added sqlmock supplementary rollback coverage for an Append error. The real PostgreSQL tests could not run locally because Testcontainers cannot locate rootless Docker; see command result below.
- JSON, Responses and WebSocket converge before persistence at the existing `UsageLogRepository.CreateBestEffort` entry; the shared-entry test verifies the repository behavior once rather than duplicating protocol fixtures.

Fix-round commands: `go test ./internal/integration ./internal/repository -run 'Test(HealthChangedEventIdentity|UsageLogCreateBestEffort|UsageLogRequestEvent)' -v` PASS. The isolated PostgreSQL Testcontainers command was attempted for both commit/rollback tests but is blocked in this environment: `panic: rootless Docker not found` during repository TestMain. The tests remain build-tagged and use a fresh container when Docker is available; sqlmock rollback coverage passed locally as the supplementary boundary check.

Controller verification reran the tagged integration tests with the active Colima socket explicitly configured:

`DOCKER_HOST=unix:///Users/gongtengxinwen/.colima/default/docker.sock TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock go test -tags=integration ./internal/repository -run '^TestUsageLogBestEffortExternalization' -count=1 -v`

Both `TestUsageLogBestEffortExternalizationCommitsUsageAndOutboxTogether` and `TestUsageLogBestEffortExternalizationRollsBackOnAppendFailure` passed against fresh Testcontainers PostgreSQL 18.1 and Redis 8.4 instances; package result `ok` in 54.258s. Testcontainers terminated the isolated containers after the run.
