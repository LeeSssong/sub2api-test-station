# T38 Schedulable Native Probe Score Retention Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Keep selected-window native probe scores and rankings for snapshot-time schedulable accounts when their current probe status is unavailable or stale, while preserving truthful current status and all existing scoring mathematics.

**Architecture:** Keep `projectAccountMonitorProbe` as the current-state projection, then add one pure window-score eligibility boundary used by both global and group window projections. That boundary preserves T32's already-eligible/capped paused-account behavior, and additionally restores scoring only when `Account.IsSchedulable()` is true and the selected-window native probe aggregate has both samples and at least one successful sample. The same selected-window evidence is normalized to `monitor_probe` for scoring without changing current `availability_status` or crossing range boundaries.

**Tech Stack:** Go 1.x service/handler tests, Gin/httptest JSON contracts, Vue 3/Vitest component contracts, PostgreSQL-backed repository interfaces already present in Sub2API.

**Spec:** `docs/superpowers/specs/2026-08-20-t38-schedulable-native-probe-score-retention-design.md`

## Global Constraints

- Baseline is `main@b5ad0cdd624e3590bd0d19000c0f78cde200ef68`; task branch is `codex/t38-retain-native-probe-score`.
- “Schedulable” means snapshot-time `Account.IsSchedulable() == true`, not only persisted `accounts.schedulable=true`.
- T38 retention requires selected-window `SampleCount > 0` and `SuccessSampleCount > 0`.
- Scores never fall back across `24h|7d|30d`; each selected range stands alone.
- Pure-failure and no-sample windows remain unscored and unranked.
- Current `availability_status`, `stale`, latest result, and checked time remain truthful even when a score exists.
- Preserve the existing formula, weights, abnormal cap, cost eligibility, sorting tie-break, T32 paused-account behavior, probe runner, scheduler, and Monitor V2 v7 contract.
- Quality scoring continues to read only `account_monitor_results`; real business request aggregates never supply quality evidence.
- No database migration, configuration change, historical backfill, production data write, API route change, or GitHub Actions workflow.
- This task does not touch `docs/project/project-progress.md`, `docs/project/native-sub-task-package-queue.md`, root `main`, release evidence, or production.

## File Structure

- Modify `upstream/sub2api/backend/internal/service/account_monitor_service.go`: add the pure selected-window score eligibility/normalization boundary and use it in global and group scoring projections.
- Modify `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`: add RED/GREEN coverage for unavailable, stale, pure-failure, no-sample, native schedulability, T32 paused compatibility, global ranking, and group cost eligibility.
- Modify `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`: lock the JSON combination `unavailable|stale + quality_score + group_rank` and the no-score cases.
- Modify `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts` only if the existing component contract does not already render a non-empty score beside unavailable/stale state; production component code changes are conditional on a demonstrated RED.
- Create `docs/superpowers/reports/2026-08-20-t38-schedulable-native-probe-score-retention-verification.md`: record RED/GREEN commands, final focused verification, scope, and remaining root-owned checks.
- Create `docs/handoffs/2026-08-20-t38-schedulable-native-probe-score-retention-handoff.md`: provide the final READY_FOR_ROOT_REVIEW contract and candidate identity.

---

