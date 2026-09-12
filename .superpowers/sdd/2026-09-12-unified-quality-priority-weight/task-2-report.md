# Task 2 Report: Global and Group Priority Cap Configuration

## Scope

Implemented only Task 2 in `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/unified-quality-priority-weight`.

## Changes

- Added global unified-quality priority caps to `GatewayOpenAISchedulerConfig`.
- Added Viper defaults of `50` for cold-start and `20` for daily priority caps under `gateway.openai_ws`.
- Added config validation rejecting negative, NaN, and infinite global cap values.
- Added nullable group-policy JSON fields for independent cold-start and daily overrides.
- Preserved omitted group fields as `nil`, with read normalization and write validation.
- Preserved existing `WeightOverrides` and `LegacyWeightOverrideIgnored` behavior.
- Added request-time resolution from global caps, then independently applied valid selected-group overrides; malformed values fall back safely to the current finite non-negative value.
- Cloned new pointer fields with cached group policies.
- Did not change ranking or comparator behavior.

## Changed Files

- `upstream/sub2api/backend/internal/config/config.go`
- `upstream/sub2api/backend/internal/config/config_test.go`
- `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- `upstream/sub2api/backend/internal/service/openai_account_scheduler_test.go`
- `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go`
- `upstream/sub2api/backend/internal/service/scheduler_fairness_settings_test.go`
- `upstream/sub2api/backend/internal/service/setting_parse.go`
- `upstream/sub2api/backend/internal/service/settings_view.go`

## TDD Evidence

- RED: `go test ./internal/config ./internal/service -run '(PriorityCaps|SchedulerGroupPolicy|OpenAILegacyWeight)' -count=1` failed because the new config fields, group-policy fields, and resolver were undefined.
- GREEN: the same focused command passed after implementation.

## Tests

- `cd upstream/sub2api/backend && go test ./internal/config ./internal/service -run '(PriorityCaps|SchedulerGroupPolicy|OpenAILegacyWeight)' -count=1`: PASS
- `cd upstream/sub2api/backend && go test ./internal/config`: PASS
- `git diff --check`: PASS

## Commit

- `cb2d1e898a1d7a3a303409b496ddb713b07eb94f` (`feat: configure unified quality priority caps`)

## Concerns / Unverified

- Ranking/comparator integration remains intentionally deferred to the next task.
- No migration, deployment, push, or production verification was performed.
- Global caps are typed with `GatewayOpenAISchedulerConfig` while their compatibility configuration keys remain under `gateway.openai_ws`, as required by the task contract.
