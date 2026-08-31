# Plan: T95 账号有效成本归一化与官方利润保护

> Execute in this worktree only. Do not modify root `main`, the global queue, project progress, production, or credentials.

## Source

- Spec: `docs/superpowers/specs/2026-08-30-t95-effective-cost-normalization.md`

## Tasks

- [x] 1. Add RED tests for cost-model formulas, OAuth locking, invalid input, and profit-gate consumption.
- [x] 2. Add backward-compatible account `extra` projection and preservation tests (no schema columns or Ent regeneration).
- [x] 3. Implement the service-level effective-cost provider and admin request/response wiring.
- [x] 4. Extend the existing Account Monitor cost dialog and API types for model selection and ratio inputs.
- [x] 5. Run focused backend/frontend tests, typecheck/build where available, and diff checks.
- [x] 6. Review the diff, commit the candidate, and write handoff evidence.

## Verification Commands

- `go test ./internal/service -run 'TestEffectiveCost|TestOpenAIProfitControl' -count=1`
- `go test ./internal/handler/admin -run 'Test.*EffectiveCost|Test.*Procurement' -count=1`
- `pnpm --dir upstream/sub2api/frontend exec vitest run src/components/admin/account-monitor/AccountMonitorCostDialog.spec.ts`
- `pnpm --dir upstream/sub2api/frontend typecheck`
- `git diff --check`

## Acceptance

- [x] All three models produce the documented `A/R/U` values.
- [x] Profit gate uses computed `U`; unknown U remains in the native invalid-cost partition for T96 availability fallback.
- [x] Admin can configure API Key direct/ratio models in the existing cost dialog; OAuth-family accounts stay self-owned.

## Risks

- Go toolchain may be older than the repository requirement; record the exact blocker if focused tests cannot run.
