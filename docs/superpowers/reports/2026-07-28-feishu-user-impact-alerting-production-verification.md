# Feishu User-Impact Alerting Production Verification

**Date:** 2026-07-28 (Asia/Shanghai)
**Scope:** severity-aware Feishu alerting, acknowledgement, bounded escalation,
production deployment, synthetic P0 acceptance, and the related Sub2API runtime
and usage investigations.

## Production Incident Findings

The high-error event was an upstream-capacity failure, not a host-resource or
container failure. During 02:12-02:17 CST, the only schedulable account in
`GPT-PLUS-内测` returned two upstream 429 responses. Eight later requests then
returned 503 `No available OpenAI accounts`. The group had four bound accounts,
but only one was schedulable, so there was no usable failover capacity. Service
recovered after about five minutes.

The rolling 24-hour error sample contained 25 errors from two users, compared
with four errors in the preceding 24 hours:

```text
8  503 no available OpenAI account
6  403 insufficient user balance
6  400 failed to read request body
2  429 upstream rate limit
1  502 upstream temporarily unavailable
1  200 upstream request failed
1  403 image generation disabled for the group
```

The six balance errors belonged to one user and were not a server outage.
Recent TTFT evidence also points upstream: affected account paths had multi-second
P50/P95 first-token latency, while the host had low load, about 2.6 GiB available
memory, 53% disk use, no OOM, and no container restarts.

## Usage-Record Finding

The administrator endpoint does not bind usage results to the current
administrator. `/admin/usage` defaults to the latest 24 hours, 20 request rows
per page, ordered newest first. The first page contained requests from only two
high-frequency users, which made it look like an access-control restriction.

Fresh production counts were:

```text
active users                         11
users with usage in rolling 24h       6
all usage rows                     4245
active users with no usage ever       2
distinct users in latest 20 rows      2
```

The administrator navigation also has two similar entries: `/admin/usage` is
the all-site administrator view, while `/usage` is the current user's own view.
The administrator page's user ranking tab, email search, and pagination expose
the wider population.

## TTFT Location And Interpretation

The native Sub2API view is:

```text
Administrator sidebar -> Account Monitor
/admin/accounts/monitor
```

Each account card shows `TTFT P50` for typical first-token latency and
`Latency P95` for tail end-to-end latency. The clock button opens historical
results. At acceptance time, the five schedulable account cards showed TTFT P50
values of approximately 1126, 1460, 1461, 1512, and 2308 ms.

Relay-ops raises a P1 TTFT alert only when the public group's 15-minute P95 is
above 3000 ms, is at least 1.3 times its 24-hour baseline, and remains so for
two consecutive windows. A single slow request remains visible in history but
does not page an operator.

## Deployed Alerting Behavior

The production image is:

```text
sub2api-relay-ops:user-impact-alerting-20260728-v2
sha256:0079ae2d85a4913cf1f3fe70cd01c67619b8c1fdda2f44ad5988297d4bea2e26
linux/amd64
```

The deployment backup is:

```text
/opt/sub2api/production/backups/feishu-user-impact-alerting-20260728T164003Z
```

Only `relay-ops` was recreated. Sub2API, PostgreSQL, Redis, and Caddy remained
running with unchanged container IDs and restart counts. All 16 user-impact
alerting columns are present, including the four delivery-race columns added in
the v2 migration. Both `incident-escalation` and `notification-retry` scheduler
jobs are present and last reported `passed`.

The verified image archive was transferred as independently retried 2 MiB
parts. The reconstructed production archive matched local SHA-256
`f022a6dfd06ee8b90511add08cf024c509e51e4ebc473568cee4af46d10a9813`
before `docker load`. The loaded image ID then matched the locally smoke-tested
image ID above.

The alerting policy now provides:

- P0 for total public-group unavailability or zero successful requests;
- P1 for sustained error rate, sustained TTFT degradation, lost redundancy,
  unschedulable active accounts, or explicit balance exhaustion;
