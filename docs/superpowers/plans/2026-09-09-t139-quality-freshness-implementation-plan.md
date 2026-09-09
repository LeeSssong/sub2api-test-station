# T139 Quality Freshness Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make unified OpenAI quality scheduling use a one-hour rolling evidence window with five-minute freshness, while preserving T138 self-owned priority and API Key cold-start behavior and native availability hard gates.

**Architecture:** Keep the existing account-quality repository and in-memory snapshot provider as the single quality fact path. Change its query window and score blend to mutually exclusive `W5` and `W55`, add a non-blocking coalesced refresh trigger after valid real-request persistence, and keep every dispatch on the latest atomically published snapshot. Do not alter T138 resource-tier partitioning, self-owned ordering, API Key cold-start prior, or native schedulability checks.

**Tech Stack:** Go 1.27 project code, `database/sql`, PostgreSQL aggregation, `golang.org/x/sync/singleflight`, existing OpenAI gateway services and tests.

**Spec:** `docs/superpowers/specs/2026-09-09-one-day-quality-window-native-availability-ranking-design.md`

## Global Constraints

- Start from the pushed clean root `main@1ee82471f` and implement only in `.worktrees/t139-quality-freshness`.
- Use Sub native unavailable/schedulable state as the hard candidate gate; quality scores never re-admit a rejected account.
- Preserve T138 self-owned resource tier, API Key cold-start priority prior, `priority=50` neutrality, and maturity handoff exactly.
- Use one-hour rolling range `[now-1h, now)` with mutually exclusive `W5=[now-5m, now)` and `W55=[now-1h, now-5m)`.
- Refresh on a five-minute schedule and after valid real-request persistence through a non-blocking coalesced trigger; dispatch reads the latest published snapshot and never synchronously scans the database per request.
- Keep account-level ranking and retain cooled/unavailable accounts in ranking display; do not create model-level ranking.
- No migration, production data/config mutation, deployment, or GitHub Actions.

## File Map

- Modify `upstream/sub2api/backend/internal/service/openai_account_quality.go`: quality window identifiers, provider refresh API, rolling one-hour query bounds, periodic refresh configuration, coalesced non-blocking refresh trigger, and atomic snapshot publication.
- Modify `upstream/sub2api/backend/internal/repository/usage_log_quality.go`: SQL window labels and comments for the one-hour `W5/W55` scan; keep existing attempt deduplication and attribution filters.
- Modify `upstream/sub2api/backend/internal/service/openai_quality_score.go`: use the new window identifiers, targets, and `40%/60%` `W5/W55` blend without touching T138 resource-tier logic.
- Modify `upstream/sub2api/backend/internal/service/openai_gateway_service.go`: configure the provider for five-minute freshness and expose the narrow refresh notification hook used after real usage persistence.
- Modify `upstream/sub2api/backend/internal/service/openai_gateway_usage.go`: notify the quality provider only after a valid real usage/error fact has been durably accepted, without delaying the request result.
- Modify `upstream/sub2api/backend/internal/service/openai_account_quality_test.go`: provider window, five-minute refresh, stale behavior, and coalescing tests.
- Modify `upstream/sub2api/backend/internal/service/openai_quality_score_test.go` and/or `openai_unified_quality_score_test.go`: `W5/W55` score blend, confidence, and no-sample fallback tests.
- Modify `upstream/sub2api/backend/internal/repository/usage_log_quality_test.go` if an existing repository test seam covers query text/arguments; otherwise add the narrowest existing SQL contract test.
- Modify `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler_test.go` only if a regression test is needed to prove T138 resource-tier behavior remains unchanged with the new window labels.

