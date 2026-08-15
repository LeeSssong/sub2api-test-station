# T11 Account Financial Dimensions Handoff

Date: 2026-08-15

State: `READY_FOR_ROOT_REVIEW_WITH_USER_WAIVER`

Branch: `codex/account-financial-dimensions`

Baseline: `bdfd05578`

Runtime candidate: `aa224a764`

## Delivered Behavior

- Fixed whole-site financial summary with group and unassigned scope tabs.
- Backend group/account projections use persisted usage group identity without
  double counting or guessed allocation of account-level adjustments.
- Group account rows are returned by the native financial API.
- Financial report failures show a localized retry action.
- Exception drill-down preserves pending/account/range filters, including an
  exact rolling 24-hour interval.
- Exception content always renders loading, data, empty, or retryable error.
- Explicit all-filter selections remain authoritative and stale overlapping
  requests cannot overwrite the latest result.

## Candidate Commits

```text
818c172fa feat: add native group financial projections
c522d69a9 feat: add account financial scope views
76fe953db fix: render cost exception route states
bba835713 fix: localize cost exception retry state
2e756fe60 fix: address financial page review findings
aa224a764 fix: localize financial retry action
```

Review/report commits follow the runtime candidate and do not change runtime
behavior.

## Verification

- Backend focused matrix: PASS.
- Frontend focused matrix: 42/42 PASS.
- Typecheck and production build: PASS.
- Diff, migration, dependency, GitHub Actions, and forbidden-scope guards:
  PASS.
- Controlled desktop/mobile visual QA: PASS; not production evidence.

## Review Exception

The user explicitly waived the fresh whole-branch independent review with
`跳过这个审核`. Initial task reviews and Task 3 re-review remain recorded; no
artifact claims that the waived review passed.

## Release Properties

- Database migrations: none.
- Dependencies: none.
- Runtime configuration: none.
- GitHub Actions: none.
- Expected downtime: false, subject to the host preflight result.
- Rollback: blue-green application rollback to the previous active image; no
  data rollback is required.

## Root Actions

1. Confirm root `main` is still `bdfd05578` and no release is active.
2. Merge this branch into root `main` and record conflicts, if any.
3. Run merged-main focused tests, typecheck/build, scope guards, and release
   evidence generation.
4. Push only verified root `main`.
5. Run the reviewed blue-green production chain.
6. If `downtime_required=true`, stop before mutation and request explicit user
   authorization.
7. Verify production financial scopes, unassigned projection, exact exception
   navigation/states, native-only requests, mobile overflow, and health.
