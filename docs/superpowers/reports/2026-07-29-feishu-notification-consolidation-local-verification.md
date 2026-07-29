# Verification: Feishu Notification Consolidation

**Date:** 2026-07-29
**Status:** local implementation verified; not deployed; production unchanged
**Implementation HEAD before this report:** `57ace95df4f430e2d514ac5d2d19ca0276451f35`
**Branch:** `codex/feishu-notification-consolidation`
**Worktree:** `/Users/gongtengxinwen/Documents/sub2api-feishu-notification-consolidation`

## Scope

- Require an explicit, strict server-side JSON policy before any proactive
  Feishu notification family can deliver.
- Correlate fresh capacity, native-monitor, and real-traffic evidence into one
  user-impact incident per public group.
- Keep numeric evidence updates silent unless they materially change the
  incident, and render escalation reminders from the latest incident snapshot.
- Deliver reliable production pricing changes as P2 one-shot messages and
  daily operations as one digest, outside the incident lifecycle.
- Retry incident and one-shot deliveries through one bounded worker using
  `1/2/5/10` minute backoff.
- Remove production notifier wiring from candidate, quality-report, Usage
  Session, and synthetic acceptance paths.

This verification is local only. It did not deploy an image, install a
production policy, connect to production, or manufacture a Feishu message.

## Commands / Steps

The worktree was clean at implementation HEAD `57ace95df` before Task 12
documentation began. Immediately before this report was created,
`git status --short` contained only:

```text
 M docs/project/current-state.md
```

Fresh required verification:

```bash
cd relay-ops-service
go build ./...
go vet ./...
go test ./... -count=1
cd ..
bash tests/relay_ops/validate_relay_ops_contract.sh
bash tests/infra/validate-baseline.sh
git diff --check
```

Disabled-path audit:

```bash
rg -n 'new_evidence|group #[0-9]+|error_rate|ttft_p95' \
  relay-ops-service/internal/notify
rg -n 'SendIncident|SendOneShot' \
  relay-ops-service/internal/acceptance \
  relay-ops-service/internal/candidates \
  relay-ops-service/internal/qualityreports \
  relay-ops-service/internal/billing
```

Production-safety review:

```bash
git diff codex/feishu-notification-consolidation-design...HEAD -- \
  infra relay-ops-service config tests docs
git status --short
```

The complete scoped diff was reviewed together with targeted added-line scans
over the implementation commits for deployment commands, business writes, and
literal secrets.

## Results

All required verification commands completed successfully:

| Check | Exit | Result |
|---|---:|---|
| `go build ./...` | 0 | all relay-ops packages built |
| `go vet ./...` | 0 | no findings |
| `go test ./... -count=1` | 0 | 39 packages discovered; `cmd/relay-ops` had no test files and every other package reported `ok` |
| relay-ops contract | 0 | container and routing contracts passed |
| infrastructure baseline | 0 | official Sub2API release and infrastructure contracts passed |
| `git diff --check` | 0 | no whitespace errors |
| scoped production-safety diff | 0 | reviewed |

The forbidden-copy search returned two test-only matches:

- `internal/notify/user_impact_test.go` asserts that user-visible cards do not
  contain `group #`, `new_evidence`, `error_rate`, or `ttft_p95`.
- `internal/notify/feishu_test.go` keeps `new_evidence` as an internal
  transition fixture while checking acknowledgement behavior.

The inactive-sender search returned exit `1`, meaning there were no
`SendIncident` or `SendOneShot` matches in acceptance, candidates,
qualityreports, or billing.

The implementation-specific added-line scans found no deployment/push command,
no route/account/multiplier/price/balance/credential/Key write, and no literal
secret. The infrastructure/configuration delta contains only the strict example
policy, two environment-variable declarations, and a read-only policy mount.

## Evidence

Implementation commits after the merged production baseline:

```text
2797d5faa feat: gate Feishu notifications with server policy
fcc00f8d2 feat: persist notification signals and one-shot delivery
d2ee6985b feat: distinguish incident progress from evidence updates
4e36ec267 feat: evaluate and render public group user impact
b503dc43d refactor: collect capacity and monitor evidence without paging
de6ddc492 feat: correlate one user impact incident per public group
0c50089a8 refactor: deliver pricing changes as P2 events
9b29668e8 refactor: deliver daily operations as a digest
095e9da05 fix: render escalation reminders from latest evidence
a7a8591e7 chore: retire inactive Feishu notification paths
57ace95df test: cover consolidated notification delivery lifecycle
```

The end-to-end suite covers:

```text
P1 -> silent numeric update -> concise reminder
   -> two healthy observations -> recovery

reliable production pricing diff -> one P2 one-shot
repeated identical diff -> zero additional delivery
```

The example policy uses `delivery_mode: "shadow"`. It is not installed on the
production host. The production image is unchanged, and the production policy
file does not exist as part of this work.

No Feishu message was manufactured. No route, account scheduling, multiplier,
price, balance, credential, API Key, or user Key was written. No production
database migration, Compose recreation, SSH command, image publication,
deployment, merge, or `git push` was performed.

## Not Verified

- The example policy has not received a separate production review and has not
  been installed in `shadow` mode.
- No 48-hour production review of would-deliver decisions has run.
- `delivery_mode: "enabled"` has not been approved or activated.
- No real Feishu notification has been observed with the consolidated code.
- No 72-hour post-activation notification observation has run.
- Production database migrations and production restart behavior were not
  exercised in this local-only task.

## Follow-ups

1. Review the policy family allowlist and production object/source allowlists
   independently.
2. Install the reviewed policy in `shadow` mode without enabling delivery.
3. Review would-deliver decisions for at least 48 hours, including deduplication,
   severity, freshness, redaction, and inactive-path suppression.
4. Switch to `enabled` only after a separate explicit approval.
5. Observe real notifications for at least 72 hours before considering the
   rollout complete.