### Task 1: Lock the New Window and Refresh Contract in Tests

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_account_quality_test.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_quality_score_test.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_unified_quality_score_test.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_quality_test.go` when an existing SQL test seam is available

**Interfaces:**
- Consumes: current `OpenAIAccountQualitySnapshotProvider`, `OpenAIAccountQualityRepository`, `OpenAIQualityWindow`, and score helpers.
- Produces: failing tests that specify `W5/W55`, `[now-1h,now)`, `40%/60%`, five-minute refresh throttling, and coalesced refresh semantics without changing T138 ordering.

- [ ] **Step 1: Write the failing provider window test.** Assert the repository receives `start=end-1h`, `end=now`, and the snapshot reports the same bounds. Assert no seven-day interval is requested.
- [ ] **Step 2: Write the failing score blend test.** Build one account with distinct `W5` and `W55` metric scores and assert the calculated metric uses `W5*0.40 + W55*0.60`, including the existing confidence fallback when one window has no samples.
- [ ] **Step 3: Write the failing refresh-on-real-result test.** Use a repository stub with a blocking/counting query and assert a refresh notification causes one asynchronous refresh, does not block the caller, and concurrent notifications are coalesced through the existing singleflight path.
- [ ] **Step 4: Write the failing T138 compatibility test if the existing scheduler test seam permits it.** Assert a healthy self-owned candidate still wins the resource-tier partition over API Key candidates, and that changing quality window labels does not alter the self-owned/API Key partition contract.
- [ ] **Step 5: Run only the new focused tests and confirm they fail for the intended missing behavior.** Run from `upstream/sub2api/backend`: `go test ./internal/service -run 'TestOpenAIAccountQuality|TestOpenAIUnifiedQualityScore|TestOpenAIQuality' -count=1` plus the focused repository test if available. Expected: failures mention old seven-day bounds, old window labels/weights, or missing refresh hook.
- [ ] **Step 6: Commit the red tests.** Use `git add` on only the focused test files and commit `test: define t139 quality freshness behavior`.

### Task 2: Implement Rolling Window Aggregation and Score Blend

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_account_quality.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_quality.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_quality_score.go`

**Interfaces:**
- Consumes: failing tests from Task 1 and existing repository/service contracts.
- Produces: `OpenAIQualityWindow5M` and `OpenAIQualityWindow55M` (or equivalent names following existing conventions), one-hour repository bounds, and score blending with `W5=0.40`, `W55=0.60`.

- [ ] **Step 1: Add the new window identifiers and targets.** Keep old identifiers only where required for read-only compatibility; ensure the runtime score path iterates only `W5` and `W55` and does not assign runtime weight to `W7`.
- [ ] **Step 2: Change the repository SQL to classify only the one-hour range.** Replace the current seven-day partitioning with `created_at >= end - interval '5 minutes'` as `w5`, otherwise `w55`; retain all existing deduplication, usage completeness, client-error, unsupported-model, image/video, and account attribution filters.
- [ ] **Step 3: Replace score blending constants and ordering.** Implement the same confidence/carry-to-neutral behavior over `W5` then `W55`, with base weights `0.40` and `0.60`; update output-rate blending and metric targets consistently.
- [ ] **Step 4: Increment the quality score version.** Use a new version string so a seven-day snapshot cannot be mistaken for the one-hour runtime contract.
- [ ] **Step 5: Run the focused tests from Task 1 and fix only implementation defects.** Expected: provider bounds, score weights, confidence fallback, and old-window exclusion pass.
- [ ] **Step 6: Commit the aggregation change.** Use `git add` on the repository/service files and commit `feat: use one-hour quality evidence windows`.

