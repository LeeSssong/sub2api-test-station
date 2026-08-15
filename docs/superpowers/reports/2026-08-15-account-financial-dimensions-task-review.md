# T11 Account Financial Dimensions Task Review

Date: 2026-08-15

Candidate branch: `codex/account-financial-dimensions`

Baseline: `bdfd05578`

Current candidate: `aa224a764`

Status: task implementation verified; independent scoped re-review and final
whole-branch review remain required before `READY_FOR_ROOT_REVIEW`.

## Task 1

- Implementation: `818c172fa` (`feat: add native group financial projections`)
- Independent read-only review: PASS
- Review confirmed real `usage_logs.group_id` attribution, repeatable-read
  snapshot consistency, unassigned projection, `(group_id, account_id)` rows,
  no whole-site double counting, and no guessed allocation of account-level
  overrides or OAuth daily costs.

## Task 2

- Implementation: `c522d69a9` (`feat: add account financial scope views`)
- Initial independent review: FAIL
- Findings:
  - financial report failure had no visible retryable state;
  - `24h` drill-down did not preserve a rolling 24-hour interval;
  - incomplete and unallocated group notices were mutually exclusive;
  - the required localized group-summary label was missing.
- Fix: `2e756fe60` (`fix: address financial page review findings`)
- Added regression coverage for report failure/retry, simultaneous group
  notices, localized group summary, and rolling 24-hour exception routing.
- First scoped re-review found one remaining P2: the retry action referenced a
  missing production `common.retry` key that the component test had mocked.
- Follow-up fix: `aa224a764` (`fix: localize financial retry action`) adds the
  page-owned retry key to both production locales and asserts the real locale
  objects in the focused test.
- Fresh focused verification: 10/10 tests, `pnpm typecheck`, and
  `git diff --check` passed.
- Browser verification against the local controlled native fixture rendered
  the English error message and literal `Retry` action; it did not render an
  unresolved locale key. Screenshot:
  `output/playwright/t11-account-financial-error-desktop.png`.
- Final whole-branch independent confirmation remains required.

## Task 3

- Implementation: `76fe953db` (`fix: render cost exception route states`)
- Localization follow-up: `bba835713` (`fix: localize cost exception retry state`)
- Initial independent review: FAIL
- Findings:
  - selecting visible "all" fell back to routed evidence/review values;
  - unrelated filter changes reset an explicit local selection;
  - `24h` routing expanded to a calendar-day interval;
  - overlapping reloads could publish stale rows/error/loading state.
- Fix: `2e756fe60`
- Added authoritative local filter semantics, route-specific synchronization,
  exact rolling 24-hour timestamps, latest-request publication guards, and
  deferred-promise concurrency coverage.
- Independent scoped re-review: PASS at `2e756fe60`; 29/29 focused tests,
  `pnpm typecheck`, and scoped `git diff --check` passed. No new P0-P2 finding.

## Fresh Verification After Fix

Backend:

```text
go test ./internal/service -run 'TestAccountFinancial' -count=1        PASS
go test ./internal/repository -run 'TestAccountFinancial' -count=1     PASS
go test ./internal/handler/admin -run 'TestAccountFinancial' -count=1  PASS
go test ./internal/server/routes -run 'Test.*Financial|Test.*CostException' -count=1
                                                                      PASS (no matching tests)
```

Frontend:

```text
4 focused files / 42 tests passed
pnpm typecheck                                                        PASS
pnpm build                                                            PASS
```

Scope guards:

```text
git diff --check bdfd05578...HEAD                                     PASS
no migration delta                                                    PASS
no .github/workflows delta                                            PASS
no /xingqiao, controlPlaneAPI, ReadModelStatus, or external-primary   PASS
```

The existing Playwright captures use a controlled mock/native fixture and are
local visual QA only. They are not production acceptance evidence.

## Review Blocker Record

The desktop collaboration reviewers completed the initial independent reviews.
After `2e756fe60`, attempted command-line read-only reviewers could not start
because both installed external CLIs returned HTTP 401 for their configured
credentials. These attempts are not counted as review passes. The desktop
collaboration reviewer subsequently completed Task 3 and passed it. The
candidate remains before the final review gate until a fresh whole-branch
review confirms Task 2's final localization fix and the complete branch.