### Task 1: Lock the score-eligibility boundary with RED service tests

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`

**Interfaces:**
- Consumes: existing `Account`, `AccountMonitorQualityEvidence`, `accountMonitorScoreEligible`, `accountMonitorScoreCapped`, and `accountMonitorScoreIneligible`.
- Produces for Task 2: expected pure helper `accountMonitorWindowScoreProjection(account Account, currentScoreStatus string, evidence AccountMonitorQualityEvidence) (AccountMonitorQualityEvidence, string, bool)`.

- [ ] **Step 1: Add a table-driven RED test for the pure boundary**

Add a focused test near the existing projection tests:

```go
func TestAccountMonitorWindowScoreProjectionSeparatesCurrentStateFromScoreEligibility(t *testing.T) {
	now := time.Now().UTC()
	valid := AccountMonitorQualityEvidence{
		Source: "stale", SampleCount: 24, SuccessSampleCount: 21,
		SuccessRate: 21.0 / 24.0, ObservedAt: now.Add(-20 * time.Minute),
	}
	tests := []struct {
		name       string
		account    Account
		status     string
		evidence   AccountMonitorQualityEvidence
		wantSource string
		wantStatus string
		want       bool
	}{
		{name: "schedulable unavailable retains selected-window score", account: Account{Status: StatusActive, Schedulable: true}, status: accountMonitorScoreIneligible, evidence: valid, wantSource: "monitor_probe", wantStatus: accountMonitorScoreEligible, want: true},
		{name: "schedulable stale retains selected-window score", account: Account{Status: StatusActive, Schedulable: true}, status: accountMonitorScoreIneligible, evidence: valid, wantSource: "monitor_probe", wantStatus: accountMonitorScoreEligible, want: true},
		{name: "pure failure stays unscored", account: Account{Status: StatusActive, Schedulable: true}, status: accountMonitorScoreIneligible, evidence: AccountMonitorQualityEvidence{Source: "monitor_probe", SampleCount: 24}, wantSource: "monitor_probe", wantStatus: accountMonitorScoreIneligible},
		{name: "no sample stays unscored", account: Account{Status: StatusActive, Schedulable: true}, status: accountMonitorScoreIneligible, evidence: AccountMonitorQualityEvidence{Source: "stale"}, wantSource: "stale", wantStatus: accountMonitorScoreIneligible},
		{name: "future cooldown uses native schedulability", account: Account{Status: StatusActive, Schedulable: true, TempUnschedulableUntil: timePtr(now.Add(time.Hour))}, status: accountMonitorScoreIneligible, evidence: valid, wantSource: "stale", wantStatus: accountMonitorScoreIneligible},
		{name: "t32 paused eligible remains eligible", account: Account{Status: StatusActive, Schedulable: false}, status: accountMonitorScoreEligible, evidence: valid, wantSource: "monitor_probe", wantStatus: accountMonitorScoreEligible, want: true},
		{name: "t32 abnormal cap remains capped", account: Account{Status: StatusActive, Schedulable: false}, status: accountMonitorScoreCapped, evidence: valid, wantSource: "monitor_probe", wantStatus: accountMonitorScoreCapped, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEvidence, gotStatus, gotEligible := accountMonitorWindowScoreProjection(tt.account, tt.status, tt.evidence)
			if gotEvidence.Source != tt.wantSource || gotStatus != tt.wantStatus || gotEligible != tt.want {
				t.Fatalf("source=%q status=%q eligible=%v", gotEvidence.Source, gotStatus, gotEligible)
			}
		})
	}
}
```

- [ ] **Step 2: Run the pure-boundary test and capture RED**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run '^TestAccountMonitorWindowScoreProjectionSeparatesCurrentStateFromScoreEligibility$' -count=1 -v
```

Expected: build failure naming undefined `accountMonitorWindowScoreProjection`. Record the command and exact failure in the verification report after Task 4 creates it.

- [ ] **Step 3: Add a RED ListWindow matrix for global and group behavior**

Create one test with these accounts in a single group with a valid confirmed multiplier:

```go
accounts := []Account{
	{ID: 501, Name: "latest-unavailable", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate},
	{ID: 502, Name: "stale-current", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate},
	{ID: 503, Name: "pure-failure", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate},
	{ID: 504, Name: "no-sample", Status: StatusActive, Schedulable: true, GroupIDs: []int64{7}, RateMultiplier: &rate},
	{ID: 505, Name: "cooling", Status: StatusActive, Schedulable: true, TempUnschedulableUntil: timePtr(now.Add(time.Hour)), GroupIDs: []int64{7}, RateMultiplier: &rate},
}
```

Use selected-window probe aggregates:

```go
aggregates := map[int64]AccountMonitorAggregate{
	501: {SampleCount: 24, SuccessCount: 21, SuccessSampleCount: 21, ErrorCount: 3, SuccessRate: 21.0 / 24.0, LastCheckedAt: timePtr(now)},
	502: {SampleCount: 24, SuccessCount: 24, SuccessSampleCount: 24, SuccessRate: 1, LastCheckedAt: timePtr(now.Add(-20 * time.Minute))},
	503: {SampleCount: 24, ErrorCount: 24, SuccessRate: 0, LastCheckedAt: timePtr(now)},
}
```

Use latest/timeline facts so account 501 is `unavailable`, 502 is `stale`, 503 is `unavailable`, and 504 has no result. Assert for both `page.Accounts` and `page.Groups[0].Accounts`:

```go
if row := global[501]; row.AvailabilityStatus != accountMonitorAvailabilityUnavailable || row.QualityScore == nil || row.GroupRank == nil || !row.Eligible {
	t.Fatalf("latest unavailable row = %#v", row)
}
if row := global[502]; row.AvailabilityStatus != accountMonitorAvailabilityStale || row.QualityScore == nil || row.GroupRank == nil || !row.Eligible {
	t.Fatalf("stale row = %#v", row)
}
for _, id := range []int64{503, 504, 505} {
	if row := global[id]; row.QualityScore != nil || row.GroupRank != nil || row.Eligible {
		t.Fatalf("account %d unexpectedly scored: %#v", id, row)
	}
}
```

