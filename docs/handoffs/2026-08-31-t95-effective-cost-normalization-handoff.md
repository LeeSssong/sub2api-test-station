# T95 Effective Cost Normalization Handoff

## Status

`READY_FOR_ROOT_REVIEW`. The candidate is implemented and directly verified. It has not been merged to root `main`, pushed, deployed, or used to change production data.

T99 remains `INTEGRATING` and owns the only integration/deployment/verification lane. T95 must wait until that lane is released. T96 must not create its worktree or consume this API until T95 is merged into the then-latest clean `main`.

## Identity

- Baseline: `main@879787096a7bc4b3ff4ab4820d4a5f3c3a63a29a`
- Branch: `codex/t95-effective-cost-normalization`
- Worktree: `.worktrees/t95-effective-cost-normalization`
- Refresh merge: `747eaa9d9`
- Implementation commits: `01d1bd30e`, `f5c5a5fee`
- Spec: `docs/superpowers/specs/2026-08-30-t95-effective-cost-normalization.md`
- Plan: `docs/superpowers/plans/2026-08-30-t95-effective-cost-normalization.md`
- Verification: `docs/superpowers/reports/2026-08-31-t95-effective-cost-normalization-verification.md`

## T96 Dependency Contract

T96 may consume only the merged service contract:

```go
type EffectiveCost struct {
    Model  string
    Status string
    A      *float64
    R      *float64
    U      *float64
}

func EffectiveCostForAccount(account *Account) EffectiveCost

const EffectiveCostStatusReady = "ready"
```

T96 must read U live from `EffectiveCostForAccount`; it must not cache U in the 60-second quality snapshot. `oauth` and `setup-token` are both self-owned. Unknown U remains the native invalid-cost veto until T96 applies its separately approved availability-first full-pool fallback.

## Verification And Risk

All direct verification listed in the report passed. The only unverified item is a completely unmodified `go test ./internal/service` run, blocked by three pre-existing main test compilation defects documented in the report. No migration, configuration, production write, deployment, or credentials are involved.
