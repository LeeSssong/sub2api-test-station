# Native OpenAI Four-Switch Failover and Logical Request Monitoring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore ordinary OpenAI requests to the native Sub failover state machine, raise the OpenAI request-level account-switch ceiling from 2 to 4, and make Monitor V4 group health count one final logical-request outcome per client request.

**Architecture:** Keep the existing `extra_retry_count` settings JSON and API compatibility, but remove it from runtime routing and retry-budget decisions. Ordinary OpenAI text requests use native error classification, account-level `pool_mode_retry_count`, failed-account exclusion, and request-level `FailoverState`; only the OpenAI native switch ceiling changes to 4. T87 remains a read-time projection over `usage_logs` and `ops_error_logs`: group health uses the final logical request outcome, while account-management diagnostics retain physical-attempt evidence.

**Tech Stack:** Go 1.27, PostgreSQL SQL projections, existing Sub2API OpenAI gateway handlers, Monitor V4 repository/service/snapshot code, Go tests with `sqlmock`, existing frontend contracts unchanged.

**Spec:** `docs/superpowers/specs/2026-09-01-t87-logical-request-error-lifecycle-design.md`

## Global Constraints

- Preserve Sub native billing, usage, account cooldown, stream protocol, cancellation, and account-slot semantics.
- `extra_retry_count` remains readable/writable for compatibility but is not consulted by runtime selection or retry code.
- Ordinary OpenAI request failover allows at most 4 account switches; same-account retry remains controlled by `pool_mode_retry_count` and error classification.
- Failover remains forbidden after semantic output, usage/charge, side effects, unknown billing state, or client disconnect.
- Monitor V4 group health counts final logical requests, not physical attempts; automatic recovery is success, final user-visible errors count once.
- Do not add a second error fact table, historical backfill, production data mutation, GitHub Actions workflow, merge, push, or deployment from this worktree.
- Only the root release controller may edit `main`, the global task queue, the project progress ledger, or deploy.

### Task 1: Establish the native failover boundary with RED tests

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/openai_retry_budget_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go`
- Test: `upstream/sub2api/backend/internal/handler/failover_loop_test.go`

**Interfaces:**
- Consumes: `openAIMaxAccountSwitches`, `NewFailoverState`, `FailoverState.HandleFailoverError`, and the OpenAI request handler's native failover loop.
- Produces: failing tests proving ordinary OpenAI runtime does not read `extra_retry_count`, permits four native account switches, and still stops on unsafe replay.

- [ ] **Step 1: Write the failing native switch-limit test**

Add a focused test around the OpenAI handler's switch ceiling using the existing failover test fixture. Drive five sequential safe `UpstreamFailoverError` results across six distinct accounts and assert that the first five account transitions are attempted (initial account plus four switches), while the sixth candidate is not forwarded. Keep the test's error values `OutputStarted=false`, `UsageKnown=false`, `UnsafeToReplay=false`, and `NextAccountAction=NextAccountRetry` so the only expected boundary is the switch ceiling.

- [ ] **Step 2: Write the failing compatibility test for `extra_retry_count`**

Add a handler-level test that supplies a valid group policy with `extra_retry_count=0` and then a policy with `extra_retry_count=3`, injects the same safe native failover sequence, and asserts that runtime switch behavior is identical. This test must verify compatibility storage is accepted separately from routing behavior; it must not assert deletion of the setting field.

- [ ] **Step 3: Run the focused tests and verify RED**

Run:

```bash
cd upstream/sub2api/backend
go test -vet=off ./internal/handler -run 'Test(OpenAI.*Four|OpenAI.*ExtraRetryCount|FailoverState)' -count=1
```

Expected: failure because the current OpenAI ceiling is 2 and the unified runtime path still applies `extra_retry_count` through `openAIRetryBudget`.

### Task 2: Restore native OpenAI failover with a four-switch ceiling

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_retry_budget.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- Test: `upstream/sub2api/backend/internal/handler/openai_retry_budget_test.go`
- Test: `upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go`

**Interfaces:**
- Consumes: the existing native `FailoverState` contract and account-level `pool_mode_retry_count`.
- Produces: ordinary OpenAI text requests that use native failover with `maxAccountSwitches=4`; compatibility settings continue to round-trip but no longer alter runtime attempt budgets.

- [ ] **Step 1: Change the native OpenAI switch ceiling**

Change the OpenAI-specific constant from `2` to `4` in `openai_retry_budget.go`, retain the existing configuration clamp at the new ceiling, and ensure all OpenAI handler constructors/defaults use that constant. Do not change the Gemini or other platform ceilings.

- [ ] **Step 2: Remove `extra_retry_count` from runtime budget adoption**

Replace unified OpenAI request budget adoption in the ordinary text paths with the native request-level budget/state. The runtime must continue to call native `ShouldRetryNextAccount`, `pool_mode_retry_count`, `TempUnscheduleRetryableError`, account exclusion, OAuth 429 cooldown, and stream safety checks. `OpenAIUnifiedExtraRetryCount` may remain as a compatibility read helper only if existing settings/API code requires it, but no handler or scheduler decision may use its value to cap attempts or switches.

- [ ] **Step 3: Preserve the quality selector without making it a retry controller**

Keep T96 account ordering and explicit opt-in only where already enabled, but make its selection result feed the native failover loop. Remove or neutralize only the `UnifiedQuality`-specific switch-budget gate and `extra_used` accounting that blocks native failover. Do not alter image, Responses WebSocket, alpha-search, or non-OpenAI routing boundaries.

- [ ] **Step 4: Run the focused GREEN tests**

Run:

```bash
cd upstream/sub2api/backend
go test -vet=off ./internal/handler -run 'Test(OpenAI.*Four|OpenAI.*ExtraRetryCount|FailoverState|OpenAIUnified)' -count=1
go test -vet=off ./internal/service -run 'TestOpenAIUnifiedQuality|TestOpenAISchedulerGroupPolicy|TestOpenAI.*Retry' -count=1
```

Expected: native four-switch and compatibility tests pass; existing unsafe-replay and same-account retry tests remain green.

- [ ] **Step 5: Commit the runtime change**

```bash
git add upstream/sub2api/backend/internal/handler/openai_gateway_handler.go \
  upstream/sub2api/backend/internal/handler/openai_retry_budget.go \
  upstream/sub2api/backend/internal/handler/openai_retry_budget_test.go \
  upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go \
  upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go \
  upstream/sub2api/backend/internal/service/openai_account_scheduler.go
