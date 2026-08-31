# Plan: T95 账号有效成本归一化与官方利润保护

> Execute in this worktree only. Do not modify root `main`, the global queue, project progress, production, or credentials.

## Source

- Spec: `docs/superpowers/specs/2026-08-30-t95-effective-cost-normalization.md`

## Tasks

- [ ] 1. Add RED tests for cost-model formulas, OAuth locking, invalid input, and profit-gate consumption.
- [ ] 2. Add backward-compatible account `extra` projection and preservation tests (no schema columns or Ent regeneration).
- [ ] 3. Implement the service-level effective-cost provider and admin request/response wiring.
- [ ] 4. Extend the existing Account Monitor cost dialog and API types for model selection and ratio inputs.
- [ ] 5. Run focused backend/frontend tests, typecheck/build where available, and diff checks.
- [ ] 6. Review the diff, commit the candidate, and write handoff evidence.

## Verification Commands

- `go test ./internal/service -run 'TestEffectiveCost|TestOpenAIProfitControl' -count=1`
- `go test ./internal/handler/admin -run 'Test.*EffectiveCost|Test.*Procurement' -count=1`
- `pnpm --dir upstream/sub2api/frontend exec vitest run src/components/admin/account-monitor/AccountMonitorCostDialog.spec.ts`
- `pnpm --dir upstream/sub2api/frontend typecheck`
- `git diff --check`

## Acceptance

- [ ] All three models produce the documented `A/R/U` values.
- [ ] Profit gate uses the computed `U` and stays fail-open on unknown cost.
- [ ] Admin can configure API Key direct/ratio models in the existing cost dialog; OAuth stays self-owned.

## Risks

- Go toolchain may be older than the repository requirement; record the exact blocker if focused tests cannot run.
