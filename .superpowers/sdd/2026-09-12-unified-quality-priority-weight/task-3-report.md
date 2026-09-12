# Task 3 Report: Unified Quality Priority Weighting

## Result

- Baseline main: `b9796b108a`
- Candidate branch: `codex/unified-quality-priority-weight`
- Implementation commit: `597c9768f16b15c7eff0d50dbc51e877bf291c1d`
- Corrective review commit: `611bb683648c9c10edfda984e005b29f3524d952`
- Scope: Task 3 only; Task 1 and Task 2 signal/cap fixes preserved.

## Behavior

- API-key combined score is `quality.QualityScore + coldStartPrioritySignal + dailyPrioritySignal`.
- Cold-start signal is calculated only for eligible API-key candidates below maturity confidence.
- Daily signal is calculated for every eligible API-key candidate, including cold-start candidates.
- Caps are resolved once per unified-quality request and reused by request-local rechecks.
- Global caps default to `50.0` cold-start and `20.0` daily when the service/config fallback is used; group overrides take precedence when valid.
- Self-owned OAuth/SetupToken ordering remains native priority/load ordering.
- Existing success-score, nullable TTFT, and account-ID tie breakers remain after combined score.
- Runtime-blocked candidates are excluded before ranking.
- Decision observability adds finite selected priority, combined signal, cold-start signal, and daily signal fields without sensitive request/account credential data.

## Review Corrections

- Projection now resolves global/group unified-quality caps once per projection request and applies the same cold-start plus daily signals to every API-key candidate before ranking. Traditional projection paths are unchanged.
- Selected priority observability is populated only after an account is acquired or a wait candidate is selected; terminal acquisition failure no longer reports the first ranked account as selected.
- The legacy unresolved-candidate fallback remains available only to isolated direct helper callers. Live unified-quality selection and projection resolve all scheduler candidates with request-scoped caps before comparison.

## Changed Files

- `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go`
- `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- `upstream/sub2api/backend/internal/service/openai_resilience_observability.go`
- `upstream/sub2api/backend/internal/service/openai_scheduler_log_sink.go`
- `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler_test.go`
- `upstream/sub2api/backend/internal/service/openai_unified_quality_observability_test.go`
- `upstream/sub2api/backend/internal/service/openai_scheduler_log_sink_test.go`
- `upstream/sub2api/backend/internal/service/openai_account_scheduler_projection.go`
- `upstream/sub2api/backend/internal/service/openai_account_scheduler_projection_test.go`
- `upstream/sub2api/backend/internal/service/openai_account_scheduler_test.go`

## Verification

- RED: `go test ./internal/service -run 'TestOpenAIUnifiedQuality' -count=1` failed before implementation because the new decision/event fields were absent.
- Review RED: `go test ./internal/service -run 'TestOpenAIUnifiedQualitySelectionClearsPriorityObservabilityWhenAcquisitionFails|TestOpenAIAccountSchedulerProjectionUsesResolvedUnifiedQualityPrioritySignals' -count=1` failed on the pre-correction implementation because failed acquisition retained `SelectedPriority=1`; the strengthened projection regression also targets the old unresolved-signal ranking path.
- Focused scheduler tests: passed.
- Corrective focused tests: `go test ./internal/service -run 'TestOpenAIUnifiedQualitySelectionClearsPriorityObservabilityWhenAcquisitionFails|TestOpenAIAccountSchedulerProjectionUsesResolvedUnifiedQualityPrioritySignals' -count=1 -v` passed.
- Required direct tests: `go test ./internal/config ./internal/service ./internal/repository -run '(OpenAIUnifiedQuality|Scheduler|Settings)' -count=1` passed.
- Build: `go build ./cmd/server` passed.
- Formatting and diff check: `gofmt` and `git diff --check` passed.

## Boundaries and Concerns

- No migration, credential, production-data, production-setting, deployment, or GitHub Actions changes.
- Candidate worktree remains for root review; no merge, push, deployment, or cleanup performed.
- Full repository test suite and online validation were not run; they are outside this implementation task.
- Rollback is reverting the candidate commit before integration or using the prior verified deployment slot after integration.
