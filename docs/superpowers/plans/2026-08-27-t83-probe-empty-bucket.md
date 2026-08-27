# T83 主动探测空桶准入与模型检测降载 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让三条自动主动探测仅在当前北京时间 5 分钟桶没有对应真实用户请求时发送，并消除失败重试与模型检测升级 burst。

**Architecture:** 复用 `usage_logs` 作为唯一真实请求事实源，通过窄的、可选的 usage-window reader 查询 account/group 当前桶。账号与渠道 runner 继续使用现有 singleton/in-flight 结构；模型检测继续使用既有 slot-key/claim。自动渠道路径拆出单模型检查，手动路径不变；v4.1.1 的受许可检测器仅降低调度档位和 worker，不改变其内部 prompt/hash 基线。

**Tech Stack:** Go、PostgreSQL、现有 Sub2API service/repository、Python 3 adapter、Testify、sqlmock。

**Spec:** `docs/superpowers/specs/2026-08-27-t83-probe-empty-bucket-design.md`

## Global Constraints

- 时间桶固定为 `Asia/Shanghai` 的 5 分钟自然桶。
- `usage_logs` 是唯一真实用户请求事实源；主动 probe 不写入该表。
- 自动路径 fail closed；usage 查询异常时不得访问上游。
- 自动失败不重试；手动检查维持既有合同。
- 不迁移、不回填、不改账号/倍率/计费/用户路由，不使用 GitHub Actions。
- 不修改 v4.1.1 detector 核心、基线或 prompt/hash 合同；不得记录 key、题面或完整响应。

---

### Task 1: Current-bucket usage reader and deterministic bucket helper

**Files:**
- Create: `upstream/sub2api/backend/internal/service/active_probe_bucket.go`
- Create: `upstream/sub2api/backend/internal/service/active_probe_bucket_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_usage_service.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_repo_stats.go`
- Create: `upstream/sub2api/backend/internal/repository/usage_log_repo_active_probe_test.go`

**Interfaces:**
- Produces `ActiveProbeUsageWindowReader` with `HasAccountUsageInWindow(ctx, accountID, from, until)` and `HasGroupUsageInWindow(ctx, groupID, from, until)`.
- Produces `currentActiveProbeBucket(now time.Time) (start time.Time, end time.Time)` and `activeProbeBucketKey(now time.Time) string`.
- `AccountUsageService` implements the narrow reader by forwarding only if its underlying repository supports the two methods; otherwise it returns an error so production callers skip.

- [ ] **Step 1: Write the failing bucket test**

```go
func TestCurrentActiveProbeBucketUsesShanghaiFiveMinuteBoundary(t *testing.T) {
	start, end := currentActiveProbeBucket(time.Date(2026, 8, 27, 2, 7, 59, 0, time.UTC))
	require.Equal(t, "2026-08-27T10:05:00+08:00", start.Format(time.RFC3339))
	require.Equal(t, "2026-08-27T10:10:00+08:00", end.Format(time.RFC3339))
}
```

- [ ] **Step 2: Run the test to verify RED**

Run: `go test ./internal/service -run TestCurrentActiveProbeBucketUsesShanghaiFiveMinuteBoundary -count=1`

Expected: FAIL because `currentActiveProbeBucket` does not exist.

- [ ] **Step 3: Implement the minimal bucket helper**

```go
func currentActiveProbeBucket(now time.Time) (time.Time, time.Time) {
	local := now.In(activeProbeLocation)
	start := local.Truncate(5 * time.Minute)
	return start, start.Add(5 * time.Minute)
}
```

- [ ] **Step 4: Run the bucket test to verify GREEN**

Run: `go test ./internal/service -run TestCurrentActiveProbeBucketUsesShanghaiFiveMinuteBoundary -count=1`

Expected: PASS.

- [ ] **Step 5: Write failing repository tests for bounded account/group existence queries**

```go
func TestUsageLogRepositoryHasAccountUsageInWindowUsesExclusiveEnd(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(`SELECT EXISTS`).WithArgs(int64(7), from, until).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	got, err := newUsageLogRepositoryWithSQL(nil, db).HasAccountUsageInWindow(context.Background(), 7, from, until)
	require.NoError(t, err)
	require.True(t, got)
}
```

