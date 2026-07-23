# D04 Launch Readiness And Quality Report Production Design

**Date:** 2026-07-22 (Asia/Shanghai)  
**Status:** Approved execution scope from the current thread

## Problem

The quality-first upstream cycle and the one-user D04 write-path acceptance are complete, but neither proves that registration may safely open for up to 15 users. The running relay-ops image also predates the locally verified quality-report notification wiring.

## Goal

1. Produce an executable, secret-free D04 launch-readiness gate and a safe launch overlay without opening registration.
2. Create and independently restore a fresh production PostgreSQL custom-format backup.
3. Deploy the already tested quality-report monitoring/Feishu increment while preserving `read_only + dry_run` and zero paid probes.
4. Close the three original work lines with requirement-by-requirement evidence.

## Non-goals

- Do not open public registration in this loop.
- Do not create a second D04 user or credit grant.
- Do not send a model request, create a production candidate, or enable a paid probe.
- Do not switch a route or change a multiplier, price, balance, Key, account binding, or Feishu deduplication row.
- Do not manufacture an alert/recovery event for visual evidence.
- Do not process Neko balance.

## Launch Policy

The prepared policy is deliberately conservative:

```text
maximum launch users: 15
daily login credit: USD 20 per Shanghai day
provisional total cost-risk budget: USD 100
budget cost factor: 1000 bps (10%)
minimum Wawazz balance coverage: max(USD 10, 3 days of observed actual spend)
minimum 15-minute samples: 20
minimum success rate: 95%
maximum error rate: 5%
maximum TTFT P95: 5000 ms
maximum total-latency P95: 45000 ms
maximum backup age: 24 hours
maximum restore-drill age: 31 days
maximum disk use: 80%
```

The USD 100 budget is a proposal derived from the existing read-only D04 configuration, not permission to spend. Registration remains closed until the user separately approves the exact launch window and the current provider balance covers the policy. If the observed spend rate is unavailable, the balance gate fails closed.

## Readiness Evaluator

`ops/evaluate-d04-launch-readiness.rb` consumes two secret-free YAML files:

- a tracked policy file with the exact thresholds above;
- a Git-ignored/live snapshot containing only health, aggregate metrics, backup ages, mode flags, and role labels.

It emits deterministic JSON with:

```text
decision: go | no_go
blocking_reasons: stable lowercase codes
required_actions: ordered operator actions
real_action_executed: false
external_system_contacted: false
```

Invalid, missing, stale, or unknown evidence is blocking. Credential-shaped keys and values are rejected.

## Production Backup And Restore

The backup uses `pg_dump -Fc` from the running PostgreSQL 18 container and is stored under a server-only `0700` directory with a `0600` archive. It records only timestamp, size, SHA-256, PostgreSQL version, duration, and aggregate verification counts.

Restore uses a new temporary PostgreSQL 18 container, isolated network, isolated volume, and a different database name. It publishes no port and never connects to the production network. `pg_restore --exit-on-error` must succeed; schema/table counts, migration evidence, administrator count, key setting count, and relay-ops schema presence are compared using aggregate values. The temporary container, network, and volume are removed after evidence capture. The verified archive remains for rollback and must not enter Git.

## D04 Opening And Rollback Contract

`infra/compose.d04-launch.yaml` is a preparation-only overlay. It sets `write`, registration open, 15 users, USD 20 daily credit, USD 100 total budget, and a qualified 1000-bps Wawazz policy. It is never applied by this loop.

The opening runbook requires a fresh evaluator `go` result and explicit user approval. Rollback always recreates D04 from `infra/compose.d04-read-only.yaml` only, then proves:

```text
D04_MODE=read_only
D04_REGISTRATION_OPEN=false
same-origin registration = 403 D04_REGISTRATION_CLOSED
```

## Relay-ops Deployment

The new image contains the locally verified chain:

```text
fast result -> stored quality report -> stable incident -> deduplicated Interactive Card
```

Deployment changes only the relay-ops image and container. Production stays `RELAY_OPS_MODE=read_only` and `RELAY_OPS_FEISHU_COMMAND_MODE=dry_run`. With zero candidates and read-only mode, no fast job, paid request, or quality card is generated during acceptance. Production acceptance therefore proves image identity, migration compatibility, health/readiness, existing row counts, and scheduler suppression; wire and notification semantics remain covered by the full real-PostgreSQL test suite.

## Acceptance Criteria

- [ ] Accounts `73/74/75` retain their documented cleanup hash and no-switch decision.
- [ ] The launch evaluator rejects unknown or stale evidence and reports the current Wawazz state as `no_go` without contacting an external system.
- [ ] The launch overlay and read-only rollback contract pass automated validation.
- [ ] A fresh production backup restores successfully into an isolated PostgreSQL 18 instance and aggregate checks match.
- [ ] D04 remains healthy, `read_only`, and registration closed; no second grant or usage appears.
- [ ] Only relay-ops is rebuilt; it returns healthy/ready on the new image in `read_only + dry_run`.
- [ ] Candidate, probe-run, notification, route, and D04 reconciliation evidence is unchanged by deployment.
- [ ] Current-state, handoff, runbook, checklist, and final verification report describe the true final state.

## Risks

- The provider balance may be too low for a 15-user opening even when the software gate is ready. This produces `no_go`, not a threshold relaxation.
- A database archive contains sensitive application data. It stays server-side with restrictive permissions and is never printed or committed.
- A relay-ops migration could fail. The old image tag and Compose copy are retained; rollback only recreates relay-ops and never removes the database.
- Server-local backup does not protect against total host loss. Encrypted off-site backup remains a separately tracked resilience improvement and must not be represented as completed.
