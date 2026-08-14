# T05 用量页仅使用原生 Sub 数据实施报告

## Baseline

- Baseline SHA: 4c5f0d1587004cfb4d7386d0c947f157678d8803
- Branch: codex/t05-usage-native-only

## RED

- Command: `cd upstream/sub2api/frontend && pnpm exec vitest run src/views/admin/__tests__/UsageView.spec.ts`
- Result: RED as expected. `UsageView.spec.ts` failed 2 tests because native-only assertions observed `/api/v1/xingqiao/externalization/pages/accounting?timezone=Asia%2FShanghai` during initial load and refresh; 16 tests passed.

## GREEN

- Focused UsageView: `cd upstream/sub2api/frontend && pnpm exec vitest run src/views/admin/__tests__/UsageView.spec.ts` — PASS. `UsageView.spec.ts` passed 18 tests in 1 file.
- Neighboring regression suite: `cd upstream/sub2api/frontend && pnpm exec vitest run src/views/admin/__tests__/UsageView.spec.ts src/components/admin/usage/__tests__/UsageTable.spec.ts src/components/usage/__tests__/UsageDetailDialog.spec.ts src/api/admin/__tests__/usageDetail.spec.ts src/components/admin/usage/__tests__/CostExceptionTable.spec.ts src/views/admin/ops/components/__tests__/OpsErrorLogTable.spec.ts src/views/admin/ops/components/__tests__/OpsErrorDetailModal.spec.ts src/api/admin/__tests__/errorDetailResponse.spec.ts src/api/admin/__tests__/admin.usage.spec.ts` — PASS. Vitest matched and ran the 6 existing files from that command: `UsageView.spec.ts`, `UsageTable.spec.ts`, `UsageDetailDialog.spec.ts`, `CostExceptionTable.spec.ts`, `OpsErrorLogTable.spec.ts`, and `OpsErrorDetailModal.spec.ts`; 69 tests passed.
- Neighboring relocated files: `cd upstream/sub2api/frontend && pnpm exec vitest run src/components/usage/__tests__/usageDetail.spec.ts src/views/admin/ops/utils/__tests__/errorDetailResponse.spec.ts src/api/__tests__/admin.usage.spec.ts` — PASS. The 3 files currently present at these paths passed 18 tests.
- Typecheck: `cd upstream/sub2api/frontend && pnpm typecheck` — PASS. `vue-tsc --noEmit` completed successfully.
- Scope checks: `git diff --check` — PASS with no output.
- Scope checks: `rg -n "controlPlaneAPI|ControlPlaneResponse|ReadModelStatus|useReadModelFreshness|resolveTrustedPageDecision|controlPlaneResponse|controlPlaneDegraded|renderSource|loadControlPlaneLedger|accountingLedger|/api/v1/xingqiao|/xingqiao" upstream/sub2api/frontend/src/views/admin/UsageView.vue upstream/sub2api/frontend/src/views/admin/__tests__/UsageView.spec.ts` — PASS, no matches.
- Scope checks: `git status --short --branch` before report update showed only `UsageView.vue` and `UsageView.spec.ts` modified on `codex/t05-usage-native-only`.

## Changed Files

- `upstream/sub2api/frontend/src/views/admin/UsageView.vue`
- `upstream/sub2api/frontend/src/views/admin/__tests__/UsageView.spec.ts`
- `docs/superpowers/reports/2026-08-14-t05-usage-native-only-implementation.md`

## Scope Review

- No shared control-plane files changed.
- No docs/project progress or queue files changed.
- No backend, migration, config, GitHub Actions, cost formula, profit page, account monitor, scheduler, external-primary, or relay-ops main-path changes.
- UsageView no longer imports or renders `ReadModelStatus`, calls `controlPlaneAPI`, reads externalization decisions, tracks control-plane freshness, or overwrites native stats from an accounting ledger.
- `downtime_required=false`.