Also assert accounts 501/502 keep `EvidenceSource == "monitor_probe"`, while 503/504 keep their existing non-score source.

- [ ] **Step 4: Run the ListWindow matrix and capture RED**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run '^TestAccountMonitorListWindowRetainsSchedulableUnavailableAndStaleNativeScores$' -count=1 -v
```

Expected: account 501 and/or 502 has nil score/rank under the current projection.

- [ ] **Step 5: Commit RED tests only**

```bash
git add upstream/sub2api/backend/internal/service/account_monitor_service_test.go
git commit -m "test: reproduce t38 probe score retention"
```

---

### Task 2: Implement the minimal shared window-score projection

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go:550-675`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go:1477-1498`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`

**Interfaces:**
- Consumes: `accountMonitorWindowEvidence`, `Account.IsSchedulable()`, existing score status constants, `accountMonitorWindowScoreBreakdown`, and group cost eligibility.
- Produces: `accountMonitorWindowScoreProjection(account Account, currentScoreStatus string, evidence AccountMonitorQualityEvidence) (AccountMonitorQualityEvidence, string, bool)` used identically by global and group projections.

- [ ] **Step 1: Add the pure normalization/eligibility helper**

Place the helper immediately after `accountMonitorWindowEvidence`:

```go
func accountMonitorWindowScoreProjection(
	account Account,
	currentScoreStatus string,
	evidence AccountMonitorQualityEvidence,
) (AccountMonitorQualityEvidence, string, bool) {
	if evidence.SampleCount <= 0 || evidence.SuccessSampleCount <= 0 {
		return evidence, accountMonitorScoreIneligible, false
	}
	if currentScoreStatus == accountMonitorScoreEligible || currentScoreStatus == accountMonitorScoreCapped {
		evidence.Source = "monitor_probe"
		return evidence, currentScoreStatus, true
	}
	if !account.IsSchedulable() {
		return evidence, accountMonitorScoreIneligible, false
	}
	evidence.Source = "monitor_probe"
	return evidence, accountMonitorScoreEligible, true
}
```

Rationale locked by the spec:

- the first branch blocks pure failure and no-sample scores;
- the second branch preserves T32 paused eligible/capped behavior;
- the third branch uses the full native schedulability method for the new unavailable/stale retention path;
- only scoring evidence source is normalized; availability remains untouched.

- [ ] **Step 2: Apply the helper in global window projection**

In `projectGlobalWindowQuality`, after `projectAccountMonitorProbe`, replace the direct `ScoreStatus` eligibility assignment with:

```go
scoreEvidence, scoreStatus, scoreEligible := accountMonitorWindowScoreProjection(account, row.ScoreStatus, evidence)
row.ScoreStatus = scoreStatus
row.Eligible = scoreEligible
if row.Eligible {
	row.EvidenceSource = scoreEvidence.Source
	breakdown, score := accountMonitorWindowScoreBreakdown(1, row.EffectiveMultiplier, weights, scoreEvidence)
	row.ScoreBreakdown = &breakdown
	row.QualityScore = score
	capAccountMonitorAbnormalScore(row)
}
```

Keep `AvailabilityStatus`, `ServiceState`, `MonitorBucket`, stable score sort, account-ID tie-break, and continuous rank loop as they are.

- [ ] **Step 3: Apply the same helper in group window projection**

In `projectGroupWindowQuality`, after `projectAccountMonitorProbe` and after current group cost calculation, use:

```go
scoreEvidence, scoreStatus, scoreEligible := accountMonitorWindowScoreProjection(account, row.ScoreStatus, evidence)
row.ScoreStatus = scoreStatus
row.Eligible = scoreEligible && row.GroupEligibility == accountMonitorEligibilityEligible
if row.Eligible {
	row.Evidence = scoreEvidence
	row.EvidenceSource = scoreEvidence.Source
	breakdown, score := accountMonitorWindowScoreBreakdown(group.RateMultiplier, cost.EffectiveMultiplier, group.ScoreWeights, scoreEvidence)
	row.ScoreBreakdown = &breakdown
	row.QualityScore = score
	capAccountMonitorAbnormalScore(&row.AccountMonitorAccount)
}
```

Do not loosen `multiplier_pending` or `cost_ineligible` behavior.