- responsible-person mentions for P0/P1;
- P0 application urgency;
- an acknowledgement action that stops later escalation;
- bounded escalation at 5 and 15 minutes for P0 and 15 minutes for P1;
- stable incident occurrence identity and recovery cards;
- group-level runtime alerts instead of duplicate per-account alert storms.

## Synthetic P0 Evidence

A dedicated P0 acceptance occurrence was delivered to the production alert
chat. The card included the responsible-person mention and acknowledgement
action. The original occurrence was acknowledged through the browser card
flow. The final v2 occurrence used the same authenticated acknowledgement
endpoint with a production admin session obtained from the normal Sub2API login
flow; no credential or token was printed or persisted.

Database evidence, recorded without identifiers:

```text
severity                         P0
confirmed delivery               1
application urgency              delivered / HTTP 200
escalation_1                     1
acknowledgement HTTP             204
acknowledged actor stored        true
recovery delivery                1 / HTTP 200
state before cleanup             recovered
occurrence                       1
next escalation                  null
message id stored                true
escalation_2 after >15 minutes   0
scheduler last status            passed / passed
synthetic residue after cleanup  0
```

The first historical application-urgency attempt returned HTTP 400 and Feishu
business code `99991672` because the application did not yet have
`im:message.urgent` application-identity permission. The minimum permission was
then enabled. A retry of the original message and the final v2 P0 occurrence
both completed application urgency with HTTP 200.

The final v2 incident deliberately remained unacknowledged long enough to emit
exactly one five-minute escalation. A normal authenticated acknowledgement
then returned HTTP 204 and cleared `next_escalation_at`. The production incident
state machine subsequently recorded recovery and `DeliverySender` delivered
exactly one green recovery card. More than 15 minutes after first delivery,
there was no `escalation_2`. The final cleanup transaction removed the current
and two older acceptance incidents, their deliveries, and any analysis rows;
the no-residue query returned zero.

## Local Verification

The following commands passed against the deployed branch:

```text
go test ./... -count=1
go vet ./...
go build ./...
bash tests/relay_ops/validate_relay_ops_contract.sh
bash tests/infra/validate-baseline.sh
ruby -Itest tests/operations/account_quality_monitor_test.rb
ruby -Itest tests/operations/analyze_account_monitor_test.rb
```

Ruby results were 4 runs / 103 assertions and 3 runs / 10 assertions, with zero
failures, errors, or skips.

The final v2 image also completed an isolated PostgreSQL migration and HTTP
startup smoke before transfer:

```text
/healthz                       200
alerting migration columns    4 / 4
container image ID            sha256:0079ae2d85a4913cf1f3fe70cd01c67619b8c1fdda2f44ad5988297d4bea2e26
container platform            linux/amd64
```

## Fresh Production Health

At 01:07 CST on 2026-07-29:

```text
host load                     0.07 / 0.08 / 0.09
available memory              2683 MiB
root filesystem               53% used
relay-ops                     running / healthy / restart 0 / OOM false
Sub2API                       running / healthy / restart 0 / OOM false
PostgreSQL                    running / healthy / restart 0 / OOM false
Redis                         running / healthy / restart 0 / OOM false
Caddy                         running / restart 0 / OOM false
/healthz /readyz /ops /monitor HTTP 200
```

## Completion Evidence

The production acceptance is complete:

- responsible-person mention and application urgency were delivered;
- unacknowledged P0 escalation occurred once at the bounded first level;
- acknowledgement returned HTTP 204 and stopped later escalation;
- one green recovery card was delivered;
- all dedicated synthetic incidents, deliveries, analyses, and retry
  reservations were removed;
- final synthetic residue was zero.

The remaining operational dependency is Feishu itself: application urgency
still depends on the bot remaining in the configured chat, the configured
responsible person remaining a member, and the `im:message.urgent` permission
remaining enabled. Delivery audits distinguish ordinary card success from
urgency status so a future permission regression is visible.
