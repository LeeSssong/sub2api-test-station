# Task 1 Report — Page-Level Guard Tests

- Task: T06 Task 1, add page-level guard tests for the native profitability page.
- Baseline: `main@032b3591e2df7408641b48ae584c10eee8e7a0be`
- Candidate commit: `824338788`
- Scope: only `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts` and this report file.

## What changed

- Added a page-source denylist guard using `AccountProfitabilityView.vue?raw`.
- Kept the existing native financial report setup and assertions.
- Extended the render test so it confirms refresh goes through `adminAPI.accountFinancial.getReport` and does not touch `setTodayOverride` or `setOAuthCost`.

## RED evidence

1. First attempt used a temporary failing assertion that expected `'/xingqiao/'` to exist in the page source.
2. Focused Vitest failed, but the first version failed for an implementation detail in the test itself:
   - command: `pnpm exec vitest run src/views/admin/__tests__/AccountProfitabilityView.spec.ts`
   - failure: `TypeError: The URL must be of scheme file`
3. I corrected the test to use the page `?raw` import, then reran the same focused Vitest command.
4. The test then failed for the intended reason:
   - failure: `expected '<template>...` to contain `'/xingqiao/'`
   - meaning: the current page source does not contain the banned control-plane path, so the temporary red assertion behaved as expected.

## GREEN evidence

- Focused Vitest:
  - command: `pnpm exec vitest run src/views/admin/__tests__/AccountProfitabilityView.spec.ts`
  - result: `Test Files 1 passed (1)`, `Tests 5 passed (5)`
- Scope checks:
  - `git diff --check` passed with no output.
  - `rg -n "controlPlaneAPI|ControlPlaneResponse|ReadModelStatus|useReadModelFreshness|resolveTrustedPageDecision|controlPlaneResponse|controlPlaneDegraded|renderSource|/api/v1/xingqiao|/xingqiao" upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`
  - result: the only match was the new denylist assertion in the spec file; the runtime page file had no matches.

## Self-review

- Runtime code was not modified.
- No shared control-plane files were modified.
- No API, database, config, production, or project-ledger files were touched.
- The guard remains page-local and source-based, which is the intended contract for this task.

## Residual risks

- The page is already runtime-clean, so this task is primarily a regression guard.
- The denylist only protects the page source covered by this spec; it does not scan unrelated files.
- Existing untracked design/planning docs in `.superpowers/sdd/2026-08-14-t06-profitability-native-only/` were left untouched.
