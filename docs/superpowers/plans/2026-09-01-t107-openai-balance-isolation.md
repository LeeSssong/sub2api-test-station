# T107 OpenAI Balance Isolation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reliably isolate only OpenAI accounts when an upstream error contains explicit balance or quota exhaustion evidence, before pool-mode and custom-error-code early returns.

**Architecture:** Extend the existing deterministic failure classifier rather than adding a new state system. Keep `temp_unschedulable_until` and the existing billing-probe recovery path as the sole isolation and recovery mechanism, while moving the deterministic decision ahead of policy gates in `RateLimitService.HandleUpstreamError`.

**Tech Stack:** Go, `gjson`, existing Sub2API rate-limit service and account repository interfaces, `testify/require`.

**Spec:** `docs/superpowers/specs/2026-09-01-t107-openai-balance-isolation-design.md`

## Global Constraints

- Scope is limited to `PlatformOpenAI`.
- Do not add account states, balance fields, migrations, configuration, pages, or a second recovery system.
- Explicit balance evidence must not classify ordinary permission errors, model errors, 429 responses, or 5xx responses.
- Balance isolation must run before pool-mode and custom-error-code early returns.
- Recovery remains probe-required through the existing `ClearTempUnschedulable` path.

---

### Task 1: Expand and constrain OpenAI balance evidence

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/deterministic_failure_isolation.go`
- Test: `upstream/sub2api/backend/internal/service/deterministic_failure_isolation_test.go`

**Interfaces:**
- Consumes: `classifyDeterministicUpstreamFailure(account *Account, statusCode int, responseBody []byte, requestedModel string)`
- Produces: an account-scoped `balance_exhausted` decision only for OpenAI error responses with approved structured codes/types or explicit English/Chinese exhaustion text.

- [ ] Add table tests for `insufficient_quota`, `insufficient_user_quota`, `E44001`, top-level codes, error types, English phrases, and Chinese phrases.
- [ ] Add negative tests for non-OpenAI accounts, permission/scope errors, model errors, bare 402/403, ordinary 429, and 5xx responses.
- [ ] Run the focused classifier test and confirm the new cases fail for the missing behavior.
- [ ] Implement bounded JSON field extraction and explicit evidence matching with status/platform guards.
- [ ] Re-run the focused classifier test and confirm it passes.

### Task 2: Make balance isolation precede policy early returns

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/ratelimit_service.go`
- Test: `upstream/sub2api/backend/internal/service/ratelimit_service_deterministic_isolation_test.go`

**Interfaces:**
- Consumes: `handleDeterministicUpstreamFailure(...) (handled bool, shouldDisable bool)`
- Produces: one `SetTempUnschedulable` call, no `SetError` call, runtime scheduling block notification, and `shouldDisable=true` even in pool mode or when custom error codes would otherwise skip the status.

- [ ] Add failing tests for pool mode and custom error code exclusion with explicit OpenAI balance evidence.
- [ ] Add a non-balance control test showing existing policy behavior is unchanged.
- [ ] Run the focused rate-limit test and confirm the new precedence cases fail.
- [ ] Move the deterministic balance handling before policy gates while preserving existing credential/model ordering semantics.
- [ ] Re-run the focused rate-limit test and confirm it passes.

### Task 3: Verify recovery compatibility and candidate readiness

**Files:**
- Verify: `upstream/sub2api/backend/internal/service/upstream_billing_probe_test.go`
- Create: `docs/superpowers/handoffs/2026-09-01-t107-openai-balance-isolation-handoff.md`

**Interfaces:**
- Consumes: persisted reason containing `"failure_class":"balance_exhausted"` and `"recovery_policy":"probe_required"`.
- Produces: a `READY_FOR_ROOT_REVIEW` handoff with exact commit, tree, changed files, tests, risks, rollback, and deployment boundaries.

- [ ] Run the existing billing-probe recovery test that clears a balance-exhausted temporary isolation.
- [ ] Run the focused deterministic classifier and rate-limit service tests together.
- [ ] Run `gofmt`, `go test` for the directly affected service package tests, `go build ./cmd/server`, and `git diff --check`.
- [ ] Review the diff for OpenAI-only scope and absence of migration/config changes.
- [ ] Commit the implementation and handoff, then notify the unified release coordinator without merging, pushing, or deploying.
