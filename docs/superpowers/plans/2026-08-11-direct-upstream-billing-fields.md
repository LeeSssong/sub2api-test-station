# Direct Upstream Billing Fields Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Make native per-request upstream cost reliable by reading Sub2API `actual_cost` and New API `quota / quota_per_unit` through correctly normalized native endpoints.

**Architecture:** Preserve the existing administrator cost service, exact request-ID ranking, and native field decoding. Add one focused regression for a New API inference base ending in `/api/v1`, then minimally normalize the duplicated `/api` prefix when constructing New API management endpoints.

**Tech Stack:** Go, `net/http`, `httptest`, Testify.

## Global Constraints

- Keep only exact `request_id` / `upstream_request_id` matching.
- Sub2API cost must come directly from `actual_cost`.
- New API cost must come directly from `quota / quota_per_unit`.
- Add no estimates, fuzzy matching, new credentials, relay-ops dependency, unrelated compatibility work, or GitHub Actions workflow.
- Use strict focused TDD and run only feature-specific validation requested by the user.
- Do not touch the protected `external-primary-production-closure` worktree.
- Do not mark the project task complete until merged `main` is pushed, deployed, and verified online.

### Task 1: Normalize New API Native Billing Endpoints

**Files:**

- Modify: `upstream/sub2api/backend/internal/service/sub_upstream_cost_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_multiplier.go`
- Modify only if required by the focused implementation: `upstream/sub2api/backend/internal/service/sub_upstream_cost.go`
- Modify: `docs/project/project-progress.md`

- [ ] Add a focused failing test whose New API account `base_url` ends in `/api/v1`. Its test server must accept only `/api/log/token` and `/api/status`, return a row with exact request IDs and native `quota`, return native `quota_per_unit`, and assert confirmed cost/profit.
- [ ] Run `cd upstream/sub2api/backend && go test ./internal/service -run 'TestSubUpstreamCostServiceConfirmsNewAPIQuotaCostForAPIInferenceBase|TestSubUpstreamCostServiceConfirmsMatchedActualCost' -count=1` and record the expected RED failure caused by `/api/api/...`.
- [ ] Make the smallest URL-normalization change so `/api/v1`, `/v1`, and `/api` inference bases reach the same New API native `/api/...` endpoints without changing the Sub2API path or matching logic.
- [ ] Rerun the focused RED command and require GREEN, then run `cd upstream/sub2api/backend && go test ./internal/service -run 'TestSubUpstreamCost|NewAPI' -count=1` and `git diff --check`.
- [ ] Update the ledger with implementation and focused validation evidence, self-review the diff, and commit the task.