git commit -m "fix: restore native OpenAI failover with four switches"
```

### Task 3: Establish T87 logical-request aggregation with RED repository tests

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`
- Create: `upstream/sub2api/backend/internal/repository/logical_request_monitor_test.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`

**Interfaces:**
- Consumes: `usage_logs.logical_request_id`, `usage_logs.attempt_id`, `usage_completeness`, `unsafe_to_replay`, and `ops_error_logs` request/error fields.
- Produces: a repository projection that returns one final group-level `AccountMonitorWindowAggregate` per logical request and keeps account-level physical diagnostics available through existing account monitor queries.

- [ ] **Step 1: Add a recovered-cross-account RED fixture**

Use `sqlmock` to assert a query containing a logical-request CTE. The fixture must represent one request with attempt A on account 101 ending in upstream 502, followed by attempt B on account 202 with complete usage and positive actual cost. The expected group aggregate is one request, one success, zero final failures, and one logical group row—not two account attempts.

- [ ] **Step 2: Add a retry-exhausted RED fixture**

Represent one logical request with two failed attempts on different accounts and a final user-visible 503 error. The expected group aggregate is one request and one failure. The intermediate 502/503 attempts must not expand the failure denominator.

- [ ] **Step 3: Add unsafe and deterministic-error coverage**

Add fixtures for an already-output/usage-known failure and a single 404 request. Both must produce one final user-visible failure; neither may be classified as recovered success or silently removed.

- [ ] **Step 4: Run the repository tests and verify RED**

Run:

```bash
cd upstream/sub2api/backend
go test -vet=off ./internal/repository -run 'Test(LogicalRequest|ListGroupRealRequestAggregates)' -count=1
```

Expected: failure because the current group aggregate partitions by `group_id, account_id, request_key` and has no final logical-request terminal projection.

