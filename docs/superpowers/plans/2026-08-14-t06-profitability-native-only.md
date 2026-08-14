# T06 原生管理员利润页移除外部控制面状态/调用实施计划

> **For agentic workers:** follow this plan task-by-task. Keep every change scoped to `AccountProfitabilityView.spec.ts` and the matching spec/plan/report handoff files. Do not touch runtime code.

## Source

- Spec: `docs/superpowers/specs/2026-08-14-t06-profitability-native-only-design.md`
- Baseline: `main@032b3591e2df7408641b48ae584c10eee8e7a0be`
- Current state: spec approved, no implementation yet, only page-level test guard work is allowed

## Scope

- In:
  - `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`
  - `docs/superpowers/reports/2026-08-14-t06-profitability-native-only-implementation.md`
  - `docs/superpowers/reports/2026-08-14-t06-profitability-native-only-task-review.md`
  - `docs/superpowers/reports/2026-08-14-t06-profitability-native-only-final-review.md`
- Out:
  - `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`
  - shared control-plane files
  - API, database, config, production, GitHub Actions, project-progress, and task-queue files

## Acceptance

- [ ] `AccountProfitabilityView.spec.ts` proves the page only exercises native `adminAPI.accountFinancial` paths.
- [ ] The spec rejects any future page-level control-plane import/call or `ReadModelStatus`/unknown/degraded/integrity-style state resurfacing.
- [ ] Verification evidence is recorded in the implementation report, task review, and final review files.
- [ ] No runtime code or shared control-plane file is modified.

## Tasks

## Task 1: Add the page-level guard tests

- [ ] Add the guard tests in `AccountProfitabilityView.spec.ts`.
  - Keep the current native financial report setup.
  - Add a test that asserts the page renders and refreshes through `adminAPI.accountFinancial.getReport` only.
  - Add a static contract check against the page source so the spec fails if the page reintroduces control-plane symbols or `/xingqiao/` paths.
  - Keep the assertions page-local: no shared control-plane files, no runtime edit targets.

## Task 2: Run the focused regression and record evidence

- [ ] Run the focused regression and record RED/GREEN evidence.
  - First run the focused Vitest command against `AccountProfitabilityView.spec.ts`.
  - If the spec fails because the guard is too strict or under-specified, adjust only the test file.
  - Once green, record the exact command and outcome in the implementation report.

## Task 3: Capture scope checks and review evidence

- [ ] Capture neighboring scope checks and self-review evidence.
  - Run the smallest useful adjacent checks for the frontend area that still prove this page stayed native-only.
  - Record `git diff --check`, scoped `rg`, and `git status` evidence in the implementation report.
  - Write the task-review and final-review reports as read-only handoff evidence.

## Task 4: Hand off the candidate for root gating

- [ ] Hand off the candidate for root gating.
  - Summarize the candidate SHA, changed files, verification commands, and residual risks.
  - Stop after preparing the handoff evidence; do not implement runtime code or broaden the scope.

## Verification Commands

- `cd upstream/sub2api/frontend && pnpm exec vitest run src/views/admin/__tests__/AccountProfitabilityView.spec.ts`
- `cd upstream/sub2api/frontend && pnpm exec vitest run src/views/admin/__tests__/AccountProfitabilityView.spec.ts src/api/admin/__tests__/admin.usage.spec.ts`
- `git diff --check`
- `rg -n "controlPlaneAPI|ControlPlaneResponse|ReadModelStatus|useReadModelFreshness|resolveTrustedPageDecision|controlPlaneResponse|controlPlaneDegraded|renderSource|/api/v1/xingqiao|/xingqiao" upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`

## Risks

- The page is already runtime-clean, so the main risk is writing a guard that is too weak and misses a future regression.
- A static source check can be brittle if the test file imports helper names from nearby code; keep the denylist scoped to page-owned symbols only.
- Any failure that points at unrelated frontend fixtures or environment setup should be treated as a test issue, not a scope expansion.

## Self-review

- Scope check: only page-level tests and report handoff files are allowed in this round.
- Behavior check: the plan does not authorize changes to `AccountProfitabilityView.vue`, shared control-plane modules, APIs, schema, config, or production.
- Evidence check: every acceptance item has a concrete command or file-backed proof path.
- Handoff check: the plan ends with root gating, not implementation.
