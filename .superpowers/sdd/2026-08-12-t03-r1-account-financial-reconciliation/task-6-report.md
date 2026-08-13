# T03-R1 Task 6 Report

- Baseline: `2944a36d11fec648930ac0fef8321a44a66cd377`
- Frontend-only financial home with six cards, unified cutoff, 60-second refresh/cleanup, manual refresh, today-only override, read-only ranges, and exception navigation.
- Removed control-plane dependencies from the profitability view.
- Validation: focused Vitest PASS (4 tests), `pnpm typecheck` PASS, `pnpm build` PASS, `git diff --check` PASS.
- No backend, schema, migration, production, or GitHub Actions changes. `downtime_required=false`.

## Fix Round 1

- Added today-only site cost and literal OAuth daily cost controls; profit and margin remain read-only and non-today ranges expose no edit controls.
- Business dates now use `Asia/Shanghai` formatting rather than UTC ISO truncation.
- Commit: `94063b3393eca9857625af1b68420bc7eb29b7f8`.
- Validation: focused AccountProfitability Vitest (3 passed), `pnpm typecheck`, `pnpm build`, `git diff --check`.