- [ ] **Step 6: Run the repository tests to verify RED**

Run: `go test ./internal/repository -run 'TestUsageLogRepositoryHas(Account|Group)UsageInWindow' -count=1`

Expected: FAIL because the repository methods do not exist.

- [ ] **Step 7: Implement bounded `SELECT EXISTS` methods and AccountUsageService forwarding**

```go
const accountUsageExistsQuery = `SELECT EXISTS(
  SELECT 1 FROM usage_logs
  WHERE account_id = $1 AND created_at >= $2 AND created_at < $3
)`

func (s *AccountUsageService) HasAccountUsageInWindow(ctx context.Context, id int64, from, until time.Time) (bool, error) {
	reader, ok := s.usageLogRepo.(ActiveProbeUsageWindowReader)
	if !ok { return false, errors.New("active probe usage reader unavailable") }
	return reader.HasAccountUsageInWindow(ctx, id, from, until)
}
```

Implement the group method with identical half-open bounds and `group_id = $1`. Validate IDs and `until.After(from)` before querying.

- [ ] **Step 8: Run the focused service/repository tests to verify GREEN**

Run: `go test ./internal/service ./internal/repository -run 'TestCurrentActiveProbeBucket|TestUsageLogRepositoryHas(Account|Group)UsageInWindow' -count=1`

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add upstream/sub2api/backend/internal/service/active_probe_bucket.go \
  upstream/sub2api/backend/internal/service/active_probe_bucket_test.go \
  upstream/sub2api/backend/internal/service/account_usage_service.go \
  upstream/sub2api/backend/internal/repository/usage_log_repo_stats.go \
  upstream/sub2api/backend/internal/repository/usage_log_repo_active_probe_test.go
git commit -m "feat: add active probe usage window gate"
```

### Task 2: Gate automatic account monitoring and randomize its probe prompt

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_probe.go`
- Create: `upstream/sub2api/backend/internal/service/account_monitor_probe_test.go`

**Interfaces:**
- `listPool` returns only active, schedulable accounts for automatic runs.
- `RunAll` calls `HasAccountUsageInWindow` before invoking `probeAccount`; a true result or reader error means no probe result insertion.
- `accountMonitorProbePrompt()` returns a short string containing a UUID nonce and no account credential.

- [ ] **Step 1: Write failing account-run tests**

```go
func TestAccountMonitorRunAllSkipsAccountWithCurrentBucketUsage(t *testing.T) {
	usage := &activeProbeUsageStub{accountUsed: map[int64]bool{7: true}}
	service := newAccountMonitorServiceForTest(accounts, usage)
	service.probeConnection = func(context.Context, int64, string, string, string) (AccountMonitorProbeResult, error) {
		t.Fatal("probe must not be called when current bucket has user usage")
		return AccountMonitorProbeResult{}, nil
	}
	completed, err := service.RunAll(context.Background(), 1)
	require.NoError(t, err)
	require.Zero(t, completed)
}

func TestAccountMonitorProbePromptIsUnique(t *testing.T) {
	require.NotEqual(t, accountMonitorProbePrompt(), accountMonitorProbePrompt())
}
```

- [ ] **Step 2: Run the account tests to verify RED**

Run: `go test ./internal/service -run 'TestAccountMonitorRunAllSkipsAccountWithCurrentBucketUsage|TestAccountMonitorProbePromptIsUnique' -count=1`

Expected: FAIL because automatic monitoring still probes and the prompt helper does not exist.

- [ ] **Step 3: Implement the minimal automatic gate**

```go
start, end := currentActiveProbeBucket(time.Now())
used, err := s.usage.HasAccountUsageInWindow(gctx, account.ID, start, end)
if err != nil || used {
	return nil
}
result := s.probeAccount(gctx, account)
```

Generate prompts with `uuid.NewString()` and pass the generated value rather than the literal `"hi"`. Keep `RunOne` (manual refresh) outside this automatic gate.

- [ ] **Step 4: Run the focused account tests to verify GREEN**

Run: `go test ./internal/service -run 'TestAccountMonitor(RunAll|ProbePrompt|Service)' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add upstream/sub2api/backend/internal/service/account_monitor_service.go \
  upstream/sub2api/backend/internal/service/account_monitor_service_test.go \
  upstream/sub2api/backend/internal/service/account_monitor_probe.go \
  upstream/sub2api/backend/internal/service/account_monitor_probe_test.go
git commit -m "feat: gate automatic account monitor probes"
```

