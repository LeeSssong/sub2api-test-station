# SDD ledger — plan: docs/superpowers/plans/2026-08-08-upstream-failure-cache-aware-resilience-implementation-plan.md

Workspace: `/Users/gongtengxinwen/Documents/sub2api-upstream-resilience-spec`
Branch: `codex/upstream-resilience-implementation`

- Task 1: fix round 2/5 (HTTP 400 transient compatibility, hard-error precedence, retry/side-effect matrix; commits 318cf5bc7..c37565eff; scoped re-review clean, 0 open findings)
- Task 1: complete (commits eb4f9e375..c37565eff; focused and -race verification passed)
- Task 2: fix round 2/5 (fixed passthrough recovery metadata, resume normalization, Messages response-ID capture, hard-event failover, OutputStarted guard, exact OpenAI §6.2 error envelope, and Anthropic Messages protocol envelope; commit d08c495ef; scoped re-review clean, 0 open findings)
- Task 2: complete (commits c37565eff..d08c495ef; focused, focused -race, and broad non-race verification passed)
- Task 3: fix round 4/5 implemented and locally verified (full half-open eligibility/freshness/DB/slot/profit pipeline reuse, account+canonical-model lease scoping, leak-proof pre-Forward release, handler completion ordering, and standalone regressions); candidate pending independent review, with no push or deployment performed
- Task 4: pending
- Task 5: pending
- Task 6: pending
- Task 7: pending