- [ ] **Step 4: Run the RED tests and confirm GREEN**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMonitorWindowScoreProjectionSeparatesCurrentStateFromScoreEligibility|TestAccountMonitorListWindowRetainsSchedulableUnavailableAndStaleNativeScores' -count=1 -v
```

Expected: PASS; unavailable/stale accounts 501/502 are scored/ranked, while 503/504/505 remain unscored.

- [ ] **Step 5: Run the directly adjacent T32 and scoring regressions**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMonitorProbeProjectionUsesOnlyFreshProbeEvidence|TestAccountMonitorPausedProbeProjectionScoresRanksAndKeepsNoEvidencePending|TestAccountMonitorWindowScoreBreakdownSumsToRoundedQualityScore|TestCalculateAccountMonitorQualityScore' -count=1 -v
```

Expected: PASS. In particular, paused successful/capped rows remain scored and paused HTTP-error/no-evidence rows retain the T32 result.

- [ ] **Step 6: Format and commit the service implementation**

```bash
gofmt -w upstream/sub2api/backend/internal/service/account_monitor_service.go upstream/sub2api/backend/internal/service/account_monitor_service_test.go
git add upstream/sub2api/backend/internal/service/account_monitor_service.go upstream/sub2api/backend/internal/service/account_monitor_service_test.go
git commit -m "fix: retain schedulable native probe scores"
```

---

### Task 3: Lock API and UI field-combination compatibility

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`
- Test: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`
- Test/conditional modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- Conditional modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`

**Interfaces:**
- Consumes: unchanged account-monitor JSON fields `availability_status`, `score_status`, `quality_score`, `group_rank`, `latest_status`, and `stale`.
- Produces: a handler contract that permits truthful unavailable/stale status with a score/rank, plus a component contract that renders both dimensions independently.

- [ ] **Step 1: Add a handler RED contract for unavailable with retained score**

Extend the handler fixture to provide a schedulable account with 24 probe samples, 21 successful samples, three trailing failures, and valid cost evidence. Decode the response and assert:

```go
row := payload.Data.Accounts[0]
if row.AvailabilityStatus != "unavailable" || row.ScoreStatus != "eligible" || row.QualityScore == nil || row.GroupRank == nil {
	t.Fatalf("unexpected retained-score payload: %s", res.Body.String())
}
```

Add `AvailabilityStatus`, `LatestStatus`, and `Stale` to the local decoded struct. Add a second schedulable stale fixture with successful selected-window samples and assert `availability_status="stale"`, `stale=true`, and non-nil score/rank.

- [ ] **Step 2: Run the handler test before any handler production edit**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/handler/admin -run '^TestAccountMonitorHandlerReturnsUnavailableAndStaleRowsWithRetainedNativeScores$' -count=1 -v
```

Expected after Task 2: PASS without production handler changes, proving the existing mapper transports the new field combination. If it fails because a mapper clears fields, apply only the narrow mapper correction demonstrated by the failure and rerun.

- [ ] **Step 3: Add a frontend component contract**

In `AccountMonitorCard.spec.ts`, mount an account with:

```ts
{
  availability_status: 'unavailable',
  service_state: 'unavailable',
  stale: false,
  score_status: 'eligible',
  quality_score: 82,
  group_rank: 3,
  eligible: true,
}
```

Assert the card simultaneously contains the unavailable status label, score `82`, and rank `第 3`. Repeat with `availability_status: 'stale'`, `stale: true`, and a non-empty score/rank; assert the status label is `待确认` while score/rank remain visible.

- [ ] **Step 4: Run the frontend contract and constrain any production edit to the demonstrated failure**

Run:

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts
```

Expected: PASS with the existing component because status and score already use separate computed properties. If RED shows a hard coupling, edit only `AccountMonitorCard.vue` so `scoreEligible` continues to derive from `score_status` and not `availability_status`, then rerun to PASS.

- [ ] **Step 5: Commit API/UI compatibility tests and any proven narrow fix**

```bash
git add upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go \
  upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts \
  upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue
git commit -m "test: lock t38 score status compatibility"
```

Before committing, omit `AccountMonitorCard.vue` from `git add` when no production frontend edit occurred.

---

### Task 4: Run direct verification and write task evidence

**Files:**
- Create: `docs/superpowers/reports/2026-08-20-t38-schedulable-native-probe-score-retention-verification.md`
- Create: `docs/handoffs/2026-08-20-t38-schedulable-native-probe-score-retention-handoff.md`
- Verify: all T38-modified files

**Interfaces:**
- Consumes: final candidate commits and direct test results from Tasks 1–3.
- Produces: committed evidence and a `READY_FOR_ROOT_REVIEW` handoff; it does not merge, push, deploy, or update global ledgers.

- [ ] **Step 1: Run the complete direct backend matrix**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'AccountMonitor.*(WindowScore|ListWindow|ProbeProjection|PausedProbeProjection|QualityScore|ScoreBreakdown)' -count=1
go test ./internal/handler/admin -run 'AccountMonitor.*(CompleteWindow|UnavailableAndStaleRows|List)' -count=1
go test ./internal/service ./internal/handler/admin -run '^$'
```

