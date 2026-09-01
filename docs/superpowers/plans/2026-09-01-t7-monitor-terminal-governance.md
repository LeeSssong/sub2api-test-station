# T7 Monitor Terminal Governance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Monitor V4 and administrator request diagnostics project one consistent terminal result per logical request while preserving native Sub retry, billing, probe, and account-slot behavior.

**Architecture:** Reuse `usage_logs`, `ops_error_logs`, existing logical/attempt identifiers, and existing probe terminal facts in bounded read-time SQL projections. A shared service-level terminal classifier will define the terminal kinds and correlation quality; Monitor V4 will consume the native repository projection, while administrator request details will expose the same terminal fields without creating a new fact table.

**Tech Stack:** Go 1.27, PostgreSQL CTE/window functions, `sqlmock`, existing Vue/TypeScript admin contracts and Vitest.

**Spec:** `docs/superpowers/specs/2026-09-01-t7-monitor-terminal-governance-design.md`

## Global Constraints

- Do not add logical-request, error, billing, or snapshot fact tables or migrations.
- Do not change native retry, account state, billing, concurrency-slot, safe-replay, or streaming semantics.
- Correlate with `logical_request_id`, fall back to `request_id`, and never merge by time, account name, or email.
- Count only `success` and `auto_retry_recovered` as successful requests; `missing` probe terminals are integrity alerts, not service failures.
- Keep user responses redacted and keep credentials, authorization, full request bodies, full upstream responses, and model output out of persistence and admin contracts.
- Do not modify global queue or progress ledger from this candidate worktree.

### Task 1: Define the terminal projection contract and classifier

**Files:**
- Create: `upstream/sub2api/backend/internal/service/logical_request_terminal.go`
- Test: `upstream/sub2api/backend/internal/service/logical_request_terminal_test.go`

**Interfaces:**
- Consumes normalized usage/error/probe evidence structs.
- Produces `LogicalRequestTerminal`, `CorrelationQuality`, and `ProbeTerminalKind` values used by repository projections and handlers.

- [ ] **Step 1: Write failing table-driven tests** for logical/request fallback correlation, attempt deduplication identity, terminal precedence, recovered success, exhausted failure, unsafe replay stop, and incomplete unknown; add probe `success`, `failed`, and `missing` tests.
- [ ] **Step 2: Run `go test ./internal/service -run 'TestLogicalRequestTerminal|TestProbeTerminal' -count=1` and confirm the new symbols/tests fail because the classifier is absent.
- [ ] **Step 3: Implement the minimal pure classifier and exact-key helpers with conservative `unknown` behavior.
- [ ] **Step 4: Re-run the focused service tests and confirm all cases pass.
- [ ] **Step 5: Run `gofmt` on the new files and commit `feat: define logical request terminal projection contract`.

### Task 2: Project Monitor V4 real requests by final logical terminal

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- Test: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`

**Interfaces:**
- Consumes the Task 1 terminal keys and existing `usage_logs`, `ops_error_logs`, and probe-terminal tables.
- Produces `MonitorV4GroupProjection` with logical request counts, final successes/failures, P95 samples from successful logical requests, and `MissingProbeTerminalCount`.

- [ ] **Step 1: Add failing SQL-contract tests** proving two failed attempts plus a final success produce one success, multiple final attempts produce one failure, and `usage_completeness=unknown` is excluded from success/failure service counts.
- [ ] **Step 2: Run the focused repository tests and confirm the SQL expectations fail against the current per-attempt projection.
- [ ] **Step 3: Implement CTEs that first bound the time window, derive exact logical keys, deduplicate physical attempts, select one final terminal by evidence precedence, and aggregate one row per group/logical request. Keep real-request buckets exclusive of probes.
- [ ] **Step 4: Add explicit probe terminal classification so missing terminal rows increment the integrity count without incrementing request or failure counts.
- [ ] **Step 5: Run the repository focused tests, `gofmt`, and `git diff --check`; commit `feat: project monitor v4 by logical request terminal`.

### Task 3: Unify administrator request details with terminal fields

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/ops_request_details.go`
- Modify: `upstream/sub2api/backend/internal/repository/ops_repo_request_details.go`
- Test: `upstream/sub2api/backend/internal/repository/ops_repo_request_details_test.go`
- Test: `upstream/sub2api/backend/internal/repository/ops_repo_request_details_integration_test.go`
- Modify: `upstream/sub2api/frontend/src/api/admin/ops.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/ops/components/OpsRequestDetailsModal.vue`
- Test: `upstream/sub2api/frontend/src/views/admin/ops/components/__tests__/OpsRequestDetailsModal.lifecycle.spec.ts`

