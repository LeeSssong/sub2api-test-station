# Task 1 Report: Bounded Priority Signal Primitives

## Scope

Implemented only Task 1 in `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/unified-quality-priority-weight`.

## Changes

- Added `openAIUnifiedQualityPriorityCaps` with cold-start and daily caps.
- Added a cap-aware cold-start priority helper while preserving the existing helper signature and behavior.
- Added the daily priority signal helper using `((50-priority)/49)*max`.
- Added finite-positive cap validation and priority clamping through `clampInt`.
- Applied `openAIUnifiedQualityMaturityConfidence` to cold-start signal strength and zeroed the signal at maturity.
- Added `coldStartPrioritySignal` and `dailyPrioritySignal` fields to `openAIUnifiedQualityCandidate`.
- Added table-driven coverage for priority 1, 50, and 100; maturity zeroing; and invalid caps.

## Changed Files

- `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go`
- `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler_test.go`
- `.superpowers/sdd/2026-09-12-unified-quality-priority-weight/task-1-report.md`

## TDD Evidence

- RED: `go test ./internal/service -run 'TestOpenAIUnifiedQuality.*Priority' -count=1` failed because the new cap type and helper symbols were undefined.
- GREEN: the same focused command passed after implementation.

## Tests

- `cd upstream/sub2api/backend && go test ./internal/service -run 'TestOpenAIUnifiedQuality.*Priority' -count=1`: PASS
- `git diff --check`: PASS

## Commits

- Implementation and tests: `8b871f4e64c0f17f95c20c36d1492e7f2f9e701a` (`feat: add unified quality priority signals`)
- This report is recorded separately after the implementation commit.

## Concerns / Unverified

- The new signal fields are primitives only; scheduler comparator integration and settings/config wiring remain intentionally deferred to later tasks.
- No deployment, push, migration, configuration, or production verification was performed.
