# Sub2API Native Ops Simplification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace relay-ops upstream-management forms with a live, read-only Sub2API administrator projection and a clear controlled-testing readiness view.

**Architecture:** `DatabaseOpsSource` reads current accounts and group names through the existing Sub2API reader on every authenticated projection request, then joins the latest D04 result only when the canonical account-set hash matches. The public `/ops` response remains a data-free bootstrap; a hidden-admin middleware protects the data projection and retired mutation paths are removed from the router.

**Tech Stack:** Go 1.24, `net/http`, embedded Go templates, vanilla JavaScript, Ruby/Minitest policy evaluator, Docker Compose, Caddy.

## Global Constraints

- Sub2API is authoritative for accounts, Base URLs, Keys, groups, scheduling, native registration, users, and sessions.
- Active membership is only `status == "active" && schedulable == true`.
- Do not hard-code provider names or account IDs.
- Keep relay-ops `read_only + dry_run` and D04 `read_only` with registration closed.
- Do not mutate routes, accounts, scheduling, prices, multipliers, balances, Keys, candidates, probes, users, or databases.
- Do not delete historical relay-ops records.
- The generic minimum upstream balance is USD 5.00.

---

### Task 1: Hide administrator resources as not found

**Files:**
- Modify: `relay-ops-service/internal/adminauth/middleware.go`
- Modify: `relay-ops-service/internal/adminauth/middleware_test.go`
- Modify: `relay-ops-service/internal/http/server.go`
- Modify: `relay-ops-service/internal/http/server_test.go`

**Interfaces:**
- Produces: `adminauth.RequireHiddenAdmin(Verifier, http.Handler) http.Handler`
- Consumes: existing `Verifier.VerifyAdminSession` and `domain.AdminActor` context.

- [ ] Write failing tests requiring HTTP 404 for missing, invalid, non-admin, disabled-admin, and retired mutation routes while a valid active administrator still receives the projection.
- [ ] Run `go test ./internal/adminauth ./internal/http -count=1` and confirm the new tests fail because hidden middleware and route retirement are absent.
- [ ] Implement `RequireHiddenAdmin` without changing token parsing or Sub2API verification, mount it on the projection, and remove candidate/upstream/billing/synthetic/daily-report/quality-preview mutation routes from `NewServer`.
- [ ] Run `go test ./internal/adminauth ./internal/http -count=1` and confirm success.

### Task 2: Project live Sub2API active accounts

**Files:**
- Modify: `relay-ops-service/internal/http/sources.go`
- Modify: `relay-ops-service/internal/http/sources_test.go`
- Modify: `relay-ops-service/internal/http/d04_readiness_test.go`
- Modify: `relay-ops-service/internal/app/app.go`

**Interfaces:**
- Consumes: `sub2api.Reader.ListAccounts(context.Context)` and `sub2api.Reader.ListGroups(context.Context)`.
- Produces: current active-account rows and a readiness join that rejects old account-set evidence.

- [ ] Write failing source tests with live accounts `10/11` and stale readiness accounts `7/8`; require the output to contain only `10/11`, a fresh current-set hash, and the blocker `upstream_account_set_changed`.
- [ ] Add tests for active/unschedulable/disabled filtering, deterministic sorting, group-name projection, empty sets, and Sub2API read failure.
- [ ] Run `go test ./internal/http -run 'Ops|D04' -count=1` and confirm RED.
- [ ] Replace the legacy production-table projection with a live Sub2API account reader, canonicalize the active set using the existing v3 contract, and join readiness rows by account ID only when hashes match.
- [ ] Wire the existing Sub2API reader into `DatabaseOpsSource` in `app.New` and run the focused tests to GREEN.

### Task 3: Distill `/ops` to a read-only status page

**Files:**
- Modify: `relay-ops-service/internal/http/templates/ops.html`
- Modify: `relay-ops-service/internal/http/templates/ops-bootstrap.html`
- Modify: `relay-ops-service/internal/http/static/ops.js`
- Modify: `relay-ops-service/internal/http/static/ops-admin.js`
- Modify: `relay-ops-service/internal/http/static/app.css`
- Modify: `relay-ops-service/internal/http/server_test.go`