**Interfaces:**
- Consumes the same exact correlation and terminal classifier contract as Monitor V4.
- Produces one administrator request row with `logical_request_id`, `correlation_quality`, attempt/failover/upstream-error counts, terminal kind/reason, user-visible and unsafe-replay flags, usage completeness, and final protocol/status fields.

- [ ] **Step 1: Add failing Go and Vitest assertions** for one row per logical request, intermediate errors hidden from the terminal result, and the terminal fields rendered without exposing sensitive payloads.
- [ ] **Step 2: Run the focused Go/Vitest tests and confirm they fail against the current combined usage/error list.
- [ ] **Step 3: Replace the raw union with a bounded CTE projection that deduplicates attempts and selects the terminal using the same precedence as Monitor V4.
- [ ] **Step 4: Extend the service/API DTOs and render the diagnostic fields in the existing modal, preserving user/admin redaction boundaries.
- [ ] **Step 5: Run focused backend and frontend tests, `pnpm typecheck`, and `git diff --check`; commit `feat: expose unified request terminal diagnostics`.

### Task 4: Wire, verify, and hand off the candidate

**Files:**
- Modify only if required: `upstream/sub2api/backend/internal/handler/monitor_v4_handler.go`, `upstream/sub2api/backend/internal/handler/handler.go`, `upstream/sub2api/backend/internal/service/wire.go`
- Create: `docs/handoffs/2026-09-01-t7-monitor-terminal-governance-handoff.md`

- [ ] **Step 1: Add or update wiring tests** proving the repository/service projection is used by Monitor V4 and admin request details without a second fact source.
- [ ] **Step 2: Run the complete direct verification set: focused Go service/repository/handler tests, `go build ./cmd/server`, frontend focused Vitest, `pnpm typecheck`, `pnpm build`, `gofmt`, and `git diff --check`.
- [ ] **Step 3: Re-read the spec and inspect the diff for migrations, writes, secret exposure, native retry/concurrency changes, and accidental unrelated files.
- [ ] **Step 4: Write the handoff with baseline SHA, candidate SHA, changed files, test evidence, no-migration/config/data-write statement, `downtime_required=false` as an unexecuted release assumption, rollback, and remaining risks.
- [ ] **Step 5: Commit `docs: hand off t7 monitor terminal governance` and report `READY_FOR_ROOT_REVIEW`; do not merge, push, deploy, or edit global ledgers.

## Verification Matrix

| Spec requirement | Verification |
| --- | --- |
| Multiple attempts ending success count once | Monitor V4 repository test + terminal classifier test |
| Multiple attempts ending failure count once | Monitor V4 repository test + admin projection test |
| `pool`, `eligible`, `attempted`, `filtered`, `upstream_failed` remain distinct | Existing account-monitor diagnostic contract tests plus any new DTO assertions |
| Probe success/failed/missing semantics | Account-monitor repository tests |
| Real request suppresses probe sample | Monitor V4 repository tests |
| Client/model responsibility excluded from service failure rate | Existing T97-compatible projection tests plus new terminal classification test |
| Safe replay boundary unchanged | Existing T87 streaming/replay tests; no handler retry code changes |
| No new fact source or migration | Diff guard and explicit migration/file-scope inspection |
