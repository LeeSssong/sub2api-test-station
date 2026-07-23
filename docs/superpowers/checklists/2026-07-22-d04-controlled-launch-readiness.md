# D04 Lightweight Controlled Launch Readiness Checklist

**Scope:** no more than 15 launch users
**Policy:** `D04-LIGHTWEIGHT-LAUNCH-v2`
**Prepared values:** USD 20 on the first successful login of each Shanghai day; USD 100 cost-risk ceiling at 1000 bps
**Default decision:** `NO-GO` until the provider-neutral evaluator returns `go`

## Permanent Boundaries

- [x] Reuse the native Sub2API user center and email/password authentication.
- [x] Keep invitation, referral, affiliate rewards, and manual check-in disabled for the launch.
- [x] Enforce the 15-user cap atomically in D04.
- [x] Keep relay-ops `read_only` and Feishu commands `dry_run`.
- [x] Evaluate only the current active upstream; inactive and historical upstream balances are not launch gates.
- [x] Do not change routes, multipliers, prices, Keys, account bindings, or existing balances to open registration.

## A. Software And Reconciliation

- [x] One isolated production user received exactly one USD 20 `daily_login_credit`.
- [x] Same-day login produced no second balance effect.
- [x] D04 ledger, upstream balance history, and current balance reconciled.
- [x] Upstream and D04 usage remained zero during acceptance.
- [x] Registration closed with `403 D04_REGISTRATION_CLOSED` after rollback.
- [x] Launch overlay fixes max users `15`, daily credit `20`, total budget `100.00`, and cost factor `1000` bps.
- [x] Read-only Compose is the unconditional rollback target.

## B. Single Approval And Active-upstream Evidence

- [ ] The one explicit launch approval is recorded as `approvals.launch_approved: true`.
- [ ] Current active-upstream balance is at least the configured USD 5 minimum.
- [ ] Financial evidence is no more than 20 minutes old.
- [ ] The latest 15-minute natural-production-traffic window has at least 20 requests.
- [ ] Quality evidence is no more than 20 minutes old.
- [ ] Success rate is at least 95%; error rate is at most 5%.
- [ ] TTFT P95 is at most 5000 ms; total-latency P95 is at most 45000 ms.

Do not calculate balance-runway days or manufacture a model request to obtain samples. Any unknown or threshold failure is `NO-GO`.

## C. Local Account Backup And Health

- [ ] A server-local account backup less than 24 hours old contains `sub2api.dump` and `d04.sqlite`.
- [ ] `SHA256SUMS` verifies both files and metadata marks both scopes complete.
- [x] Sub2API, PostgreSQL, Redis, Caddy, D04, and relay-ops are healthy.
- [x] No launch-critical container is OOM-killed; unexplained restart count is zero.
- [x] Disk use is at most 80%.
- [x] D04 balance drift is zero and no unresolved read-only reason exists.

Off-site backup, retention by days, and recurring restore drills are not part of this lightweight gate. The server keeps the latest three verified local sets as operational housekeeping.

## D. Ownership And Rollback

- [x] Primary operator role is assigned as `site-owner`.
- [x] Support and escalation channel is the existing Feishu operations group.
- [x] The operator can execute the read-only rollback without editing environment values ad hoc.

## E. Opening Gate

Generate the Git-ignored live snapshot and run:

```bash
ruby ops/evaluate-d04-lightweight-launch-readiness.rb evaluate \
  config/operations/D04-lightweight-launch-readiness-v2.yaml \
  config/operations/d04-lightweight-launch-snapshot.local.yaml
```

- [ ] Output is `decision=go`, `blocking_reasons=[]`, `real_action_executed=false`, and `external_system_contacted=false`.
- [ ] The `go` snapshot contains the single explicit `launch_approved: true` approval.

Those two lines describe one gate: approval is an input to the same evaluator result. There is no separate budget approval, opening-window approval, off-site-backup approval, or second application approval. Until it passes, keep `D04_MODE=read_only` and `D04_REGISTRATION_OPEN=false`.

## F. Immediate Stop And Rollback

Close registration immediately on any budget, active-upstream balance, quality, health, backup, drift, or reconciliation failure. Recreate only D04 from the independent read-only Compose file:

```bash
cd /opt/sub2api/production
docker compose -f compose.d04-read-only.yaml config --quiet
docker compose -f compose.d04-read-only.yaml up -d --no-deps --force-recreate internal-test-service
```

Then require:

```text
D04_MODE=read_only
D04_REGISTRATION_OPEN=false
D04 healthy, restart count 0, OOM false
same-origin registration returns 403 D04_REGISTRATION_CLOSED
```

Do not delete users, grants, balance history, D04 ledger rows, or backups during rollback.
