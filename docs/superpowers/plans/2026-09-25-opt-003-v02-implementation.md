# OPT-003 v0.2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the confirmed OPT-003 v0.2 user-end reliability, formatting, shared interaction, responsive layout, typography, and monitor-check changes without changing payment-gate behavior or backend billing semantics.

**Architecture:** Keep Sub2API's existing Vue and Go boundaries. Add or extend small shared frontend utilities/components where behavior is duplicated, then migrate the confirmed user/admin surfaces incrementally. Keep payment routes under the native `requiresPayment` guard. Integrate the existing monitor candidate logic only after its tests are rebased onto this branch.

**Tech Stack:** Vue 3, TypeScript, Tailwind CSS, Vitest, Vue Test Utils, Go, existing Sub2API API/store/component patterns.

---

## Scope Guardrails

- Do not change `/purchase` or `/orders` payment gating. When `payment_enabled` is false, the native hidden-entry and route-guard behavior remains expected.
- Do not change backend/API numeric precision, billing calculations, database fields, request parameters, or CSV raw precision.
- Do not implement OPT-004's real-user request path for monitor checks.
- Preserve unrelated dirty changes in the root checkout; all implementation work occurs in `/Users/awen/.codex/worktrees/opt-003-v02/星桥测试服`.

### Task 1: Establish frontend baseline and shared money formatter

**Files:**
- Modify: `upstream/sub2api/frontend/src/utils/format.ts`
- Create: `upstream/sub2api/frontend/src/utils/__tests__/format.spec.ts`
- Inspect and migrate: all user/admin files found by `rg -n "toFixed\\((4|6|8)\\)|formatCostFixed|Intl.NumberFormat" upstream/sub2api/frontend/src`

- [ ] Write tests for fixed two-decimal money output for integer, zero, sub-cent, negative, null/undefined, and non-finite values; assert unavailable values use a non-zero placeholder path rather than `$0.00`.
- [ ] Run the focused formatter test and verify it fails for the new contract.
- [ ] Add a single `formatMoneyFixed` helper with explicit invalid-value behavior and keep payment-currency formatting separate.
- [ ] Run formatter tests, then replace only user/admin money display calls; leave token, percentage, duration, multiplier, payment-gateway display, and CSV exports on their existing formatters.
- [ ] Run `pnpm test:run` for formatter and directly affected view tests, `pnpm typecheck`, and `git diff --check`.

### Task 2: Make user usage and history states explicit

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/user/UsageView.vue`
- Modify: `upstream/sub2api/frontend/src/views/user/RedeemView.vue`
- Modify: `upstream/sub2api/frontend/src/views/user/UserOrdersView.vue`
- Modify: `upstream/sub2api/frontend/src/views/__tests__/KeyUsageView.spec.ts`
- Modify or create: affected `src/views/user/__tests__/*.spec.ts`

- [ ] Add failing tests distinguishing initial loading, success with data, successful empty, and failed states for usage overview/charts/details.
- [ ] Add failing tests for refresh failure preserving prior rows, filters, pagination, sorting, and successful sibling sections.
- [ ] Add failing tests for redeem/order history failures showing retryable errors instead of empty states.
- [ ] Implement per-section state and retry actions without converting failures to zero values; keep CSV export raw precision.
- [ ] Verify focused tests and typecheck.

### Task 3: Harden keys, key usage, and independent setting submissions

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/user/KeysView.vue`
- Modify: `upstream/sub2api/frontend/src/views/user/__tests__/KeysView.spec.ts`
- Inspect and modify: shared key dialogs/components under `upstream/sub2api/frontend/src/components/keys/` and `src/features/ai-tools/`

- [ ] Add failing tests for initial list failure, retry, refresh preservation of search/filter/page/sort, and usage failure showing unavailable instead of a fabricated zero.
- [ ] Add failing tests proving each independent settings form disables only its own submit action while another setting remains usable.
- [ ] Implement explicit list/usage error states and local submitting flags; preserve form input on failed saves.
- [ ] Run focused key tests and typecheck.

### Task 4: Unify create-key data contract and branded dropdown behavior

**Files:**
- Modify/create shared adapter and option components under `upstream/sub2api/frontend/src/components/keys/` or `src/features/ai-tools/`
- Modify: `upstream/sub2api/frontend/src/views/user/KeysView.vue`
- Modify: `upstream/sub2api/frontend/src/features/ai-tools/CreateLineKeyDialog.vue`
- Add focused tests beside the affected components

- [ ] Add failing tests for current-tool-only group filtering, shared field-label styles, `/api/v1/groups/rates` user-specific multiplier precedence, group multiplier fallback, and missing-rate handling.
- [ ] Add failing tests for dropdown keyboard open/select/escape behavior and constrained placement at desktop and 390px widths.
- [ ] Implement one shared line-option adapter and one branded dropdown shell; pass scope and form capabilities from each entry point rather than duplicating parsing.
- [ ] Keep multiplier text in `1.0倍率` format and never hard-code production multiplier values.
- [ ] Run focused tests and `pnpm typecheck`.

### Task 5: Consolidate keys layout and compact typography

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/user/KeysView.vue`
- Modify: corresponding user/admin shared styles and layout components identified by the prototype parity scan
- Add/update focused responsive tests where existing conventions support them

- [ ] Add regression assertions for continuous keys work surface, preserved operation/filter/API endpoint order, readable endpoint/list headers, and no horizontal overflow at 390px.
- [ ] Apply the confirmed compact type scale: page title 20/600, section 17/600, panel 14/600, tool/recharge step 15/600, body 14/400, labels/status 12/400-500, dense helper text 11-12px, key stats 28px, secondary stats 24px.
- [ ] Preserve the create-key dialog's dedicated typography contract.
- [ ] Run frontend unit tests, typecheck, build, and browser smoke checks at desktop and 390px.

### Task 6: Integrate monitor-check candidate behavior

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/monitor_v4_check.go`
- Modify: `upstream/sub2api/backend/internal/service/monitor_v4_check_test.go`

- [ ] Rebase the existing candidate logic from `codex/monitor-group-random-probe` onto this branch without copying unrelated files.
- [ ] Run the three focused regression tests first and confirm they cover current-group priority, same-priority random selection, model allowlist intersection, and no-measurable-model rejection.
- [ ] Run `go test ./internal/service -count=1` and `git diff --check`.

### Task 7: Full verification and delivery record

**Files:**
- Update: `docs/project/tasks/OPT-003-user-end-v0.2.md`
- Update: `docs/project/tasks/OPT-003-user-end-v0.2-handoff.md`

- [ ] Run frontend focused tests, full frontend test suite, typecheck, production build, and relevant backend tests.
- [ ] Start a local frontend server and use browser checks for desktop and 390px user/admin surfaces; record any unverified external/payment behavior explicitly.
- [ ] Review diff for scope violations, generated files, secrets, hard-coded multipliers, and accidental CSV precision changes.
- [ ] Update the v0.2 documents to distinguish implemented, locally verified, not deployed, and not yet test-station-verified items.
- [ ] Commit implementation in coherent commits; do not deploy or publish without the separate release authorization required by project rules.