**Interfaces:**
- Consumes: the Task 2 live readiness projection.
- Produces: a data-free bootstrap and 30-second authenticated read-only refresh.

- [ ] Write failing HTML/JavaScript contract tests requiring `内测开放状态`, `当前活动上游`, a collapsed technical-details region, a 30-second refresh, and a native not-found redirect.
- [ ] Assert absence of all forms, inputs, selects, textareas, mutation buttons, Base URL/Key/candidate/billing-session copy, acceptance controls, and retired API paths.
- [ ] Run `go test ./internal/http -count=1` and confirm RED.
- [ ] Replace the current template with the approved read-only layout, reduce `ops-admin.js` to authenticated refresh/error handling, and keep the bootstrap free of operational data.
- [ ] Run focused Go tests plus `node --check internal/http/static/ops.js` and `node --check internal/http/static/ops-admin.js`.

### Task 4: Correct the lightweight balance policy

**Files:**
- Modify: `config/operations/D04-lightweight-launch-readiness-v3.yaml`
- Modify: `tests/operations/evaluate_d04_lightweight_launch_readiness_v3_test.rb`
- Modify: `docs/superpowers/checklists/2026-07-22-d04-controlled-launch-readiness.md`
- Modify: affected current runbook references found by exact search.

**Interfaces:**
- Produces: provider-neutral USD 5.00 minimum-balance behavior.

- [ ] Change/add a failing boundary test requiring USD 4.99 to block and USD 5.00 to pass.
- [ ] Run `ruby -Itest tests/operations/evaluate_d04_lightweight_launch_readiness_v3_test.rb` and confirm the USD 5.00 assertion fails against the old USD 10.00 policy.
- [ ] Set only the v3 active-upstream minimum to `5.0` and update current v3 controlled-launch documentation; retain historical v2 evidence unchanged.
- [ ] Re-run v3 and historical evaluator regression tests.

### Task 5: Verify, deploy safely, and decide launch readiness

**Files:**
- Modify: `tests/relay_ops/validate_relay_ops_contract.sh`
- Create: `docs/superpowers/reports/2026-07-22-sub2api-native-ops-simplification-verification.md`
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`

**Interfaces:**
- Consumes: Tasks 1-4.
- Produces: production-safe read-only evidence and the next controlled-testing decision.

- [ ] Extend the deployment contract to forbid retired browser mutation routes and require unchanged production modes.
- [ ] Run the complete relay-ops race suite, Go vet, Ruby v3/v2 policy suites, JavaScript syntax checks, Compose/infra contracts, and `git diff --check`.
- [ ] Recheck current production `active + schedulable` account IDs, health, modes, routing hash, and business-row counts without reading secrets or issuing model requests.
- [ ] Build a pinned AMD64 relay-ops image and recreate only relay-ops if the pre-deployment evidence still matches; do not recreate Sub2API, PostgreSQL, Redis, Caddy, or D04.
- [ ] Browser-verify desktop and mobile `/ops`: valid administrator sees current live accounts and auto-refresh metadata; missing/non-admin paths expose no operational view and end on not found.
- [ ] Confirm all service IDs except relay-ops are unchanged, relay-ops is healthy/restart `0`, modes remain `read_only + dry_run`, D04 remains `read_only/closed`, and no route/account/scheduling/price/multiplier/balance/Key/candidate/probe/user/database state changed.
- [ ] Record whether launch remains `NO-GO` due to current XM quality evidence. Do not open registration merely because the UI is fixed.

## Plan Self-review

- Spec coverage: authentication hiding, live Sub2API discovery, stale-result rejection, UI removal, auto-refresh, USD 5 boundary, deployment safety, and launch decision each have a dedicated task.
- Placeholder scan: no deferred implementation requirement remains.
- Type consistency: `sub2api.Reader` is the sole live account/group source; the existing `d04readiness.Result` remains the evidence input; no new database or privileged control API is introduced.

