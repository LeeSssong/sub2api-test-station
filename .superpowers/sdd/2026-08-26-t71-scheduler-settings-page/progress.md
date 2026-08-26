# T71 scheduler settings page progress

- **Status:** READY_FOR_ROOT_REVIEW
- **Worktree:** `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t71-scheduler-settings-page`
- **Branch:** `codex/t71-scheduler-settings-page`
- **Baseline:** `main@a98e932581eaa643c8ec2edbfefbdcdd75e60353`
- **Scope:** Move the T68 operational scheduler editor into an administrator-only native page, add visible selected states and live previews, and remove the legacy SettingsView presentation.
- **Constraints:** Reuse `/admin/settings` and `openai_advanced_scheduler_*`; no backend/API/migration/scheduler/config-source changes, no production writes, no GitHub Actions.
- **Implementation:** Added `SchedulerSettingsView.vue` and pure `schedulerPolicy.ts`; added route `/admin/scheduler-settings`, admin sidebar entry, zh/en locales, route/sidebar contracts, and direct page/policy tests. Legacy scheduler markup is hidden from SettingsView so only the dedicated page edits the policy.
- **Direct gates:** Scheduler/policy/router/sidebar/locale/SettingsView Vitest passed (`52 passed, 11 legacy scheduler tests skipped because their old surface was intentionally replaced`); `pnpm typecheck` passed; `pnpm build` passed; `git diff --check` passed.
- **Visual check:** Local dev server started on `http://127.0.0.1:3000`; unauthenticated browser correctly redirected to `/login?redirect=/admin/scheduler-settings`. Authenticated visual inspection remains for root online acceptance.
- **Migration/config:** No database migration, backend schema, dependency, or new configuration source.
- **Release:** Root must review and merge this candidate, then run the existing local/host blue-green release chain. This candidate has not been pushed, merged, or deployed.
- **Rollback:** Revert the T71 candidate commit and redeploy the previous validated `main` through the same release chain.
- **Residual risk:** Authenticated desktop/mobile visual acceptance and production route/sidebar verification remain for root; existing SettingsView test harness emits unrelated router-link/jsdom warnings.
