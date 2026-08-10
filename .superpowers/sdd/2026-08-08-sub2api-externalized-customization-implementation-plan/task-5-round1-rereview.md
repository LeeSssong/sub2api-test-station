### Finding Verdicts

1. ADDRESSED. Account Monitor and Account Profitability no longer select the
   control-plane projection in `external_primary`: both load their legacy
   response into the visible model while marking that mode locally degraded
   ([AccountMonitorView.vue:382](/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue:382), [AccountMonitorView.vue:404](/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue:404), [AccountProfitabilityView.vue:295](/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue:295), [AccountProfitabilityView.vue:315](/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue:315)). The new compatible-looking and incomplete-response tests assert legacy content, source, and local degradation ([AccountMonitorView.spec.ts:380](/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts:380), [AccountProfitabilityView.spec.ts:80](/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts:80)).

2. ADDRESSED. Monitor, Profitability, and Usage each exercise 401 and 403 as
   page-local degradation while retaining legacy output ([AccountMonitorView.spec.ts:394](/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts:394), [AccountProfitabilityView.spec.ts:94](/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts:94), [UsageView.spec.ts:238](/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/views/admin/__tests__/UsageView.spec.ts:238)). The real Axios-client regression calls `controlPlaneAPI.monitor`, receives a 401 through the response interceptor, and proves `skipSessionRecovery`, retained tokens, and no route change ([client.spec.ts:354](/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/api/__tests__/client.spec.ts:354)); that covers the actual 401-only session-recovery branch ([client.ts:177](/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/api/client.ts:177), [client.ts:190](/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/fix-official-update-stuck/upstream/sub2api/frontend/src/api/client.ts:190)). This incidentally resolves the deferred mock-only control-plane assertion concern.

### New Breakage in the Fix Diff

None.

### Out-of-Scope Observations

`external_primary` deliberately remains a legacy-rendering, locally degraded
mode until Task 9 provides the approved full-contract mapping and comparison
gate. This is the specified containment behavior, not a new regression.

### Verdict

All findings addressed, no new Critical/Important breakage.