Expected: all commands PASS. The compile-only command covers the affected packages without broadening into unrelated runtime tests.

- [ ] **Step 2: Run the direct frontend contract and type check only if frontend files changed**

Always run the focused component test:

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts
```

If either frontend production code or TypeScript test fixtures changed in a way checked by project types, also run:

```bash
pnpm typecheck
```

Expected: PASS.

- [ ] **Step 3: Run formatting, scope, and migration/config guards**

From repository root:

```bash
gofmt -w upstream/sub2api/backend/internal/service/account_monitor_service.go \
  upstream/sub2api/backend/internal/service/account_monitor_service_test.go \
  upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go
git diff --check b5ad0cdd624e3590bd0d19000c0f78cde200ef68 HEAD --
git diff --name-only b5ad0cdd624e3590bd0d19000c0f78cde200ef68 HEAD -- | \
  grep -E '(^|/)(migrations|\.github/workflows|docs/project/project-progress\.md|docs/project/native-sub-task-package-queue\.md)($|/)' && exit 1 || true
```

Expected: formatting and diff check have no output; protected path scan has no matches.

- [ ] **Step 4: Write the verification report with exact evidence**

Record:

```markdown
# T38 Direct Verification

- Baseline: main@b5ad0cdd624e3590bd0d19000c0f78cde200ef68
- Candidate: record the exact SHA printed by `git rev-parse HEAD` at execution time
- RED: pure helper undefined; ListWindow unavailable/stale rows unscored
- GREEN: list each exact command, exit code, and test count
- Scope: service projection, direct handler contract, component contract if changed
- Migrations: none
- Configuration: none
- Production data writes: none
- Expected downtime: false; root preflight remains authoritative
- Unverified: root-main integration, release preflight, deployment, production login-state sample
```

Replace the angle-bracketed execution-time identity with the actual SHA before committing; do not retain template markers.

- [ ] **Step 5: Write the final handoff**

The handoff must include:

```markdown
# T38 READY_FOR_ROOT_REVIEW Handoff

- Task: T38
- Baseline main SHA and tree
- Candidate tip and tree
- Commit list
- Changed files
- Direct tests and results
- Unverified items
- Migration/config/production-write status
- downtime_required=false expectation, pending root preflight
- Rollback: revert T38 commits and redeploy through the reviewed root chain
- Remaining risks: stale score age interpretation; ranking is not scheduler order
- State: READY_FOR_ROOT_REVIEW
```

Use actual identities and complete lists.

- [ ] **Step 6: Commit evidence and handoff**

```bash
git add docs/superpowers/reports/2026-08-20-t38-schedulable-native-probe-score-retention-verification.md \
  docs/handoffs/2026-08-20-t38-schedulable-native-probe-score-retention-handoff.md
git commit -m "docs: hand off t38 probe score retention"
```

- [ ] **Step 7: Perform final candidate identity checks**

```bash
git status --short --branch
git rev-parse HEAD
git rev-parse HEAD^{tree}
git log --oneline b5ad0cdd624e3590bd0d19000c0f78cde200ef68..HEAD
git diff --check b5ad0cdd624e3590bd0d19000c0f78cde200ef68 HEAD --
```

Expected: clean worktree, candidate commits only, no diff-check output. Report `READY_FOR_ROOT_REVIEW`; leave merge, push, release evidence, deployment, and online verification to the unique root controller.

## Plan Self-Review

- Spec coverage: current status, selected-window evidence, native schedulability, pure-failure/no-sample exclusion, no cross-window fallback, group cost eligibility, T32 compatibility, API field combinations, testing, rollback, and root-owned release checks each map to an explicit task.
- Placeholder scan: execution-time SHA markers are confined to documentation-writing instructions and explicitly require replacement before commit; the finished plan contains no deferred implementation decision.
- Type consistency: the helper signature is identical in Task 1 and Task 2; all field names match `AccountMonitorQualityEvidence` and `AccountMonitorAccount`; global and group call sites consume the same helper result.
- Scope: the plan has one production-code unit in the existing service, with handler/frontend changes driven only by demonstrated contract failures.