### Task 3: Split scheduled channel checks from manual checks and remove automatic retry

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/channel_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/channel_monitor_runner.go`
- Modify: `upstream/sub2api/backend/internal/service/channel_monitor_challenge.go`
- Modify: `upstream/sub2api/backend/internal/service/channel_monitor_runner_test.go`
- Modify: `upstream/sub2api/backend/internal/service/channel_monitor_retry_test.go`
- Create: `upstream/sub2api/backend/internal/service/channel_monitor_scheduled_test.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`

**Interfaces:**
- `RunCheck` remains the manual all-model endpoint.
- `RunScheduledCheck(ctx, id)` returns at most one `CheckResult` and never calls retry helper.
- `ChannelMonitorService.SetActiveProbeUsageReader(reader ActiveProbeUsageWindowReader)` is wired with `AccountUsageService`.
- `monitorRunnerSvc` exposes `RunScheduledCheck`; runner calls it only for scheduled fires.

- [ ] **Step 1: Write failing scheduled-channel tests**

```go
func TestRunScheduledCheckSkipsWhenGroupHasCurrentBucketUsage(t *testing.T) {
	svc := newChannelMonitorServiceForTest(monitorWithGroup(9))
	svc.SetActiveProbeUsageReader(&activeProbeUsageStub{groupUsed: map[int64]bool{9: true}})
	_, err := svc.RunScheduledCheck(context.Background(), 1)
	require.ErrorIs(t, err, ErrChannelMonitorProbeSkipped)
}

func TestRunScheduledCheckUsesOneModelWithoutRetry(t *testing.T) {
	// Test HTTP server returns 500 and asserts one received request.
	results, err := svc.RunScheduledCheck(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, 1, server.RequestCount())
}
```

- [ ] **Step 2: Run the scheduled-channel tests to verify RED**

Run: `go test ./internal/service -run 'TestRunScheduledCheck' -count=1`

Expected: FAIL because `RunScheduledCheck` and its skip behavior do not exist.

- [ ] **Step 3: Implement scheduled single-model selection and fail-closed gate**

```go
func scheduledMonitorModel(m *ChannelMonitor, bucket time.Time) string {
	models := append([]string{m.PrimaryModel}, m.ExtraModels...)
	return models[int((m.ID+bucket.Unix()/300)%int64(len(models)))]
}

func (s *ChannelMonitorService) RunScheduledCheck(ctx context.Context, id int64) ([]*CheckResult, error) {
	// load runtime and monitor; require GroupID and usage reader
	// skip when current bucket has group usage
	// call runCheckForModel once, persist that one result
}
```

Use a stable skip error internal to runner logging, do not create history rows for skips, and do not invoke `runChannelMonitorCheckWithRetry` on the automatic path. Update `runOne` to use the new method while preserving manual handler behavior.

- [ ] **Step 4: Write a failing nonce test for channel challenges**

```go
func TestGenerateChallengeIncludesUniqueNonce(t *testing.T) {
	first, second := generateChallenge(), generateChallenge()
	require.NotEqual(t, first.Prompt, second.Prompt)
	require.True(t, validateChallenge(first.Expected, first.Expected))
}
```

- [ ] **Step 5: Run the nonce test to verify RED**

Run: `go test ./internal/service -run TestGenerateChallengeIncludesUniqueNonce -count=1`

Expected: FAIL because challenges can repeat without a nonce.

- [ ] **Step 6: Add nonce without changing answer validation**

```go
nonce := uuid.NewString()
prompt := fmt.Sprintf(monitorChallengePromptTemplate, a, op, b) + "\nRequest nonce: " + nonce
```

Keep the nonce out of `CheckResult.Message` and retain arithmetic-only validation.

- [ ] **Step 7: Run the focused channel tests to verify GREEN**

Run: `go test ./internal/service -run 'TestRunScheduledCheck|TestGenerateChallengeIncludesUniqueNonce|TestChannelMonitorRunner|TestChannelMonitorRetry' -count=1`

Expected: PASS, including existing manual retry tests unchanged.

- [ ] **Step 8: Commit**

```bash
git add upstream/sub2api/backend/internal/service/channel_monitor_service.go \
  upstream/sub2api/backend/internal/service/channel_monitor_runner.go \
  upstream/sub2api/backend/internal/service/channel_monitor_challenge.go \
  upstream/sub2api/backend/internal/service/channel_monitor_runner_test.go \
  upstream/sub2api/backend/internal/service/channel_monitor_retry_test.go \
  upstream/sub2api/backend/internal/service/channel_monitor_scheduled_test.go \
  upstream/sub2api/backend/internal/service/wire.go
