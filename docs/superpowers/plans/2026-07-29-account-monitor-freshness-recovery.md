# Account Monitor Freshness Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure account monitor collection cannot remain stuck and concurrent manual refreshes recover fresh evidence without HTTP 409 conflicts.

**Architecture:** Add bounded per-account probe contexts and replace the long-held run mutex with a short-lived in-flight run record. Full refresh callers coalesce onto one physical run; single-account refresh waits for the active full run before claiming exclusivity.

**Tech Stack:** Go 1.24, `context`, channels, `sync.Mutex`, Go testing

## Global Constraints

- Keep the existing stale threshold: twice the configured monitor interval.
- Persist timed-out probes as fresh failed observations with error code `timeout`.
- A joining caller's cancellation must not cancel the active leader.
- Do not change monitor projection JSON or HTTP endpoint paths.

---

### Task 1: Bounded Account Probes

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`

**Interfaces:**
- Consumes: `AccountTestService.ProbeAccountConnection(context.Context, int64, string, string, string)`
- Produces: `probeAccount(context.Context, Account) AccountMonitorProbeResult` that always returns within `accountMonitorProbeTimeout`

- [ ] **Step 1: Write the failing timeout test**

Add a controllable probe function seam to the service test fixture and a test
named `TestAccountMonitorServiceRunAllBoundsBlockingProbe`. The fake waits for
`ctx.Done()`. Assert that `RunAll` returns, persists one result with status
`failed` and error code `timeout`, and a second `RunAll` call is accepted.

- [ ] **Step 2: Run the timeout test and verify RED**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run TestAccountMonitorServiceRunAllBoundsBlockingProbe -count=1
```

Expected: FAIL because the probe inherits an unbounded background context and
the test's guard deadline expires.

- [ ] **Step 3: Implement the minimal bounded probe**

Introduce `accountMonitorProbeTimeout` and derive a timeout context inside
`probeAccount`. Ensure a deadline error is passed through
`buildAccountMonitorProbeResult` so classification returns `timeout`.

- [ ] **Step 4: Run the timeout test and verify GREEN**

Run the command from Step 2. Expected: PASS.

### Task 2: Coalesced Full Refresh

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`

**Interfaces:**
- Produces: process-local in-flight record containing `done chan struct{}`,
  `completed int`, and `err error`
- `RunAll(context.Context, int64) (int, error)` joins an active full run

- [ ] **Step 1: Write failing overlap and cancellation tests**

Add `TestAccountMonitorServiceConcurrentRunAllJoinsInFlightRun`, proving two
callers produce one physical batch and receive the same result. Add
`TestAccountMonitorServiceJoiningRunAllHonorsCallerCancellation`, proving a
cancelled waiter returns `context.Canceled` while the leader finishes normally.

- [ ] **Step 2: Run both tests and verify RED**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMonitorServiceConcurrentRunAllJoinsInFlightRun|TestAccountMonitorServiceJoiningRunAllHonorsCallerCancellation' -count=1
```

Expected: FAIL because the second caller currently receives
`account monitor run already in progress`.

- [ ] **Step 3: Implement the in-flight run record**

Replace `runMu.TryLock` with a state mutex and completion channel. Split the
physical batch into a private leader method. Publish the leader result before
closing the channel and clearing active state. Waiters select between
`run.done` and their own `ctx.Done()`.

- [ ] **Step 4: Run both tests and verify GREEN**

Run the command from Step 2. Expected: PASS.

### Task 3: Waiting Single-Account Refresh

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`

**Interfaces:**
- `RunOne(context.Context, int64, int64) (AccountMonitorProbeResult, error)`
  waits for active ownership and respects caller cancellation

- [ ] **Step 1: Write the failing single-account overlap test**

Add `TestAccountMonitorServiceRunOneWaitsForInFlightRunAll`. Hold the full run
at the probe boundary, start `RunOne`, assert it does not return a conflict,
release the full run, and assert the single-account probe completes.

- [ ] **Step 2: Run the test and verify RED**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run TestAccountMonitorServiceRunOneWaitsForInFlightRunAll -count=1
```

Expected: FAIL with the existing in-progress error.

- [ ] **Step 3: Implement exclusive waiting**

Use the same in-flight ownership helper for `RunOne`. Wait for an active run,
claim a new exclusive record, execute the single probe, and always publish and
release ownership.

- [ ] **Step 4: Run the test and verify GREEN**

Run the command from Step 2. Expected: PASS.

### Task 4: Regression Verification

**Files:**
- Verify: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- Verify: `upstream/sub2api/backend/internal/service/account_monitor_probe_test.go`
- Verify: `upstream/sub2api/backend/internal/service/account_monitor_runner_test.go`
- Verify: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`

**Interfaces:**
- Consumes all behavior produced by Tasks 1-3.
- Produces a verified account-monitor change ready for integration.

- [ ] **Step 1: Format changed Go files**

```bash
gofmt -w upstream/sub2api/backend/internal/service/account_monitor_service.go upstream/sub2api/backend/internal/service/account_monitor_service_test.go
```

- [ ] **Step 2: Run focused account monitor tests**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run AccountMonitor -count=1
go test ./internal/handler/admin -run AccountMonitor -count=1
```

Expected: PASS.

- [ ] **Step 3: Run the backend package suite**

```bash
cd upstream/sub2api/backend
go test ./internal/service ./internal/handler/admin -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit the repair**

```bash
git add docs/superpowers/specs/2026-07-29-account-monitor-freshness-recovery-design.md docs/superpowers/plans/2026-07-29-account-monitor-freshness-recovery.md upstream/sub2api/backend/internal/service/account_monitor_service.go upstream/sub2api/backend/internal/service/account_monitor_service_test.go
git commit -m "fix: recover stuck account monitor refreshes"
```