### Task 4: Implement T87 final logical-request projection for Monitor V4

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go` only if an existing DTO needs a non-breaking internal field
- Test: `upstream/sub2api/backend/internal/repository/logical_request_monitor_test.go`
- Test: `upstream/sub2api/backend/internal/service/monitor_v4_test.go`

**Interfaces:**
- Consumes: exact `logical_request_id`/`request_id` correlation and existing final usage/error evidence.
- Produces: group Monitor V4 real-request counts based on one final logical outcome per request, with no change to public response field names or account-management physical diagnostics.

- [ ] **Step 1: Build the correlation CTE**

In the group real-request aggregate query, derive a canonical key with `logical_request_id` first and `request_id` as the legacy fallback. Join usage/error records only through exact request identifiers. Never merge records solely by `client_request_id`, account name, timestamp proximity, or model.

- [ ] **Step 2: Select terminal evidence before aggregating**

Within each logical request, rank evidence in this order: complete successful usage/terminal evidence; final user-visible error; unsafe-to-replay or retry-exhausted stop; intermediate errors. Deduplicate repeated rows by attempt identity and preserve conservative `unknown` handling. A recovered upstream error row with `status_code=200` is not sufficient evidence of success by itself.

- [ ] **Step 3: Aggregate one final outcome per group logical request**

Partition the final projection by `group_id, request_key`, not by account. Count success only for complete final usage/protocol success. Count failures only for final user-visible failure/unsafe stop/retry exhaustion. Keep `first_token_ms` and `duration_ms` samples only from final successful requests, preserving existing P95 behavior.

- [ ] **Step 4: Keep account-level diagnostics unchanged**

Do not replace the account-management evidence queries with the group terminal projection. Existing account cards may continue to show physical account failures and attempt evidence; only the Monitor V4 group health aggregation changes to user-facing logical-request granularity.

- [ ] **Step 5: Run the repository/service GREEN tests**

Run:

```bash
cd upstream/sub2api/backend
go test -vet=off ./internal/repository -run 'Test(LogicalRequest|ListGroupRealRequestAggregates)' -count=1
go test -vet=off ./internal/service -run 'TestMonitorV4|TestAccountMonitor' -count=1
```

Expected: recovered requests count once as success, exhausted requests count once as failure, unsafe/404 requests remain failures, and existing probe/snapshot tests remain green.

- [ ] **Step 6: Commit the monitoring projection**

```bash
git add upstream/sub2api/backend/internal/repository/account_monitor_repo.go \
  upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go \
  upstream/sub2api/backend/internal/repository/logical_request_monitor_test.go \
  upstream/sub2api/backend/internal/service/account_monitor_service.go \
  upstream/sub2api/backend/internal/service/account_monitor_types.go \
  upstream/sub2api/backend/internal/service/monitor_v4_test.go
git commit -m "fix: aggregate Monitor V4 by logical request terminal"
```

### Task 5: Verify compatibility, build, and scope

**Files:**
- Modify: `docs/superpowers/reports/2026-09-01-native-openai-four-switches-logical-monitoring-verification.md`
- Modify: `docs/handoffs/2026-09-01-native-openai-four-switches-logical-monitoring-handoff.md`

**Interfaces:**
- Consumes: commits from Tasks 2 and 4 and the unchanged T87 specification.
- Produces: a `READY_FOR_ROOT_REVIEW` candidate report with exact test output, known baseline gaps, no deployment claim, and rollback guidance.

- [ ] **Step 1: Run direct combined verification**

```bash
cd upstream/sub2api/backend
go test -vet=off ./internal/handler -run 'Test(OpenAI.*Four|OpenAI.*ExtraRetryCount|FailoverState|OpenAIUnified)' -count=1
go test -vet=off ./internal/repository -run 'Test(LogicalRequest|ListGroupRealRequestAggregates|MonitorV4)' -count=1
go test -vet=off ./internal/service -run 'Test(OpenAIUnifiedQuality|MonitorV4|AccountMonitor)' -count=1
go build ./cmd/server
git diff --check
```

- [ ] **Step 2: Confirm no runtime use of `extra_retry_count`**

Run:

```bash
rg -n "OpenAIUnifiedExtraRetryCount|ExtraRetryCount|extra_retry_count" upstream/sub2api/backend/internal/handler upstream/sub2api/backend/internal/service
```

Expected: matches remain only in compatibility settings/DTO/parser and non-runtime diagnostic fields; no runtime switch/attempt gate reads the value.

- [ ] **Step 3: Write the verification report and handoff**

Record candidate branch/HEAD/tree, baseline root SHA, changed files, test results, known pre-existing failures, migration/config/data impact, `downtime_required` as unverified until root preflight, and rollback as restoring the previous verified image or disabling the existing scheduler feature switch if needed.

- [ ] **Step 4: Commit the report and handoff**

```bash
git add docs/superpowers/reports/2026-09-01-native-openai-four-switches-logical-monitoring-verification.md \
  docs/handoffs/2026-09-01-native-openai-four-switches-logical-monitoring-handoff.md \
  docs/superpowers/plans/2026-09-01-native-openai-four-switches-logical-monitoring.md
git commit -m "docs: hand off native failover and logical monitoring fix"
```

## Self-Review Checklist

- T87 remains unchanged as a specification; the implementation consumes its existing terminal-kind and correlation rules.
- No task deletes the compatibility field or changes its JSON/API contract.
- No task treats automatic recovery as a failure, but final user-visible errors remain failures.
- No task expands the public Monitor V4 response into a multi-metric health dashboard.
- Account-management diagnostics retain physical-attempt evidence.
- Native OpenAI four-switch behavior is tested independently from monitor aggregation.
- No migration, historical backfill, production mutation, or deployment is included.