git commit -m "feat: gate scheduled channel monitor probes"
```

### Task 4: Limit scheduled model detection to empty 6-hour slots without escalation

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_model_detection.go`
- Modify: `upstream/sub2api/backend/internal/service/account_model_detection_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_model_detection_v411_test.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Modify: `upstream/sub2api/deploy/model-detector-v411-adapter.py`
- Modify: `upstream/sub2api/deploy/model-detector-v411-adapter_test.py`

**Interfaces:**
- `AccountModelDetectionService.SetActiveProbeUsageReader(reader ActiveProbeUsageWindowReader)` supplies the existing `AccountUsageService` through wire.
- `dueDetectionSlot` yields only `00:00`, `06:00`, `12:00`, `18:00` local slots.
- Scheduled runs use `low`; `execute` never schedules a high escalation for scheduled runs.

- [ ] **Step 1: Write failing model-detection scheduling tests**

```go
func TestDueDetectionSlotUsesSixHourCadence(t *testing.T) {
	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	require.Equal(t, "2026-08-27T06:00", dueDetectionSlot(time.Date(2026, 8, 27, 6, 12, 0, 0, loc)))
	require.Empty(t, dueDetectionSlot(time.Date(2026, 8, 27, 7, 0, 0, 0, loc)))
}

func TestRunDueSlotsSkipsUsedOrUnschedulableAccounts(t *testing.T) {
	// Account 7 has bucket usage, 8 is unschedulable, 9 is active/schedulable.
	queued, err := svc.RunDueSlots(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, queued)
	require.Equal(t, AccountModelDetectionProfileLow, repo.enqueued[0].Profile)
}
```

- [ ] **Step 2: Run the model-detection tests to verify RED**

Run: `go test ./internal/service -run 'TestDueDetectionSlotUsesSixHourCadence|TestRunDueSlotsSkipsUsedOrUnschedulableAccounts' -count=1`

Expected: FAIL because old slots include 10/15/18/21 and all API Key accounts are eligible.

- [ ] **Step 3: Implement 6-hour empty-bucket scheduled runs**

```go
if account.Type != AccountTypeAPIKey || account.Status != StatusActive || !account.Schedulable {
	continue
}
used, err := s.usage.HasAccountUsageInWindow(ctx, account.ID, start, end)
if err != nil || used { continue }
run := newAccountModelDetectionRun(account.ID, model, AccountModelDetectionProfileLow, AccountModelDetectionModeMonitor, AccountModelDetectionTriggerScheduled)
```

Keep slot-key enqueue deduplication. Reader absence or error skips (fail closed).

- [ ] **Step 4: Write a failing no-escalation test**

```go
func TestScheduledLowDetectionDoesNotEscalateAfterInsufficientResult(t *testing.T) {
	service.execute(context.Background(), queuedRun.ID)
	require.Empty(t, repo.highEscalationRuns())
}
```

- [ ] **Step 5: Run the no-escalation test to verify RED**

Run: `go test ./internal/service -run TestScheduledLowDetectionDoesNotEscalateAfterInsufficientResult -count=1`

Expected: FAIL because `execute` enqueues high after the current result.

- [ ] **Step 6: Remove scheduled escalation and force one licensed adapter worker**

```go
// execute: do not call EnqueueEscalationHigh for any scheduled monitor run.
if run.TriggerKind != "scheduled" || run.Mode != AccountModelDetectionModeMonitor { return }
```

Delete the actual escalation call rather than merely suppressing its error. In the Python adapter set `config["workers"] = 1`; retain profile validation, response bounding and all vendor files unchanged.

- [ ] **Step 7: Run Go and Python focused tests to verify GREEN**

Run: `go test ./internal/service -run 'TestDueDetectionSlot|TestRunDueSlots|TestScheduledLowDetectionDoesNotEscalate|TestAccountModelDetection' -count=1 && python3 upstream/sub2api/deploy/model-detector-v411-adapter_test.py`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add upstream/sub2api/backend/internal/service/account_model_detection.go \
  upstream/sub2api/backend/internal/service/account_model_detection_test.go \
  upstream/sub2api/backend/internal/service/account_model_detection_v411_test.go \
  upstream/sub2api/backend/internal/service/wire.go \
  upstream/sub2api/deploy/model-detector-v411-adapter.py \
  upstream/sub2api/deploy/model-detector-v411-adapter_test.py
git commit -m "feat: reduce scheduled model detection traffic"
```