### Task 3: Implement Five-Minute Snapshot Freshness and Real-Result Trigger

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_account_quality.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_service.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_gateway_usage.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_quality_test.go`

**Interfaces:**
- Consumes: Task 2 provider and score contract; existing `OpenAIAccountQualitySnapshotProvider` use in unified scheduling.
- Produces: a narrow optional refresh interface, such as `RefreshOpenAIAccountQuality(ctx context.Context)` or `RequestOpenAIAccountQualityRefresh()`, that can be called after durable real-result persistence without making quality refresh a request-path dependency.

- [ ] **Step 1: Add a failing test for five-minute provider freshness.** Assert an expired one-minute cache does not force a database refresh before the five-minute refresh interval, while a five-minute refresh attempt is allowed; update old tests that encode one-minute refresh as the scheduling contract.
- [ ] **Step 2: Add a failing test for non-blocking notification.** Assert the request-side hook returns immediately even while the refresh query is blocked, and that the later successful query publishes a complete snapshot atomically.
- [ ] **Step 3: Implement provider refresh signaling.** Add a narrow interface separate from `Snapshot`; use singleflight plus a bounded/coalesced notification mechanism so bursts of real requests do not launch one query per request. Keep `Snapshot` reads side-effect-free and fast.
- [ ] **Step 4: Configure the gateway provider.** Set the cache/refresh behavior to the five-minute freshness contract without changing T138 candidate partitioning or native qualification.
- [ ] **Step 5: Hook the real-result path.** After the existing usage/error fact is accepted by the native writer/deduplication path, issue the refresh notification in a goroutine or equivalent non-blocking mechanism. Do not trigger it for client-local rejection, unsupported-model-only errors, images/video, unknown/incomplete facts, or failed persistence.
- [ ] **Step 6: Run provider and usage-focused tests.** Expected: no request latency dependency on the database refresh, one refresh for concurrent notifications, and existing billing/deduplication tests remain green.
- [ ] **Step 7: Commit the freshness trigger.** Use `git add` on the provider, gateway service/usage, and focused tests and commit `feat: refresh quality after real results`.

### Task 4: Preserve T138 and Native Eligibility Semantics

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler_test.go` only if needed
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler_projection_test.go` only if needed
- Modify: `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go` only if a direct regression exposes an existing stale-quality re-admission path required by this spec

**Interfaces:**
- Consumes: Task 2 and Task 3 latest snapshot behavior; T138 resource-tier and native qualification code.
- Produces: regression coverage proving self-owned tier, API Key cold-start priority, native unavailable hard filtering, and cooled-account display behavior are unchanged.

- [ ] **Step 1: Run existing T138 scheduler tests before edits.** Confirm the current self-owned/API Key behavior is the baseline and record any pre-existing failures.
- [ ] **Step 2: Add a focused regression test only for a real T139 interaction.** Cover: native-unavailable account with a stale high score is absent from actual candidates; self-owned healthy account remains in its T138 tier; API Key cold-start still uses priority when quality evidence is insufficient.
- [ ] **Step 3: Make the smallest compatibility fix if required.** Do not alter T138 resource-tier order, priority normalization, maturity threshold, or native availability semantics. If no interaction failure exists, leave production scheduler code unchanged.
- [ ] **Step 4: Run focused scheduler/projection tests.** Expected: T138 tests remain green and no model-level ranking or cooled-account removal is introduced.
- [ ] **Step 5: Commit regression coverage/fix.** Use `git add` only on directly related files and commit `test: preserve t138 scheduling semantics` or a specific `fix:` message.

### Task 5: Integration Verification and Handoff

**Files:**
- Modify: `docs/superpowers/reports/2026-09-09-t139-quality-freshness-implementation-handoff.md`

**Interfaces:**
- Consumes: all implementation commits and focused test results from Tasks 1-4.
- Produces: implementation handoff with commit, file, test, migration/config, rollback, and unverified-item evidence.

- [ ] **Step 1: Run direct backend verification.** From `upstream/sub2api/backend`, run focused service/repository tests, `go build ./cmd/server`, `gofmt` checks, and `git diff --check`. Do not modify tests to bypass baseline failures.
- [ ] **Step 2: Inspect the final diff for forbidden scope.** Confirm no migration, production config, credentials, data fixtures, GitHub Actions workflow, model-level ranking, or T138 semantic overwrite was added.
- [ ] **Step 3: Write the handoff report.** Include baseline `main@1ee82471f`, candidate branch, final commit, changed files, test outputs, migration/config status, production-data status, rollback method, and any baseline test gaps.
- [ ] **Step 4: Commit the handoff report.** Commit `docs: add t139 implementation handoff`.
- [ ] **Step 5: Push the candidate branch only.** Push `codex/t139-quality-freshness`; do not merge `main`, deploy, or alter production from this worktree.

## Self-Review Checklist

- [ ] Spec coverage: rolling one-hour range, W5/W55 definitions, five-minute scheduled refresh, real-result trigger, latest snapshot reads, native availability hard gate, T138 compatibility, account-level UI ranking, failure behavior, tests, and rollback are all mapped to tasks.
- [ ] Placeholder scan: no `TODO`, `TBD`, or unspecified implementation step remains.
- [ ] Type consistency: provider remains compatible with existing `Snapshot` consumers; refresh notification is optional/narrow and does not force repository coupling into unrelated services.
- [ ] Scope check: no migration or unrelated frontend/monitoring work is included.
