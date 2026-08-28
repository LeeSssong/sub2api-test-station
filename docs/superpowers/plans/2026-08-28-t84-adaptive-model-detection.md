# T84 模型检测跨 6 小时窗口自适应升档 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (or superpowers:subagent-driven-development) to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让自动模型检测以 low 为默认，仅在下一次 6 小时窗口按明确可疑证据逐级升到 medium/high，并始终满足 5 分钟真实请求空桶。

**Architecture:** 复用 `account_model_detection_runs` 历史推导下一 profile，不新增状态表；通过向后兼容 migration `229_active_probe_switches.sql` 持久化账号/分组主动探测开关。调度入队和执行发送前各做一次 `usage_logs` 空桶门禁。自动路径保持每 slot 单 run、失败不立即重试，high 完成后下个窗口回 low。

**Tech Stack:** Go、PostgreSQL、Testify、现有 Sub2API model-detector adapter。

**Spec:** `docs/superpowers/specs/2026-08-28-t84-adaptive-model-detection-design.md`

## Global Constraints

- 时间窗口固定为 Asia/Shanghai 每日 00:00、06:00、12:00、18:00 后 30 分钟。
- 当前 5 分钟真实请求由 `usage_logs` 唯一判定，窗口为半开区间 `[start,end)`。
- 只有明确 suspicious 证据升级；failed/insufficient/usage error 不升级、不立即重试。
- 每次实际上游请求前再次检查空桶；繁忙时不访问上游、不消耗档位。
- 不修改手动检测、计费、路由、账号状态、v4.1.1 prompt/hash；仅新增向后兼容 migration `229_active_probe_switches.sql`，不做生产数据回填。

### Task 1: Add pure adaptive profile policy

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_model_detection.go`
- Modify: `upstream/sub2api/backend/internal/service/account_model_detection_types.go`
- Modify: `upstream/sub2api/backend/internal/service/account_model_detection_scheduled_test.go`

**Interfaces:**
- Produce `nextScheduledDetectionProfile(recent []AccountModelDetectionRun) string`.
- Produce `isSuspiciousDetection(run AccountModelDetectionRun) bool`.
- Add trigger constant `AccountModelDetectionTriggerSuspicious`.

- [ ] Write RED table tests for no history, low suspicious, medium suspicious, high final reset, normal, insufficient, failed, and abnormal-without-evidence.
- [ ] Run `go test ./internal/service -run 'TestNextScheduledDetectionProfile|TestIsSuspiciousDetection' -count=1` and observe failure.
- [ ] Implement pure helpers using only finished runs, treating mismatch/strong_conflict/candidate mismatch as suspicious and preserving current profile for failed/insufficient.
- [ ] Run the focused tests and existing `TestShouldEscalateDetectionUsesTieredReasons`.
- [ ] Commit `feat: add adaptive model detection profile policy`.

### Task 2: Schedule the derived profile only in the next 6-hour slot

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_model_detection.go`
- Modify: `upstream/sub2api/backend/internal/service/account_model_detection_scheduled_test.go`

**Interfaces:**
- `RunDueSlots` uses `nextScheduledDetectionProfile` from recent history.
- Automatic runs remain `ModeMonitor`, `TriggerKind=scheduled`, unique `SlotKey`.

- [ ] Add RED tests proving low suspicious at 06:05 enqueues medium only at 12:05, medium suspicious then high at the next slot, and busy bucket leaves no run.
- [ ] Run focused tests and observe failure.
- [ ] In `RunDueSlots`, load recent runs before enqueue, derive profile, set trigger reason to `suspicious` only for an escalation, and retain profile on failed/insufficient.
- [ ] Keep the existing 6-hour slot and pre-enqueue usage gate unchanged.
- [ ] Run `go test ./internal/service -run 'TestRunDueSlots|TestNextScheduledDetectionProfile' -count=1`.
- [ ] Commit `feat: schedule adaptive detection tiers by six-hour slot`.

### Task 3: Recheck empty bucket immediately before upstream access

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_model_detection.go`
- Modify: `upstream/sub2api/backend/internal/service/account_model_detection_scheduled_test.go`

**Interfaces:**
- Scheduled `execute` performs a second `HasAccountUsageInWindow` check after claim and before `sidecar.Detect`.
- Manual runs bypass this automatic gate.

- [ ] Add RED test where usage reader changes from empty at enqueue to used at execute; assert sidecar call count is zero and no escalation run is enqueued.
- [ ] Run focused test and observe failure.
- [ ] Implement the scheduled-only execution gate; complete the run with `probe_bucket_busy` and a non-escalating failed/insufficient-safe result, without retry or extra enqueue.
- [ ] Run scheduled execution tests and existing manual execution tests.
- [ ] Commit `feat: recheck model detection bucket before send`.

### Task 4: Verify direct behavior and handoff

**Files:**
- Create: `docs/handoffs/2026-08-28-t84-adaptive-model-detection-handoff.md`

- [ ] Run `cd upstream/sub2api/backend && go test ./internal/service ./internal/repository -run 'AccountModelDetection|ActiveProbe' -count=1`.
- [ ] Run `go build ./cmd/server`, `gofmt -d` on changed Go files, Python adapter tests, and `git diff --check`.
- [ ] Record migration `229_active_probe_switches.sql` (no config/data backfill), exact commit, test outputs, remaining risks, and that deployment is not performed by this task.
- [ ] Leave task at `READY_FOR_ROOT_REVIEW`; do not push, merge, or deploy without root release authorization.