### Task 5: Integrated verification, handoff, root merge and emergency release

**Files:**
- Create: `docs/handoffs/2026-08-27-t83-probe-empty-bucket-handoff.md`
- Modify: `docs/project/project-progress.md` (root only after merge/deploy)
- Modify: `docs/project/native-sub-task-package-queue.md` (root only after merge/deploy)

**Interfaces:**
- Candidate reports baseline, commit, direct test output, no migration/config changes, expected `downtime_required=false`, rollback via prior blue-green image.

- [ ] **Step 1: Run the complete direct verification set**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service ./internal/repository -run 'ActiveProbe|AccountMonitor|ChannelMonitor|AccountModelDetection' -count=1
go build ./cmd/server
cd ../..
python3 upstream/sub2api/deploy/model-detector-v411-adapter_test.py
gofmt -d upstream/sub2api/backend/internal/service/active_probe_bucket.go \
  upstream/sub2api/backend/internal/service/account_monitor_service.go \
  upstream/sub2api/backend/internal/service/account_monitor_probe.go \
  upstream/sub2api/backend/internal/service/channel_monitor_service.go \
  upstream/sub2api/backend/internal/service/channel_monitor_runner.go \
  upstream/sub2api/backend/internal/service/channel_monitor_challenge.go \
  upstream/sub2api/backend/internal/service/account_model_detection.go
git diff --check
```

Expected: all commands exit 0 and `gofmt -d` has no output.

- [ ] **Step 2: Write handoff with exact evidence and risks**

Record candidate baseline/SHA, changed files, direct test commands/results, zero migration/config change, user’s emergency deployment authorization, rollback source, and the v4.1.1 prompt/hash non-modification boundary.

- [ ] **Step 3: Commit docs and request root merge authorization**

```bash
git add docs/superpowers/specs/2026-08-27-t83-probe-empty-bucket-design.md \
  docs/superpowers/plans/2026-08-27-t83-probe-empty-bucket.md \
  docs/handoffs/2026-08-27-t83-probe-empty-bucket-handoff.md
git commit -m "docs: hand off T83 probe admission changes"
```

- [ ] **Step 4: Root merges candidate, pushes `main`, and runs release preflight**

From clean root `main`, confirm candidate is current, merge it, rerun the direct tests, push `origin/main`, and run the reviewed preflight. If `downtime_required=true`, stop before any switch and request a new explicit stop-the-world authorization.

- [ ] **Step 5: Perform emergency production release and verify**

Because the user explicitly authorized “快速部署到主站”, run the existing blue-green production controller from merged/pushed `main` only when the preflight returns `downtime_required=false`. Verify public `/healthz`, `/readyz`, `/health`, the active source commit/tree, healthy API/worker/detector services, and a read-only account-model-detection history/API check. Do not generate synthetic upstream probes.

- [ ] **Step 6: Synchronize or verify the acceptance station immediately**

Use `ACCEPTANCE_ENV_FILE=/Users/gongtengxinwen/.config/sub2api/acceptance-20260827.env RELEASE_WORKTREE="$PWD" ops/release-sub2api-acceptance.sh` from the merged clean worktree, or record a read-only same commit/tree confirmation if it is already deployed. Verify `https://api.xingqiaolab.top/admin/lab/health` and station service health without printing credentials.

- [ ] **Step 7: Update root records and close only with evidence**

Record production and acceptance release evidence, exact commit/tree, health results, rollback location and station parity in the root progress/queue. Retain candidate worktree and branch until both deployments and verification are confirmed.
