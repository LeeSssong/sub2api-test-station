# D04 Controlled Launch Delta Design

**Date:** 2026-07-21 (Asia/Shanghai)  
**Status:** Approved execution delta  
**Scope:** D04 functional hardening, read-only production assembly, and gated XM qualification

## Goal

Prepare the invitation-only controlled launch without weakening the existing production controls. D04 continues to enforce a maximum of 15 users, one Shanghai-day `$20` check-in grant, one `$5` referrer reward after the invitee's first successful billable request, idempotency, a unified total budget, and provider/local balance reconciliation.

The desired future topology is:

```text
GPT-Plus -> XM API PLUS primary -> Wawazz backup
GPT-Pro  -> XM API PRO primary  -> Wawazz backup
```

This topology is a target, not an inferred production fact. XM accounts, credentials, model support, billing multipliers, and capacity limits must be installed through a controlled path and pass the upstream qualification workflow before any routing proposal or write-mode D04 acceptance.

## Current Production Boundary

- `relay-ops` remains `read_only`; Feishu commands remain `dry_run`.
- D04 `internal-test-service` is not yet deployed.
- Production has no identified XM API PLUS or XM API PRO account.
- Wawazz is the current Plus upstream and may become shared backup only after both required model sets and the intended concurrency policy are verified.
- Neko balance is outside this work.

## Functional Delta

1. All Admin API requests, including `GET`, carry the server-side Admin API key. User-authenticated `/auth/me` requests continue to use only the caller bearer token.
2. A read-only scheduler interval may observe usage and balances but must not create grants, advance usage cursors, mark first usage, pay referral rewards, or change provider balances. Mutation work is skipped explicitly and the interval still succeeds.
3. Cost policy is explicit configuration. Write mode fails closed when the installed policy is absent, invalid, or unqualified. No historical Neko multiplier is used as a default.
4. User-visible copy uses “首发计划”; stable internal D04 identifiers and machine error codes remain unchanged.
5. D04 alerts reuse the already controlled Feishu application path. A missing approved sender is allowed in read-only mode but is a startup error in write mode. No second unmanaged bot is introduced.
6. Public Plus/Pro groups remain the product surface. D04 does not create a hidden Neko-only group.

## Nonfunctional Work

Zero-upstream-cost checks may run in parallel: TLS and edge behavior, login and static-resource latency, service resource headroom, connection limits, and unauthenticated concurrency behavior. Paid or authenticated upstream capacity testing waits for qualified XM accounts and uses bounded isolated identities, explicit budgets, and cleanup evidence.

## Completion Gates

- Focused RED/GREEN tests cover Admin GET authentication, read-only scheduler zero writes, cost-policy fail-closed behavior, public copy, and approved alert wiring.
- Full Go race tests, vet, image build, Compose/Caddy contract tests, and diff checks pass.
- `internal-test-service` is deployed in `read_only` only and completes at least one scheduler interval with no balance, registration, invitation, referral, cursor, or routing mutation.
- XM qualification produces exact account, model, billing, SSE, capacity, and cleanup evidence before a topology proposal is applied.
- Final D04 write acceptance uses isolated low-budget users and proves registration limit, check-in and referral idempotency, total-budget enforcement, and three-way balance reconciliation.

