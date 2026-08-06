# Follow-up fix — manual multiplier for upstream accounts without native evidence

Read first:

1. `docs/project/account-monitor-v3-acceptance-contract.md`
2. `docs/project/active-delivery-contract.md`
3. `docs/superpowers/plans/2026-08-04-account-monitor-card-production-implementation-plan.md`
4. `.superpowers/sdd/2026-08-04-account-monitor-card-production-implementation-plan/progress.md`

User-confirmed addition: when an upstream account (commonly newapi) does not return a native multiplier, the account card must allow the administrator to enter and later modify a multiplier. This is a narrow follow-up; preserve all V3 layout and page-boundary decisions.

## Required behavior

1. Backend multiplier resolution: an account with `rate_multiplier_policy=manual_override` and a valid non-negative `accounts.rate_multiplier` must project that value in `AccountMonitorMultiplier` even when no upstream billing probe snapshot exists. Mark it as an editable/manual declaration in a stable API field (`source` or an explicit boolean), without pretending it was natively probed. Existing managed/measured resolution stays unchanged.
2. Backend write path: the existing account update API must accept `rate_multiplier` together with `rate_multiplier_policy=manual_override`, validate finite `>= 0`, persist both, and keep the manual value from being overwritten by a later managed probe. Clearing the manual value/policy is not required in this wave unless the existing API already has a safe explicit managed-policy path; do not invent a destructive clear button.
3. Frontend card: when procurement cost is absent and native multiplier is missing/unavailable, show a clear Chinese action to enter/edit “账号倍率”. Use a numeric input with finite `>= 0` validation, preserve draft/focus/error on rejected save, and call `adminAPI.accounts.update(accountID, { rate_multiplier: value, rate_multiplier_policy: 'manual_override' })`. After success show the saved value and manual/source detail. When a native value is present and not manual, keep it read-only; do not put group multiplier on the card.
4. View wiring: pass completion callbacks from `AccountMonitorView.vue` to the card and surface API errors without replacing the draft on failure.

## Tests first

Add failing tests before implementation, then green tests covering:

- service resolver returns manual rate without a probe snapshot and labels it manual/declaration;
- manual policy write persists and managed refresh does not overwrite it;
- card renders the enter action when multiplier is missing, validates invalid/negative input locally, emits the exact update payload, and preserves draft/focus/error when the parent rejects;
- card renders a saved manual multiplier as editable and never shows a group multiplier label.

Keep existing procurement-cost tests and six card sections intact. Do not change priority, group sorting, request/error-window semantics, or deployment state.

## Report

Append `manual-multiplier-report.md` with root cause, RED/GREEN commands and output, files changed, commit SHA, and concerns. No push, deployment, production access, or project completion claim.
