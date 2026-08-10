### Spec Compliance

- Issues found. `external_primary` accepts projections that prove only a small subset of the legacy response contract, then marks the control plane as the source and replaces the legacy result. In Account Monitor, the check is only `range`, `accounts`, and `groups` ([AccountMonitorView.vue:380](/Users/gongtengxinwen/Documents/sub2api%E6%90%AD%E5%BB%BA/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue:380)); in Account Profitability, it is only `rows`, a truthy `summary`, and two date strings ([AccountProfitabilityView.vue:291](/Users/gongtengxinwen/Documents/sub2api%E6%90%AD%E5%BB%BA/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue:291)). Either permits a response missing existing business fields, despite the binding requirement that these fields cannot be removed.

#### Cannot verify from diff

- `skipSessionRecovery: true` is passed on control-plane requests ([controlPlane.ts:55](/Users/gongtengxinwen/Documents/sub2api%E6%90%AD%E5%BB%BA/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/api/controlPlane.ts:55), [controlPlane.ts:70](/Users/gongtengxinwen/Documents/sub2api%E6%90%AD%E5%BB%BA/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/api/controlPlane.ts:70)), but this package does not include the primary API client's interceptor. It cannot establish that a real control-plane 401/403 is isolated from global session recovery.
- The diff does not include the existing status component or freshness composable, so their rendering of all required metadata and their local-degradation behavior cannot be independently verified here.

### Strengths

- Unknown feature-flag values fail closed to `legacy_only` ([controlPlane.ts:8](/Users/gongtengxinwen/Documents/sub2api%E6%90%AD%E5%BB%BA/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/api/controlPlane.ts:8)).
- Shadow reads preserve the legacy result on both migrated account surfaces by returning `null` unless an external-primary response is compatible ([AccountMonitorView.vue:399](/Users/gongtengxinwen/Documents/sub2api%E6%90%AD%E5%BB%BA/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue:399), [AccountProfitabilityView.vue:311](/Users/gongtengxinwen/Documents/sub2api%E6%90%AD%E5%BB%BA/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue:311)).
- Usage intentionally retains the legacy detail surface and labels `external_primary` as degraded rather than claiming external data has replaced it ([UsageView.vue:449](/Users/gongtengxinwen/Documents/sub2api%E6%90%AD%E5%BB%BA/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/views/admin/UsageView.vue:449)).

### Issues

#### Critical

- None.

#### Important

- [AccountMonitorView.vue:380](/Users/gongtengxinwen/Documents/sub2api%E6%90%AD%E5%BB%BA/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue:380) and [AccountProfitabilityView.vue:291](/Users/gongtengxinwen/Documents/sub2api%E6%90%AD%E5%BB%BA/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue:291): `external_primary` type guards are not full-contract guards. Impact: a syntactically shallow external response can be rendered as the source of truth while omitting fields used by legacy cards, details, filters, sorting, or CSV output, violating the preserved-business-fields requirement and risking a broken administrator view. Required fix: validate every required legacy response and row field before selecting an external result, or keep the legacy result with a local degraded status until Task 9's approved compatibility gate confirms an exact mapped contract.
- [AccountMonitorView.spec.ts:350](/Users/gongtengxinwen/Documents/sub2api%E6%90%AD%E5%BB%BA/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts:350), [AccountProfitabilityView.spec.ts:55](/Users/gongtengxinwen/Documents/sub2api%E6%90%AD%E5%BB%BA/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts:55), and [UsageView.spec.ts:219](/Users/gongtengxinwen/Documents/sub2api%E6%90%AD%E5%BB%BA/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/views/admin/__tests__/UsageView.spec.ts:219): new page tests cover successful shadow reads, and only the monitor test covers an error. None proves that a compatible external-primary response replaces visible legacy data with a complete contract, that an incompatible external-primary response stays visibly legacy and degraded, or that 401/403 remains local on every read surface. Impact: the task's highest-risk mode and session-isolation requirement can regress while the added suite stays green. Required fix: add page-level external-primary compatible/incompatible assertions for both account surfaces, and local 401/403 degradation assertions for Monitor, Profitability, and Usage; add an integration-level client/interceptor test where the API client behavior is owned.

#### Minor

- [controlPlaneApi.spec.ts:32](/Users/gongtengxinwen/Documents/sub2api%E6%90%AD%E5%BB%BA/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/__tests__/controlPlaneApi.spec.ts:32): the session-recovery test verifies only mock call options. Impact: it documents intent but does not exercise the actual session-recovery branch. Required fix: cover the real API client/interceptor in its owning test suite, including 401 and 403.

### Assessment

`Task quality: Needs fixes` because the external-primary compatibility checks are insufficient to guarantee preservation of the established administrator response contract, and the focused tests leave its principal fallback/session-isolation paths unproven.
